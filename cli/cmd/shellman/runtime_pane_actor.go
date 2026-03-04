package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"shellman/cli/internal/bridge"
	"shellman/cli/internal/protocol"
)

type paneStatusUpdate struct {
	Target         string
	Title          string
	CurrentCommand string
	Status         SessionStatus
	At             time.Time
	Previous       SessionStatus
}

type paneLastActiveProvider interface {
	PaneLastActiveAt(target string) (time.Time, error)
}

const (
	toolHashTransitionFast = 1200 * time.Millisecond
	realtimeSnapshotMaxLen = 512 * 1024
)

type PaneActor struct {
	target        string
	tmuxService   bridge.TmuxService
	interval      time.Duration
	inputTracker  *inputActivityTracker
	autoComplete  paneAutoCompletionExecutor
	realtime      paneOutputRealtimeSource
	taskStateSink TaskStateSink
	logger        *slog.Logger
	onStatus      func(paneStatusUpdate)

	mu                  sync.RWMutex
	subscribers         map[string]chan protocol.Message
	resetPendingByConn  map[string]bool
	pendingAppendByConn map[string][]protocol.Message
	lastSnap            string
	lastCursorX         int
	lastCursorY         int
	hasCursor           bool
	lastCurrentCommand  string
	statusState         paneStatusState
	// Prevent cold-start static panes from immediately triggering auto-process.
	startupHashCaptured bool
	startupHash         string
	autoProcessArmed    bool

	runCtx                 context.Context
	realtimeActive         bool
	realtimeStop           func()
	statusEvalTimer        *time.Timer
	statusEvalDueAt        time.Time
	statusEvalSeq          uint64
	statusHookLastAt       time.Time
	lastTaskStateReportAt  time.Time
	firstTaskStateReported bool
	seededLastActiveAt     time.Time

	startOnce sync.Once
}

type paneSubscribeOptions struct {
	GapRecover   bool
	HistoryLines int
}

func NewPaneActor(
	target string,
	tmuxService bridge.TmuxService,
	interval time.Duration,
	inputTracker *inputActivityTracker,
	autoComplete paneAutoCompletionExecutor,
	realtime paneOutputRealtimeSource,
	logger *slog.Logger,
	taskStateSinkOpt ...TaskStateSink,
) *PaneActor {
	if interval <= 0 {
		interval = streamPumpInterval
	}
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	var taskStateSink TaskStateSink
	if len(taskStateSinkOpt) > 0 {
		taskStateSink = taskStateSinkOpt[0]
	}
	return &PaneActor{
		target:              strings.TrimSpace(target),
		tmuxService:         tmuxService,
		interval:            interval,
		inputTracker:        inputTracker,
		autoComplete:        autoComplete,
		realtime:            realtime,
		taskStateSink:       taskStateSink,
		logger:              logger,
		subscribers:         map[string]chan protocol.Message{},
		resetPendingByConn:  map[string]bool{},
		pendingAppendByConn: map[string][]protocol.Message{},
	}
}

func (p *PaneActor) Target() string {
	if p == nil {
		return ""
	}
	return p.target
}

func (p *PaneActor) SetStatusHook(fn func(paneStatusUpdate)) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onStatus = fn
}

func (p *PaneActor) SetBaselineLastActiveAtUnix(unixSec int64) {
	if p == nil || unixSec <= 0 {
		return
	}
	at := time.Unix(unixSec, 0).UTC()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.statusState.LastActiveAt.IsZero() {
		p.statusState.LastActiveAt = at
	}
	if p.seededLastActiveAt.IsZero() {
		p.seededLastActiveAt = at
	}
}

func (p *PaneActor) SetRuntimeBaseline(baseline paneRuntimeBaseline) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if baseline.LastActiveAt > 0 && p.statusState.LastActiveAt.IsZero() {
		at := time.Unix(baseline.LastActiveAt, 0).UTC()
		p.statusState.LastActiveAt = at
		if p.seededLastActiveAt.IsZero() {
			p.seededLastActiveAt = at
		}
	}
	if strings.TrimSpace(baseline.SnapshotHash) != "" && p.statusState.PrevHash == "" {
		p.statusState.PrevHash = strings.TrimSpace(baseline.SnapshotHash)
	}
	if baseline.RuntimeStatus != SessionStatusRunning && baseline.RuntimeStatus != SessionStatusReady && baseline.RuntimeStatus != SessionStatusUnknown {
		return
	}
	if p.statusState.Emitted == "" {
		p.statusState.Emitted = baseline.RuntimeStatus
		p.statusState.Candidate = baseline.RuntimeStatus
		if !p.statusState.LastActiveAt.IsZero() {
			p.statusState.CandidateSince = p.statusState.LastActiveAt
		} else {
			p.statusState.CandidateSince = time.Now().UTC()
		}
	}
}

func (p *PaneActor) Start(ctx context.Context) {
	if p == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.mu.Lock()
	p.runCtx = ctx
	p.mu.Unlock()
	p.startOnce.Do(func() {
		go func() {
			<-ctx.Done()
			p.stopStatusEvalTimer()
			p.stopRealtime()
		}()
	})
	p.ensureRealtimeSubscribed()
	p.mu.RLock()
	snapshot := p.lastSnap
	cursorX := p.lastCursorX
	cursorY := p.lastCursorY
	hasCursor := p.hasCursor
	p.mu.RUnlock()
	p.reportTaskState(time.Now().UTC(), snapshot, cursorX, cursorY, hasCursor, "")
}

func (p *PaneActor) Subscribe(connID string, out chan protocol.Message, opts ...paneSubscribeOptions) {
	if p == nil || out == nil {
		return
	}
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return
	}

	opt := paneSubscribeOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}

	p.mu.Lock()
	p.subscribers[connID] = out
	p.resetPendingByConn[connID] = true
	delete(p.pendingAppendByConn, connID)
	p.mu.Unlock()

	snapshot, cursorX, cursorY, hasCursor, err := p.captureResetSnapshotWithOptions(opt)
	if err != nil {
		p.clearResetPending(connID)
		if isPaneTargetMissingError(err) {
			p.sendToConn(connID, paneEndedEventMessage(p.target, err.Error()))
		}
		return
	}

	p.mu.Lock()
	p.lastSnap = snapshot
	p.lastCursorX = cursorX
	p.lastCursorY = cursorY
	p.hasCursor = hasCursor
	p.mu.Unlock()

	for _, msg := range termOutputMessages(p.target, "reset", snapshot, cursorX, cursorY, hasCursor) {
		p.sendToConn(connID, msg)
	}
	p.clearResetPending(connID)
	if opt.GapRecover {
		p.clearPendingAppends(connID)
	} else {
		p.flushPendingAppends(connID)
	}
	p.ensureRealtimeSubscribed()

	p.onSnapshotUpdated(time.Now().UTC())
}

func (p *PaneActor) captureResetSnapshotWithOptions(opt paneSubscribeOptions) (string, int, int, bool, error) {
	if p == nil {
		return "", 0, 0, false, nil
	}
	if opt.GapRecover {
		lines := opt.HistoryLines
		if lines <= 0 {
			lines = streamHistoryLines
		}
		return p.captureResetSnapshotConsistent(func() (string, error) {
			return p.tmuxService.CaptureHistory(p.target, lines)
		})
	}
	return p.captureResetSnapshot()
}

func (p *PaneActor) Unsubscribe(connID string) {
	if p == nil {
		return
	}
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return
	}
	p.mu.Lock()
	delete(p.subscribers, connID)
	delete(p.resetPendingByConn, connID)
	delete(p.pendingAppendByConn, connID)
	p.mu.Unlock()
}

func (p *PaneActor) NotifyEnded(reason string) {
	if p == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "pane closed"
	}
	p.broadcast([]protocol.Message{paneEndedEventMessage(p.target, reason)})
	p.stopStatusEvalTimer()
	p.stopRealtime()
}

func (p *PaneActor) clearResetPending(connID string) {
	if p == nil {
		return
	}
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return
	}
	p.mu.Lock()
	delete(p.resetPendingByConn, connID)
	p.mu.Unlock()
}

func (p *PaneActor) appendPending(connID string, msg protocol.Message) bool {
	if p == nil || !isAppendTermOutput(msg) {
		return false
	}
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return false
	}
	const pendingAppendLimit = 64
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.resetPendingByConn[connID] {
		return false
	}
	queue := p.pendingAppendByConn[connID]
	if len(queue) >= pendingAppendLimit {
		queue = queue[1:]
	}
	queue = append(queue, msg)
	p.pendingAppendByConn[connID] = queue
	return true
}

func (p *PaneActor) flushPendingAppends(connID string) {
	if p == nil {
		return
	}
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return
	}
	p.mu.Lock()
	queue := p.pendingAppendByConn[connID]
	delete(p.pendingAppendByConn, connID)
	p.mu.Unlock()
	if len(queue) == 0 {
		return
	}
	for _, msg := range queue {
		p.sendToConn(connID, msg)
	}
}

func (p *PaneActor) clearPendingAppends(connID string) {
	if p == nil {
		return
	}
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return
	}
	p.mu.Lock()
	delete(p.pendingAppendByConn, connID)
	p.mu.Unlock()
}

func (p *PaneActor) onSnapshotUpdated(now time.Time) {
	if p == nil {
		return
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	p.mu.RLock()
	snapshot := p.lastSnap
	cursorX := p.lastCursorX
	cursorY := p.lastCursorY
	hasCursor := p.hasCursor
	p.mu.RUnlock()

	snapshotHash := snapshotChangeHash(snapshot)
	p.emitStatusWithHash(snapshotHash, now)
	p.reportTaskState(now, snapshot, cursorX, cursorY, hasCursor, snapshotHash)
}

func (p *PaneActor) reportTaskState(now time.Time, snapshot string, cursorX, cursorY int, hasCursor bool, snapshotHash string) {
	if p == nil || p.taskStateSink == nil {
		return
	}

	p.mu.Lock()
	if !p.lastTaskStateReportAt.IsZero() && now.Sub(p.lastTaskStateReportAt) < time.Second {
		p.mu.Unlock()
		return
	}
	p.lastTaskStateReportAt = now
	runtimeStatus := string(normalizeSessionStatus(p.statusState.Emitted))
	reportAt := p.statusState.LastActiveAt
	if !p.firstTaskStateReported && !p.seededLastActiveAt.IsZero() {
		reportAt = p.seededLastActiveAt
	}
	if reportAt.IsZero() {
		reportAt = now
	}
	p.firstTaskStateReported = true
	p.mu.Unlock()
	currentCommand := ""
	commandLoaded := false
	if provider, ok := p.tmuxService.(paneStatusMetadataProvider); ok {
		_, cmd, err := provider.PaneTitleAndCurrentCommand(p.target)
		if err == nil {
			currentCommand = strings.TrimSpace(cmd)
			commandLoaded = true
		}
	}
	if commandLoaded {
		p.mu.Lock()
		p.lastCurrentCommand = currentCommand
		p.mu.Unlock()
	}
	if snapshotHash == "" {
		snapshotHash = snapshotChangeHash(snapshot)
	}

	p.taskStateSink.OnPaneReport(PaneStateReport{
		PaneID:         p.target,
		PaneTarget:     p.target,
		CurrentCommand: currentCommand,
		RuntimeStatus:  runtimeStatus,
		Snapshot:       snapshot,
		SnapshotHash:   snapshotHash,
		CursorX:        cursorX,
		CursorY:        cursorY,
		HasCursor:      hasCursor,
		UpdatedAt:      reportAt.Unix(),
	})
}

func (p *PaneActor) statusItem(now time.Time) (sessionStatusItem, bool) {
	if p == nil {
		return sessionStatusItem{}, false
	}
	p.mu.RLock()
	started := p.startupHashCaptured || p.statusState.Emitted != ""
	status := normalizeSessionStatus(p.statusState.Emitted)
	if status == "" {
		status = SessionStatusUnknown
	}
	at := p.statusState.LastActiveAt
	if at.IsZero() {
		at = now
	}
	item := sessionStatusItem{
		Target:         p.target,
		Title:          sessionTitleFromTarget(p.target),
		CurrentCommand: strings.TrimSpace(p.lastCurrentCommand),
		Status:         status,
		UpdatedAt:      at.Unix(),
	}
	p.mu.RUnlock()
	return item, started
}

func (p *PaneActor) captureResetSnapshot() (string, int, int, bool, error) {
	return p.captureResetSnapshotConsistent(func() (string, error) {
		snapshot, err := p.tmuxService.CapturePane(p.target)
		if err != nil {
			if isPaneTargetMissingError(err) {
				return "", err
			}
			return p.tmuxService.CaptureHistory(p.target, streamHistoryLines)
		}
		return snapshot, nil
	})
}

func (p *PaneActor) captureResetSnapshotConsistent(capture func() (string, error)) (string, int, int, bool, error) {
	if p == nil || capture == nil {
		return "", 0, 0, false, nil
	}
	snapshot, err := capture()
	if err != nil {
		if isPaneTargetMissingError(err) {
			return "", 0, 0, false, err
		}
		return "", 0, 0, false, err
	}
	snapshot = normalizeTermSnapshot(snapshot)
	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		cursorX, cursorY, cursorErr := p.tmuxService.CursorPosition(p.target)
		if cursorErr != nil {
			if isPaneTargetMissingError(cursorErr) {
				return "", 0, 0, false, cursorErr
			}
			return snapshot, 0, 0, false, nil
		}

		// Guard against non-atomic tmux sampling (snapshot/cursor sampled at different moments).
		snapshotCheck, checkErr := capture()
		if checkErr != nil {
			if isPaneTargetMissingError(checkErr) {
				return "", 0, 0, false, checkErr
			}
			return snapshot, cursorX, cursorY, true, nil
		}
		snapshotCheck = normalizeTermSnapshot(snapshotCheck)
		if snapshotCheck == snapshot {
			return snapshot, cursorX, cursorY, true, nil
		}
		snapshot = snapshotCheck
	}
	return snapshot, 0, 0, false, nil
}

func (p *PaneActor) emitStatus(snapshot string, now time.Time) {
	p.emitStatusWithHash(snapshotChangeHash(snapshot), now)
}

func (p *PaneActor) emitStatusWithHash(currHash string, now time.Time) {
	p.mu.Lock()
	prev := p.statusState
	firstTick := !p.startupHashCaptured
	// Cold-start: keep baseline last_active_at on first sample even if hash differs.
	if firstTick && !prev.LastActiveAt.IsZero() && prev.PrevHash != "" {
		prev.PrevHash = currHash
	}
	if !p.startupHashCaptured {
		p.startupHashCaptured = true
		p.startupHash = currHash
	} else if !p.autoProcessArmed && currHash != p.startupHash {
		p.autoProcessArmed = true
	}
	if prev.PrevHash == "" && prev.LastActiveAt.IsZero() {
		if provider, ok := p.tmuxService.(paneLastActiveProvider); ok {
			if at, err := provider.PaneLastActiveAt(p.target); err == nil && !at.IsZero() {
				if at.After(now) {
					at = now
				}
				prev.LastActiveAt = at
			}
		}
	}
	lastInputAt := time.Time{}
	if p.inputTracker != nil {
		lastInputAt = p.inputTracker.Last(p.target)
	}
	transitionDelay := statusTransitionDelay
	if transitionDelay <= 0 || transitionDelay > toolHashTransitionFast {
		transitionDelay = toolHashTransitionFast
	}
	next := advancePaneStatus(
		prev,
		currHash,
		now,
		lastInputAt,
		transitionDelay,
		statusInputIgnoreWindow,
	)
	p.statusState = next
	nextEvalAt, hasNextEval := p.nextStatusEvalAtLocked(next, now, transitionDelay)
	if hasNextEval {
		p.scheduleStatusEvalTimerLocked(nextEvalAt)
	} else {
		p.stopStatusEvalTimerLocked()
	}
	hook := p.onStatus
	currentCommand := p.lastCurrentCommand
	autoProcessArmed := p.autoProcessArmed
	p.statusHookLastAt = now
	p.mu.Unlock()

	nextStatus := normalizeSessionStatus(next.Emitted)
	prevStatus := normalizeSessionStatus(prev.Emitted)
	if autoProcessArmed && prevStatus != SessionStatusReady && nextStatus == SessionStatusReady {
		if !consumeAutoProgressSuppression(p.target, currHash) {
			triggerAutoCompletionByPaneWithObservedAt(p.autoComplete, p.target, next.LastActiveAt, p.logger)
		}
	}
	if hook != nil {
		at := next.LastActiveAt
		if at.IsZero() {
			at = now
		}
		hook(paneStatusUpdate{
			Target:         p.target,
			Title:          sessionTitleFromTarget(p.target),
			CurrentCommand: currentCommand,
			Status:         nextStatus,
			At:             at,
			Previous:       prevStatus,
		})
	}
}

func (p *PaneActor) broadcast(messages []protocol.Message) {
	if p == nil || len(messages) == 0 {
		return
	}

	p.mu.RLock()
	subs := make(map[string]chan protocol.Message, len(p.subscribers))
	for connID, ch := range p.subscribers {
		subs[connID] = ch
	}
	p.mu.RUnlock()

	for connID, ch := range subs {
		for _, msg := range messages {
			if p.appendPending(connID, msg) {
				continue
			}
			if enqueuePaneMessage(ch, msg) {
				continue
			}
			if isAppendTermOutput(msg) {
				continue
			}
			p.logger.Warn("drop pane actor message due to backpressure", "pane_target", p.target, "conn_id", connID, "op", msg.Op)
		}
	}
}

func (p *PaneActor) sendToConn(connID string, msg protocol.Message) {
	if p == nil {
		return
	}
	p.mu.RLock()
	ch := p.subscribers[connID]
	p.mu.RUnlock()
	if ch == nil {
		return
	}
	if enqueuePaneMessage(ch, msg) {
		return
	}
	p.logger.Warn("drop pane actor direct message due to backpressure", "pane_target", p.target, "conn_id", connID, "op", msg.Op)
}

func enqueuePaneMessage(ch chan protocol.Message, msg protocol.Message) bool {
	select {
	case ch <- msg:
		return true
	default:
	}
	if isAppendTermOutput(msg) {
		return false
	}

	// Prefer preserving reset/system events over stale append frames when queue is saturated.
	select {
	case dropped := <-ch:
		if !isAppendTermOutput(dropped) {
			// Keep a non-append frame in front; do not replace it.
			select {
			case ch <- dropped:
			default:
			}
			return false
		}
	default:
	}
	select {
	case ch <- msg:
		return true
	default:
		return false
	}
}

func (p *PaneActor) ensureRealtimeSubscribed() {
	if p == nil {
		return
	}
	p.mu.RLock()
	source := p.realtime
	target := p.target
	ctx := p.runCtx
	active := p.realtimeActive
	p.mu.RUnlock()
	if source == nil || ctx == nil || active {
		return
	}

	ch, stop, err := source.Subscribe(target)
	if err != nil {
		p.logger.Warn("subscribe realtime output failed", "pane_target", target, "err", err)
		return
	}

	p.mu.Lock()
	if p.realtimeActive {
		p.mu.Unlock()
		if stop != nil {
			stop()
		}
		return
	}
	p.realtimeActive = true
	p.realtimeStop = stop
	p.mu.Unlock()
	p.logger.Debug("realtime output subscribed", "pane_target", target)

	go p.realtimeLoop(ctx, ch)
}

func (p *PaneActor) realtimeLoop(ctx context.Context, ch <-chan string) {
	for {
		select {
		case <-ctx.Done():
			p.stopRealtime()
			return
		case data, ok := <-ch:
			if !ok {
				p.stopRealtime()
				return
			}
			if data == "" {
				continue
			}
			messages := termOutputMessages(p.target, "append", data, 0, 0, false)
			p.broadcast(messages)
			p.applyRealtimeOutput(data)
			p.onSnapshotUpdated(time.Now().UTC())
		}
	}
}

func (p *PaneActor) applyRealtimeOutput(data string) {
	if p == nil || data == "" {
		return
	}
	p.mu.Lock()
	p.lastSnap = appendRealtimeSnapshot(p.lastSnap, data, realtimeSnapshotMaxLen)
	p.mu.Unlock()
}

func appendRealtimeSnapshot(prev, chunk string, limit int) string {
	if chunk == "" {
		return normalizeTermSnapshot(prev)
	}
	prevPart := prev
	chunkPart := chunk
	if limit > 0 {
		if len(chunkPart) >= limit {
			chunkPart = chunkPart[len(chunkPart)-limit:]
			prevPart = ""
		} else {
			keepPrev := limit - len(chunkPart)
			if len(prevPart) > keepPrev {
				prevPart = prevPart[len(prevPart)-keepPrev:]
			}
		}
	}
	var buf strings.Builder
	buf.Grow(len(prevPart) + len(chunkPart))
	if prevPart != "" {
		_, _ = buf.WriteString(prevPart)
	}
	_, _ = buf.WriteString(chunkPart)
	return normalizeTermSnapshot(buf.String())
}

func (p *PaneActor) stopRealtime() {
	if p == nil {
		return
	}
	p.mu.Lock()
	stop := p.realtimeStop
	p.realtimeStop = nil
	p.realtimeActive = false
	p.mu.Unlock()
	if stop != nil {
		stop()
		p.logger.Debug("realtime output unsubscribed", "pane_target", p.target)
	}
}

func (p *PaneActor) nextStatusEvalAtLocked(state paneStatusState, now time.Time, transitionDelay time.Duration) (time.Time, bool) {
	if p == nil {
		return time.Time{}, false
	}
	emitted := normalizeSessionStatus(state.Emitted)
	if emitted == SessionStatusUnknown {
		return time.Time{}, false
	}
	if transitionDelay <= 0 || transitionDelay > toolHashTransitionFast {
		transitionDelay = toolHashTransitionFast
	}
	if state.Candidate != emitted {
		if state.CandidateSince.IsZero() {
			return now.Add(transitionDelay), true
		}
		return state.CandidateSince.Add(transitionDelay), true
	}
	if emitted == SessionStatusRunning {
		return now.Add(transitionDelay), true
	}
	return time.Time{}, false
}

func (p *PaneActor) scheduleStatusEvalTimerLocked(at time.Time) {
	if p == nil {
		return
	}
	if at.IsZero() {
		p.stopStatusEvalTimerLocked()
		return
	}
	if p.statusEvalTimer != nil && !p.statusEvalDueAt.IsZero() {
		// Keep an already-scheduled earlier evaluation point; it can re-arm itself if needed.
		if !at.Before(p.statusEvalDueAt) {
			return
		}
		p.stopStatusEvalTimerLocked()
	}
	delay := time.Until(at)
	if delay < 0 {
		delay = 0
	}
	p.statusEvalSeq++
	seq := p.statusEvalSeq
	p.statusEvalDueAt = at
	p.statusEvalTimer = time.AfterFunc(delay, func() {
		p.onStatusEvalTimerFired(seq)
	})
}

func (p *PaneActor) onStatusEvalTimerFired(seq uint64) {
	if p == nil {
		return
	}
	now := time.Now().UTC()
	p.mu.Lock()
	if seq != p.statusEvalSeq {
		p.mu.Unlock()
		return
	}
	p.statusEvalTimer = nil
	p.statusEvalDueAt = time.Time{}
	transitionDelay := statusTransitionDelay
	if transitionDelay <= 0 || transitionDelay > toolHashTransitionFast {
		transitionDelay = toolHashTransitionFast
	}
	if normalizeSessionStatus(p.statusState.Emitted) == SessionStatusRunning && !p.statusState.LastActiveAt.IsZero() {
		nextEvalAt := p.statusState.LastActiveAt.Add(transitionDelay)
		if now.Before(nextEvalAt) {
			p.scheduleStatusEvalTimerLocked(nextEvalAt)
			p.mu.Unlock()
			return
		}
	}
	ctx := p.runCtx
	p.mu.Unlock()
	if ctx != nil {
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	p.onSnapshotUpdated(now)
}

func (p *PaneActor) stopStatusEvalTimer() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopStatusEvalTimerLocked()
}

func (p *PaneActor) stopStatusEvalTimerLocked() {
	if p == nil {
		return
	}
	if p.statusEvalTimer != nil {
		p.statusEvalTimer.Stop()
		p.statusEvalTimer = nil
	}
	p.statusEvalDueAt = time.Time{}
	p.statusEvalSeq++
}

func termOutputMessages(target, mode, data string, cursorX, cursorY int, hasCursor bool) []protocol.Message {
	frames := chunkTermData(mode, data)
	out := make([]protocol.Message, 0, len(frames))
	lastIdx := len(frames) - 1
	for idx, frame := range frames {
		frameData := frame.Data
		if mode == "reset" && idx == lastIdx && strings.HasSuffix(frameData, "\n") {
			// tmux capture-pane includes a terminal sentinel newline that should not
			// move prompt rows when we later restore cursor on task switch.
			frameData = strings.TrimSuffix(frameData, "\n")
		}
		payload := map[string]any{
			"target": target,
			"mode":   frame.Mode,
			"data":   frameData,
		}
		if hasCursor && idx == lastIdx {
			payload["cursor"] = map[string]int{"x": cursorX, "y": cursorY}
		}
		out = append(out, protocol.Message{
			ID:      fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			Type:    "event",
			Op:      "term.output",
			Payload: protocol.MustRaw(payload),
		})
	}
	return out
}

func paneEndedEventMessage(target, reason string) protocol.Message {
	return protocol.Message{
		ID:   fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Type: "event",
		Op:   "pane.ended",
		Payload: protocol.MustRaw(map[string]any{
			"target": target,
			"reason": reason,
		}),
	}
}

func isAppendTermOutput(msg protocol.Message) bool {
	if msg.Op != "term.output" {
		return false
	}
	var payload struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return false
	}
	return payload.Mode == "append"
}
