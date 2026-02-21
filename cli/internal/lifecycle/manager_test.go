package lifecycle

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestManager_ContextCancelRunsShutdown(t *testing.T) {
	mgr := NewManager()
	steps := make([]string, 0, 4)
	var mu sync.Mutex
	appendStep := func(v string) {
		mu.Lock()
		steps = append(steps, v)
		mu.Unlock()
	}

	mgr.AddRun("http", func(ctx context.Context) error {
		<-ctx.Done()
		appendStep("run-http-stopped")
		return nil
	})
	mgr.AddShutdown("close-db", func(context.Context) error {
		appendStep("shutdown-db")
		return nil
	})

	parent, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- mgr.StartAndWait(parent)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("StartAndWait should not fail: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(steps, "run-http-stopped") {
		t.Fatalf("missing run stop marker: %#v", steps)
	}
	if !slices.Contains(steps, "shutdown-db") {
		t.Fatalf("missing shutdown marker: %#v", steps)
	}
}

func TestManager_RunErrorTriggersShutdown(t *testing.T) {
	mgr := NewManager()
	runErr := errors.New("boom")
	shutdownCalled := 0

	mgr.AddRun("http", func(context.Context) error {
		return runErr
	})
	mgr.AddShutdown("close-db", func(context.Context) error {
		shutdownCalled++
		return nil
	})

	err := mgr.StartAndWait(context.Background())
	if !errors.Is(err, runErr) {
		t.Fatalf("expected run error, got %v", err)
	}
	if shutdownCalled != 1 {
		t.Fatalf("expected shutdown called once, got %d", shutdownCalled)
	}
}
