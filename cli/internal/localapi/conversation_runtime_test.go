package localapi

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConversationRuntime_SerializesByKey(t *testing.T) {
	rt := NewConversationRuntime(func(ctx context.Context, evt ConversationEvent) error {
		if evt.Key != "k1" {
			t.Fatalf("unexpected key: %s", evt.Key)
		}
		index := evt.Payload.(int)
		if index == 1 {
			time.Sleep(40 * time.Millisecond)
		}
		return nil
	})

	ctx := context.Background()
	if err := rt.Enqueue(ctx, ConversationEvent{Key: "k1", Payload: 1}); err != nil {
		t.Fatalf("enqueue first failed: %v", err)
	}
	if err := rt.Enqueue(ctx, ConversationEvent{Key: "k1", Payload: 2}); err != nil {
		t.Fatalf("enqueue second failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		return rt.QueueLen("k1") == 0
	})
}

func TestConversationRuntime_ParallelAcrossKeys(t *testing.T) {
	var running int32
	var maxRunning int32
	var mu sync.Mutex
	started := map[string]bool{}

	rt := NewConversationRuntime(func(ctx context.Context, evt ConversationEvent) error {
		cur := atomic.AddInt32(&running, 1)
		for {
			prev := atomic.LoadInt32(&maxRunning)
			if cur <= prev || atomic.CompareAndSwapInt32(&maxRunning, prev, cur) {
				break
			}
		}
		mu.Lock()
		started[evt.Key] = true
		mu.Unlock()
		time.Sleep(80 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		return nil
	})

	ctx := context.Background()
	if err := rt.Enqueue(ctx, ConversationEvent{Key: "a", Payload: ""}); err != nil {
		t.Fatalf("enqueue a failed: %v", err)
	}
	if err := rt.Enqueue(ctx, ConversationEvent{Key: "b", Payload: ""}); err != nil {
		t.Fatalf("enqueue b failed: %v", err)
	}

	waitUntil(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return started["a"] && started["b"]
	})
	if atomic.LoadInt32(&maxRunning) < 2 {
		t.Fatalf("expected concurrent execution across keys, maxRunning=%d", maxRunning)
	}
}

func TestConversationRuntime_CancelInflight(t *testing.T) {
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)

	rt := NewConversationRuntime(func(ctx context.Context, evt ConversationEvent) error {
		started <- struct{}{}
		<-ctx.Done()
		canceled <- struct{}{}
		return ctx.Err()
	})

	if err := rt.Enqueue(context.Background(), ConversationEvent{Key: "cancel-key", Payload: nil}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting handler start")
	}

	rt.Cancel("cancel-key")

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting cancellation")
	}
}

func waitUntil(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}
