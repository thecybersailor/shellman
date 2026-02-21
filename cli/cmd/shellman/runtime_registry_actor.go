package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
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
	connLoops map[string]struct{}
	discovery bool

	runtimeCtx    context.Context
	wsClient      *turn.WSClient
	tmuxService   bridge.TmuxService
	inputTracker  *inputActivityTracker
	autoComplete  paneAutoCompletionExecutor
	outputSource  paneOutputRealtimeSource
	paneInterval  time.Duration
	taskStateSink TaskStateSink

	paneRuntimeBaseline map[string]paneRuntimeBaseline
}

func NewRegistryActor(logger *slog.Logger) *RegistryActor {
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	return &RegistryActor{
		logger:    logger,
		conns:     map[string]*ConnActor{},
		panes:     map[string]*PaneActor{},
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
	paneInterval time.Duration,
	taskStateSinkOpt ...TaskStateSink,
) {
	if r == nil {
		return
	}
	if paneInterval <= 0 {
		paneInterval = streamPumpInterval
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runtimeCtx = ctx
	r.wsClient = wsClient
	r.tmuxService = tmuxService
	r.inputTracker = inputTracker
	r.autoComplete = autoComplete
	r.outputSource = outputSource
	r.paneInterval = paneInterval
	if len(taskStateSinkOpt) > 0 {
		r.taskStateSink = taskStateSinkOpt[0]
	}
	for _, conn := range r.conns {
		r.startConnLoopLocked(conn)
	}
	for _, pane := range r.panes {
		pane.Start(ctx)
	}
	r.startPaneDiscoveryLoopLocked()
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

func (r *RegistryActor) Subscribe(connID, target string) {
	connID = strings.TrimSpace(connID)
	target = strings.TrimSpace(target)
	if connID == "" || target == "" {
		return
	}

	conn := r.GetOrCreateConn(connID)
	if conn == nil {
		return
	}

	prevTarget := strings.TrimSpace(conn.Selected())
	if prevTarget != "" && prevTarget != target {
		r.mu.Lock()
		oldPane := r.panes[prevTarget]
		r.mu.Unlock()
		if oldPane != nil {
			oldPane.Unsubscribe(connID)
		}
	}

	pane := r.GetOrCreatePane(target)
	conn.Select(target)
	if pane != nil {
		pane.Subscribe(connID, conn.Outbound())
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
		r.paneInterval,
		r.inputTracker,
		r.autoComplete,
		r.outputSource,
		r.logger.With("module", "pane_actor", "pane_target", target),
		r.taskStateSink,
	)
	if r.paneRuntimeBaseline != nil {
		if baseline, ok := r.paneRuntimeBaseline[target]; ok {
			pane.SetRuntimeBaseline(baseline)
		}
	}
	if r.runtimeCtx != nil {
		pane.Start(r.runtimeCtx)
	}
	r.panes[target] = pane
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

func (r *RegistryActor) startPaneDiscoveryLoopLocked() {
	if r == nil || r.discovery || r.runtimeCtx == nil || r.tmuxService == nil {
		return
	}
	r.discovery = true
	ctx := r.runtimeCtx
	interval := r.paneInterval
	logger := r.logger.With("module", "pane_discovery")
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	go func() {
		r.discoverPanesOnce(logger)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.discoverPanesOnce(logger)
			}
		}
	}()
}

func (r *RegistryActor) discoverPanesOnce(logger *slog.Logger) {
	if r == nil {
		return
	}
	r.mu.Lock()
	tmuxService := r.tmuxService
	r.mu.Unlock()
	if tmuxService == nil {
		return
	}
	targets, err := tmuxService.ListSessions()
	if err != nil {
		if logger != nil {
			logger.Warn("discover panes failed", "err", err)
		}
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
