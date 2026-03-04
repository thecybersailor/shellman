package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shellman/cli/internal/localapi"
)

type changingRegistryTmux struct {
	fakeTmux
}

func (t *changingRegistryTmux) ListSessions() ([]string, error) {
	return []string{"s1"}, nil
}

type bootstrapRegistryTmux struct {
	streamPumpTmux
	sessions  []string
	listErr   error
	listCalls atomic.Int32
}

func (t *bootstrapRegistryTmux) ListSessions() ([]string, error) {
	t.listCalls.Add(1)
	if t.listErr != nil {
		return nil, t.listErr
	}
	out := make([]string, 0, len(t.sessions))
	for _, target := range t.sessions {
		out = append(out, target)
	}
	return out, nil
}

func (t *bootstrapRegistryTmux) ListCalls() int32 {
	return t.listCalls.Load()
}

type lifecycleRealtimeSource struct {
	countingRealtimeSource
	mu      sync.Mutex
	onClose func(target, reason string)
}

func (s *lifecycleRealtimeSource) SetPaneClosedHook(fn func(target, reason string)) {
	s.mu.Lock()
	s.onClose = fn
	s.mu.Unlock()
}

func (s *lifecycleRealtimeSource) EmitPaneClosed(target, reason string) {
	s.mu.Lock()
	fn := s.onClose
	s.mu.Unlock()
	if fn != nil {
		fn(target, reason)
	}
}

func TestRegistryActor_GetOrCreateConnActor(t *testing.T) {
	reg := NewRegistryActor(testLogger())
	c1 := reg.GetOrCreateConn("conn_1")
	c2 := reg.GetOrCreateConn("conn_1")
	if c1 != c2 {
		t.Fatal("same conn id should reuse same conn actor")
	}
}

func TestRegistryActor_ConfigureRuntime_TracksUnsubscribedPanes(t *testing.T) {
	oldTransitionDelay := statusTransitionDelay
	oldInputIgnoreWindow := statusInputIgnoreWindow
	statusTransitionDelay = 20 * time.Millisecond
	statusInputIgnoreWindow = 0
	defer func() {
		statusTransitionDelay = oldTransitionDelay
		statusInputIgnoreWindow = oldInputIgnoreWindow
	}()

	reg := NewRegistryActor(testLogger())
	tmuxSvc := &changingRegistryTmux{}
	realtime := &countingRealtimeSource{}
	var autoCompleteCalls atomic.Int32
	autoComplete := func(paneTarget string, observedLastActiveAt time.Time) (localapi.AutoCompleteByPaneResult, error) {
		autoCompleteCalls.Add(1)
		return localapi.AutoCompleteByPaneResult{
			Triggered: true,
			Status:    "completed",
			Reason:    "ok",
		}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.ConfigureRuntime(ctx, nil, tmuxSvc, nil, autoComplete, realtime)

	subscribeDeadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(subscribeDeadline) {
		if realtime.SubscribeCalls() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if realtime.SubscribeCalls() == 0 {
		t.Fatal("expected unsubscribed pane actor to establish realtime subscription")
	}

	realtime.Emit("s1", "boot$\n")
	time.Sleep(20 * time.Millisecond)
	realtime.Emit("s1", "run$\n")

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if autoCompleteCalls.Load() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected auto-complete to be triggered for unsubscribed pane, got %d", autoCompleteCalls.Load())
}

func TestRegistryActor_Subscribe_EvictsOldPaneWhenWatchLimitExceeded(t *testing.T) {
	reg := NewRegistryActor(testLogger())
	tmuxSvc := &streamPumpTmux{
		history:       "line\n",
		paneSnapshots: []string{"line\n"},
		cursors:       [][2]int{{0, 0}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.ConfigureRuntime(ctx, nil, tmuxSvc, nil, nil, nil)

	targets := []string{"e2e:0.1", "e2e:0.2", "e2e:0.3", "e2e:0.4", "e2e:0.5", "e2e:0.6"}
	for _, target := range targets {
		reg.Subscribe("conn_1", target)
	}

	conn := reg.GetOrCreateConn("conn_1")
	watched := conn.WatchedTargets()
	wantWatched := []string{"e2e:0.2", "e2e:0.3", "e2e:0.4", "e2e:0.5", "e2e:0.6"}
	if len(watched) != len(wantWatched) {
		t.Fatalf("unexpected watched len, want=%d got=%d watched=%v", len(wantWatched), len(watched), watched)
	}
	for i := range wantWatched {
		if watched[i] != wantWatched[i] {
			t.Fatalf("unexpected watched[%d], want=%q got=%q all=%v", i, wantWatched[i], watched[i], watched)
		}
	}

	reg.mu.Lock()
	evictedPane := reg.panes["e2e:0.1"]
	latestPane := reg.panes["e2e:0.6"]
	reg.mu.Unlock()
	if evictedPane == nil || latestPane == nil {
		t.Fatalf("expected both panes to exist, evicted=%v latest=%v", evictedPane != nil, latestPane != nil)
	}

	evictedPane.mu.RLock()
	_, evictedSubscribed := evictedPane.subscribers["conn_1"]
	evictedPane.mu.RUnlock()
	if evictedSubscribed {
		t.Fatal("expected evicted pane unsubscribe for conn_1")
	}

	latestPane.mu.RLock()
	_, latestSubscribed := latestPane.subscribers["conn_1"]
	latestPane.mu.RUnlock()
	if !latestSubscribed {
		t.Fatal("expected latest pane subscribed for conn_1")
	}
}

func TestRegistryActor_OnPaneClosed_RemovesPaneActorAndWatchers(t *testing.T) {
	reg := NewRegistryActor(testLogger())
	tmuxSvc := &streamPumpTmux{
		history:       "line\n",
		paneSnapshots: []string{"line\n"},
		cursors:       [][2]int{{0, 0}},
	}
	realtime := &lifecycleRealtimeSource{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.ConfigureRuntime(ctx, nil, tmuxSvc, nil, nil, realtime)

	reg.Subscribe("conn_1", "e2e:0.6")
	conn := reg.GetOrCreateConn("conn_1")
	out := conn.OutboundRead()
	select {
	case <-out:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected initial reset frame")
	}

	realtime.EmitPaneClosed("e2e:0.6", "tmux-window-close")

	foundEnded := false
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case msg := <-out:
			if msg.Op == "pane.ended" {
				foundEnded = true
			}
		default:
		}
		if foundEnded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !foundEnded {
		t.Fatal("expected pane.ended after pane close lifecycle event")
	}

	reg.mu.Lock()
	_, stillExists := reg.panes["e2e:0.6"]
	reg.mu.Unlock()
	if stillExists {
		t.Fatal("expected closed pane actor removed from registry")
	}
	if got := conn.Selected(); got != "" {
		t.Fatalf("expected selected target cleared after pane close, got %q", got)
	}
}

func TestRegistryActor_ConfigureRuntime_BootstrapPanesOnlyOnce(t *testing.T) {
	reg := NewRegistryActor(testLogger())
	tmuxSvc := &bootstrapRegistryTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "line\n",
			paneSnapshots: []string{"line\n"},
			cursors:       [][2]int{{0, 0}},
		},
		sessions: []string{"e2e:0.0"},
	}
	realtime := &countingRealtimeSource{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg.ConfigureRuntime(ctx, nil, tmuxSvc, nil, nil, realtime)
	if got := tmuxSvc.ListCalls(); got != 1 {
		t.Fatalf("expected one tmux bootstrap list call, got %d", got)
	}
	time.Sleep(80 * time.Millisecond)
	if got := tmuxSvc.ListCalls(); got != 1 {
		t.Fatalf("expected no background list loop, got list_calls=%d", got)
	}

	reg.ConfigureRuntime(ctx, nil, tmuxSvc, nil, nil, realtime)
	if got := tmuxSvc.ListCalls(); got != 1 {
		t.Fatalf("expected bootstrap list to remain one-time, got list_calls=%d", got)
	}

	reg.mu.Lock()
	_, hasPane := reg.panes["e2e:0.0"]
	reg.mu.Unlock()
	if !hasPane {
		t.Fatal("expected bootstrap to create pane actor for listed target")
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if realtime.SubscribeCalls() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected bootstrapped pane to subscribe realtime output, subscribe_calls=%d", realtime.SubscribeCalls())
}

func TestRegistryActor_ConfigureRuntime_BootstrapErrorNoRetryLoop(t *testing.T) {
	reg := NewRegistryActor(testLogger())
	tmuxSvc := &bootstrapRegistryTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "line\n",
			paneSnapshots: []string{"line\n"},
			cursors:       [][2]int{{0, 0}},
		},
		listErr: errors.New("tmux unavailable"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg.ConfigureRuntime(ctx, nil, tmuxSvc, nil, nil, nil)
	if got := tmuxSvc.ListCalls(); got != 1 {
		t.Fatalf("expected one failed bootstrap list call, got %d", got)
	}
	time.Sleep(80 * time.Millisecond)
	if got := tmuxSvc.ListCalls(); got != 1 {
		t.Fatalf("expected no retry loop after failed bootstrap, got list_calls=%d", got)
	}
}
