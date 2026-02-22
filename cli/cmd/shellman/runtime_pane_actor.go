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
	"shellman/cli/internal/streamdiff"
)

type paneStatusUpdate struct {
	Target   string
	Title    string
	Status   SessionStatus
	At       time.Time
	Previous SessionStatus
}

type paneLastActiveProvider interface {
	PaneLastActiveAt(target string) (time.Time, error)
}

const (
	toolHashTransitionFast = 1200 * time.Millisecond
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

	mu          sync.RWMutex
	subscribers map[string]chan protocol.Message
	lastSnap    string
	lastCursorX int
	lastCursorY int
	hasCursor   bool
	statusState paneStatusState
	// Prevent cold-start static panes from immediately triggering auto-process.
	startupHashCaptured bool
	startupHash         string
	autoProcessArmed    bool

	runCtx                 context.Context
	realtimeActive         bool
	realtimeStop           func()
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
		target:        strings.TrimSpace(target),
		tmuxService:   tmuxService,
		interval:      interval,
		inputTracker:  inputTracker,
		autoComplete:  autoComplete,
		realtime:      realtime,
		taskStateSink: taskStateSink,
		logger:        logger,
		subscribers:   map[string]chan protocol.Message{},
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
	p.mu.Lock()
	p.runCtx = ctx
	p.mu.Unlock()
	p.startOnce.Do(func() {
		go p.loop(ctx)
	})
	p.ensureRealtimeSubscribed()
}

func (p *PaneActor) loop(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

func (p *PaneActor) Subscribe(connID string, out chan protocol.Message, opts ...paneSubscribeOptions) {
	if p == nil || out == nil {
		return
	}
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return
	}

	p.mu.Lock()
	p.subscribers[connID] = out
	p.mu.Unlock()

	opt := paneSubscribeOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	snapshot, cursorX, cursorY, hasCursor, err := p.captureResetSnapshotWithOptions(opt)
	if err != nil {
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
	p.ensureRealtimeSubscribed()

	p.emitStatus(snapshot, time.Now().UTC())
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
		snapshot, err := p.tmuxService.CaptureHistory(p.target, lines)
		if err == nil {
			snapshot = normalizeTermSnapshot(snapshot)
			cursorX, cursorY, cursorErr := p.tmuxService.CursorPosition(p.target)
			if cursorErr != nil {
				if isPaneTargetMissingError(cursorErr) {
					return "", 0, 0, false, cursorErr
				}
				return snapshot, 0, 0, false, nil
			}
			return snapshot, cursorX, cursorY, true, nil
		}
		if isPaneTargetMissingError(err) {
			return "", 0, 0, false, err
		}
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
	noSubscribers := len(p.subscribers) == 0
	p.mu.Unlock()
	if noSubscribers {
		p.stopRealtime()
	}
}

func (p *PaneActor) tick(ctx context.Context) {
	snapshot, err := p.tmuxService.CapturePane(p.target)
	if err != nil {
		if isPaneTargetMissingError(err) {
			p.broadcast([]protocol.Message{paneEndedEventMessage(p.target, err.Error())})
		}
		return
	}
	snapshot = normalizeTermSnapshot(snapshot)
	cursorX, cursorY, cursorErr := p.tmuxService.CursorPosition(p.target)
	hasCursor := cursorErr == nil

	p.mu.RLock()
	lastSnapshot := p.lastSnap
	lastCursorX := p.lastCursorX
	lastCursorY := p.lastCursorY
	hasLastCursor := p.hasCursor
	realtimeActive := p.realtimeActive
	p.mu.RUnlock()

	now := time.Now().UTC()
	snapshotChanged := snapshot != lastSnapshot
	cursorChanged := hasCursor && (!hasLastCursor || lastCursorX != cursorX || lastCursorY != cursorY)
	// Fallback diff must stay enabled until realtime subscription is truly active.
	if (snapshotChanged || cursorChanged) && !realtimeActive {
		delta := streamdiff.DecideDelta(lastSnapshot, snapshot, snapshotChanged)
		messages := termOutputMessages(p.target, delta.Mode, delta.Data, cursorX, cursorY, hasCursor)
		p.broadcast(messages)
	}

	p.mu.Lock()
	p.lastSnap = snapshot
	if hasCursor {
		p.lastCursorX = cursorX
		p.lastCursorY = cursorY
		p.hasCursor = true
	}
	p.mu.Unlock()

	p.emitStatus(snapshot, now)
	p.reportTaskState(now, snapshot, cursorX, cursorY, hasCursor)

	select {
	case <-ctx.Done():
		return
	default:
	}
}

func (p *PaneActor) reportTaskState(now time.Time, snapshot string, cursorX, cursorY int, hasCursor bool) {
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
	if provider, ok := p.tmuxService.(paneStatusMetadataProvider); ok {
		_, cmd, err := provider.PaneTitleAndCurrentCommand(p.target)
		if err == nil {
			currentCommand = strings.TrimSpace(cmd)
		}
	}

	p.taskStateSink.OnPaneReport(PaneStateReport{
		PaneID:         p.target,
		PaneTarget:     p.target,
		CurrentCommand: currentCommand,
		RuntimeStatus:  runtimeStatus,
		Snapshot:       snapshot,
		SnapshotHash:   sha1Text(snapshot),
		CursorX:        cursorX,
		CursorY:        cursorY,
		HasCursor:      hasCursor,
		UpdatedAt:      reportAt.Unix(),
	})
}

func (p *PaneActor) captureResetSnapshot() (string, int, int, bool, error) {
	snapshot, err := p.tmuxService.CapturePane(p.target)
	if err != nil {
		if isPaneTargetMissingError(err) {
			return "", 0, 0, false, err
		}
		snapshot, err = p.tmuxService.CaptureHistory(p.target, streamHistoryLines)
		if err != nil {
			return "", 0, 0, false, err
		}
	}
	snapshot = normalizeTermSnapshot(snapshot)
	cursorX, cursorY, cursorErr := p.tmuxService.CursorPosition(p.target)
	if cursorErr != nil {
		if isPaneTargetMissingError(cursorErr) {
			return "", 0, 0, false, cursorErr
		}
		return snapshot, 0, 0, false, nil
	}
	return snapshot, cursorX, cursorY, true, nil
}

func (p *PaneActor) emitStatus(snapshot string, now time.Time) {
	p.mu.Lock()
	prev := p.statusState
	currHash := sha1Text(snapshot)
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
	hook := p.onStatus
	autoProcessArmed := p.autoProcessArmed
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
			Target:   p.target,
			Title:    sessionTitleFromTarget(p.target),
			Status:   nextStatus,
			At:       at,
			Previous: prevStatus,
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
	subscriberCount := len(p.subscribers)
	hasSubscribers := subscriberCount > 0
	p.mu.RUnlock()
	if source == nil || ctx == nil || active || !hasSubscribers {
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
	p.logger.Debug("realtime output subscribed", "pane_target", target, "subscriber_count", subscriberCount)

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
		}
	}
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

func termOutputMessages(target, mode, data string, cursorX, cursorY int, hasCursor bool) []protocol.Message {
	frames := chunkTermData(mode, data)
	out := make([]protocol.Message, 0, len(frames))
	for _, frame := range frames {
		payload := map[string]any{
			"target": target,
			"mode":   frame.Mode,
			"data":   frame.Data,
		}
		if hasCursor {
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
