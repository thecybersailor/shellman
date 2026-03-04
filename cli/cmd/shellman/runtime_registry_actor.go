package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"shellman/cli/internal/bridge"
	"shellman/cli/internal/protocol"
	"shellman/cli/internal/turn"
)

type RegistryActor struct {
	logger *slog.Logger

	mu        sync.Mutex
	conns     map[string]*ConnActor
	panes     map[string]*PaneActor
	paneItems map[string]sessionStatusItem
	connLoops map[string]struct{}
	paneBootstrapDone bool

	runtimeCtx    context.Context
	wsClient      *turn.WSClient
	tmuxService   bridge.TmuxService
	inputTracker  *inputActivityTracker
	autoComplete  paneAutoCompletionExecutor
	outputSource  paneOutputRealtimeSource
	taskStateSink TaskStateSink

	paneRuntimeBaseline map[string]paneRuntimeBaseline
}

const activePaneWatchLimit = 5

func NewRegistryActor(logger *slog.Logger) *RegistryActor {
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	return &RegistryActor{
		logger:    logger,
		conns:     map[string]*ConnActor{},
		panes:     map[string]*PaneActor{},
		paneItems: map[string]sessionStatusItem{},
		connLoops: map[string]struct{}{},
	}
}

func (r *RegistryActor) ConfigureRuntime(
	ctx context.Context,
	wsClient *turn.WSClient,
	tmuxService bridge.TmuxService,
	inputTracker *inputActivityTracker,
	autoComplete paneAutoCompletionExecutor,
	outputSource paneOutputRealtimeSource,
	taskStateSinkOpt ...TaskStateSink,
) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.runtimeCtx = ctx
	r.wsClient = wsClient
	r.tmuxService = tmuxService
	r.inputTracker = inputTracker
	r.autoComplete = autoComplete
	r.outputSource = outputSource
	if lifecycleSource, ok := outputSource.(paneOutputLifecycleSource); ok {
		lifecycleSource.SetPaneClosedHook(r.onPaneClosed)
	}
	if len(taskStateSinkOpt) > 0 {
		r.taskStateSink = taskStateSinkOpt[0]
	}
	for _, conn := range r.conns {
		r.startConnLoopLocked(conn)
	}
	for _, pane := range r.panes {
		pane.Start(ctx)
	}
	shouldBootstrap := !r.paneBootstrapDone && r.runtimeCtx != nil && r.tmuxService != nil
	if shouldBootstrap {
		r.paneBootstrapDone = true
	}
	r.mu.Unlock()
	if shouldBootstrap {
		r.bootstrapPanesFromTmux()
		r.emitTmuxStatus()
	}
}

func (r *RegistryActor) SetPaneRuntimeBaseline(baseline map[string]paneRuntimeBaseline) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(baseline) == 0 {
		r.paneRuntimeBaseline = nil
		return
	}
	next := make(map[string]paneRuntimeBaseline, len(baseline))
	for k, v := range baseline {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		next[key] = v
	}
	r.paneRuntimeBaseline = next
}

func (r *RegistryActor) GetOrCreateConn(connID string) *ConnActor {
	if r == nil {
		return nil
	}
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.conns[connID]; ok {
		return c
	}
	c := NewConnActor(connID)
	r.conns[connID] = c
	r.startConnLoopLocked(c)
	return c
}

func (r *RegistryActor) Conn(connID string) *ConnActor {
	if r == nil {
		return nil
	}
	connID = strings.TrimSpace(connID)
	if connID == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.conns[connID]
}

func (r *RegistryActor) GetOrCreatePane(target string) *PaneActor {
	if r == nil {
		return nil
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getOrCreatePaneLocked(target)
}

func (r *RegistryActor) Subscribe(connID, target string, opts ...paneSubscribeOptions) {
	connID = strings.TrimSpace(connID)
	target = strings.TrimSpace(target)
	if connID == "" || target == "" {
		return
	}

	conn := r.GetOrCreateConn(connID)
	if conn == nil {
		return
	}

	evictedTarget := conn.SelectAndWatch(target, activePaneWatchLimit)
	pane := r.GetOrCreatePane(target)
	subscribeOpt := paneSubscribeOptions{}
	if len(opts) > 0 {
		subscribeOpt = opts[0]
	}
	if pane != nil {
		pane.Subscribe(connID, conn.Outbound(), subscribeOpt)
	}
	if evictedTarget != "" && evictedTarget != target {
		r.mu.Lock()
		oldPane := r.panes[evictedTarget]
		r.mu.Unlock()
		if oldPane != nil {
			oldPane.Unsubscribe(connID)
		}
	}
}

func (r *RegistryActor) getOrCreatePaneLocked(target string) *PaneActor {
	if pane, ok := r.panes[target]; ok {
		return pane
	}
	if r.tmuxService == nil {
		return nil
	}
	pane := NewPaneActor(
		target,
		r.tmuxService,
		streamPumpInterval,
		r.inputTracker,
		r.autoComplete,
		r.outputSource,
		r.logger.With("module", "pane_actor", "pane_target", target),
		r.taskStateSink,
	)
	pane.SetStatusHook(r.onPaneStatusUpdate)
	if r.paneRuntimeBaseline != nil {
		if baseline, ok := r.paneRuntimeBaseline[target]; ok {
			pane.SetRuntimeBaseline(baseline)
		}
	}
	if r.runtimeCtx != nil {
		pane.Start(r.runtimeCtx)
	}
	r.panes[target] = pane
	r.seedPaneStatusLocked(target)
	return pane
}

func (r *RegistryActor) startConnLoopLocked(conn *ConnActor) {
	if conn == nil || r.runtimeCtx == nil || r.wsClient == nil {
		return
	}
	connID := conn.ID()
	if connID == "" {
		return
	}
	if _, ok := r.connLoops[connID]; ok {
		return
	}
	r.connLoops[connID] = struct{}{}

	ctx := r.runtimeCtx
	wsClient := r.wsClient
	logger := r.logger.With("module", "conn_actor", "conn_id", connID)
	outbound := conn.OutboundRead()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-outbound:
				if !ok {
					return
				}
				termTarget, termMode, termDataLen := decodeTermOutputMeta(msg)
				raw, err := json.Marshal(msg)
				if err != nil {
					logger.Error("marshal conn outbound failed", "op", msg.Op, "err", err)
					continue
				}
				wrapped, err := protocol.WrapMuxEnvelope(connID, raw)
				if err != nil {
					logger.Error("wrap conn outbound failed", "op", msg.Op, "err", err)
					continue
				}
				if err := wsClient.Send(context.Background(), string(wrapped)); err != nil {
					if msg.Op == "term.output" {
						logger.Warn("send conn outbound failed", "op", msg.Op, "target", termTarget, "mode", termMode, "data_len", termDataLen, "pending_after_send_fail", len(outbound), "err", err)
					} else {
						logger.Warn("send conn outbound failed", "op", msg.Op, "err", err)
					}
					continue
				}
			}
		}
	}()
}

func (r *RegistryActor) bootstrapPanesFromTmux() {
	if r == nil {
		return
	}
	r.mu.Lock()
	tmuxService := r.tmuxService
	logger := r.logger
	r.mu.Unlock()
	if tmuxService == nil {
		return
	}
	targets, err := tmuxService.ListSessions()
	if err != nil {
		log := logger
		if log == nil {
			log = newRuntimeLogger(io.Discard)
		}
		log = log.With("module", "pane_bootstrap")
		log.Warn("bootstrap panes failed", "err", err)
		return
	}
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		r.GetOrCreatePane(target)
	}
}

func (r *RegistryActor) seedPaneStatusLocked(target string) {
	if r == nil {
		return
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	if _, ok := r.paneItems[target]; ok {
		return
	}
	r.paneItems[target] = sessionStatusItem{
		Target:    target,
		Title:     sessionTitleFromTarget(target),
		Status:    SessionStatusUnknown,
		UpdatedAt: time.Now().UTC().Unix(),
	}
}

func (r *RegistryActor) onPaneStatusUpdate(update paneStatusUpdate) {
	if r == nil {
		return
	}
	target := strings.TrimSpace(update.Target)
	if target == "" {
		return
	}
	title := strings.TrimSpace(update.Title)
	if title == "" {
		title = sessionTitleFromTarget(target)
	}
	status := normalizeSessionStatus(update.Status)
	command := strings.TrimSpace(update.CurrentCommand)
	updatedAt := update.At.UTC().Unix()
	if update.At.IsZero() {
		updatedAt = time.Now().UTC().Unix()
	}

	shouldEmit := false
	r.mu.Lock()
	prev, hasPrev := r.paneItems[target]
	if command == "" {
		command = strings.TrimSpace(prev.CurrentCommand)
	}
	item := sessionStatusItem{
		Target:         target,
		Title:          title,
		CurrentCommand: command,
		Status:         status,
		UpdatedAt:      updatedAt,
	}
	r.paneItems[target] = item
	if !hasPrev ||
		prev.Status != item.Status ||
		strings.TrimSpace(prev.Title) != strings.TrimSpace(item.Title) ||
		strings.TrimSpace(prev.CurrentCommand) != strings.TrimSpace(item.CurrentCommand) {
		shouldEmit = true
	}
	r.mu.Unlock()
	if shouldEmit {
		r.emitTmuxStatus()
	}
}

func (r *RegistryActor) emitTmuxStatus() {
	if r == nil {
		return
	}
	r.mu.Lock()
	wsClient := r.wsClient
	ctx := r.runtimeCtx
	items := make([]sessionStatusItem, 0, len(r.paneItems))
	for _, item := range r.paneItems {
		items = append(items, item)
	}
	r.mu.Unlock()
	if wsClient == nil || ctx == nil {
		return
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.TrimSpace(items[i].Target) < strings.TrimSpace(items[j].Target)
	})
	msgs, err := buildTmuxStatusMessages(items, tmuxStatusMaxMessageBytes, time.Now)
	if err != nil {
		r.logger.Error("build tmux.status from pane actor failed", "err", err)
		return
	}
	for _, msg := range msgs {
		raw, err := json.Marshal(msg)
		if err != nil {
			r.logger.Error("marshal tmux.status from pane actor failed", "err", err)
			return
		}
		if err := wsClient.Send(ctx, string(raw)); err != nil {
			r.logger.Warn("send tmux.status from pane actor failed", "err", err)
			return
		}
	}
}

func (r *RegistryActor) onPaneClosed(target, reason string) {
	if r == nil {
		return
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "pane closed"
	}

	r.mu.Lock()
	pane, hadPane := r.panes[target]
	_, hadItem := r.paneItems[target]
	delete(r.panes, target)
	delete(r.paneItems, target)
	conns := make([]*ConnActor, 0, len(r.conns))
	connIDs := make([]string, 0, len(r.conns))
	for connID, conn := range r.conns {
		if conn == nil {
			continue
		}
		conns = append(conns, conn)
		connIDs = append(connIDs, connID)
	}
	r.mu.Unlock()

	if !hadPane && !hadItem {
		return
	}
	if pane != nil {
		pane.NotifyEnded(reason)
		for _, connID := range connIDs {
			pane.Unsubscribe(connID)
		}
	}
	for _, conn := range conns {
		conn.DropWatch(target)
	}
	r.emitTmuxStatus()
}
