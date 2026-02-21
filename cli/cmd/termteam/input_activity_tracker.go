package main

import (
	"sync"
	"time"
)

type inputActivityTracker struct {
	mu   sync.RWMutex
	last map[string]time.Time
}

func newInputActivityTracker() *inputActivityTracker {
	return &inputActivityTracker{
		last: make(map[string]time.Time),
	}
}

func (t *inputActivityTracker) Mark(target string, at time.Time) {
	if t == nil || target == "" {
		return
	}
	t.mu.Lock()
	t.last[target] = at
	t.mu.Unlock()
}

func (t *inputActivityTracker) Last(target string) time.Time {
	if t == nil || target == "" {
		return time.Time{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.last[target]
}

func (t *inputActivityTracker) DeleteMissing(valid map[string]struct{}) {
	if t == nil {
		return
	}
	t.mu.Lock()
	for target := range t.last {
		if _, ok := valid[target]; !ok {
			delete(t.last, target)
		}
	}
	t.mu.Unlock()
}
