package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"termteam/cli/internal/localapi"
)

type changingRegistryTmux struct {
	fakeTmux
	shots []string
	idx   atomic.Int32
}

func (t *changingRegistryTmux) ListSessions() ([]string, error) {
	return []string{"s1"}, nil
}

func (t *changingRegistryTmux) CapturePane(target string) (string, error) {
	if len(t.shots) == 0 {
		return "", nil
	}
	i := int(t.idx.Add(1) - 1)
	if i < 0 {
		i = 0
	}
	if i >= len(t.shots) {
		return t.shots[len(t.shots)-1], nil
	}
	return t.shots[i], nil
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
	tmuxSvc := &changingRegistryTmux{
		shots: []string{"boot$", "run$", "run$", "run$", "run$"},
	}
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
	reg.ConfigureRuntime(ctx, nil, tmuxSvc, nil, autoComplete, nil, 10*time.Millisecond)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if autoCompleteCalls.Load() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected auto-complete to be triggered for unsubscribed pane, got %d", autoCompleteCalls.Load())
}
