package main

import "sync"

type activeTarget struct {
	mu      sync.RWMutex
	target  string
	version uint64
}

func newActiveTarget(initial string) *activeTarget {
	v := uint64(0)
	if initial != "" {
		v = 1
	}
	return &activeTarget{
		target:  initial,
		version: v,
	}
}

func (a *activeTarget) Set(target string) {
	if target == "" {
		return
	}
	a.mu.Lock()
	if a.target != target {
		a.version++
	}
	a.target = target
	a.mu.Unlock()
}

// Select marks explicit pane selection intent.
// It always bumps version even when selecting the same target.
func (a *activeTarget) Select(target string) {
	if target == "" {
		return
	}
	a.mu.Lock()
	a.target = target
	a.version++
	a.mu.Unlock()
}

func (a *activeTarget) ClearIf(current string) {
	if current == "" {
		return
	}
	a.mu.Lock()
	if a.target == current {
		a.target = ""
		a.version++
	}
	a.mu.Unlock()
}

func (a *activeTarget) Get() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.target
}

func (a *activeTarget) GetWithVersion() (string, uint64) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.target, a.version
}
