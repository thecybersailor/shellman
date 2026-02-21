package main

import (
	"strings"
	"sync"
	"time"
)

type autoProgressSuppressionStore struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]autoProgressSuppressionEntry
}

type autoProgressSuppressionEntry struct {
	snapshotHash string
	expiresAt    time.Time
}

var autoProgressSuppression = newAutoProgressSuppressionStore(5 * time.Second)

func newAutoProgressSuppressionStore(ttl time.Duration) *autoProgressSuppressionStore {
	if ttl <= 0 {
		ttl = 5 * time.Second
	}
	return &autoProgressSuppressionStore{
		ttl:     ttl,
		entries: map[string]autoProgressSuppressionEntry{},
	}
}

func registerAutoProgressSuppression(paneTarget, snapshotHash string, hashChanged bool) {
	if !hashChanged {
		return
	}
	paneTarget = strings.TrimSpace(paneTarget)
	snapshotHash = strings.TrimSpace(snapshotHash)
	if paneTarget == "" || snapshotHash == "" {
		return
	}
	autoProgressSuppression.register(paneTarget, snapshotHash)
}

func consumeAutoProgressSuppression(paneTarget, snapshotHash string) bool {
	paneTarget = strings.TrimSpace(paneTarget)
	snapshotHash = strings.TrimSpace(snapshotHash)
	if paneTarget == "" || snapshotHash == "" {
		return false
	}
	return autoProgressSuppression.consumeIfMatch(paneTarget, snapshotHash)
}

func resetAutoProgressSuppressionForTest() {
	autoProgressSuppression.reset()
}

func (s *autoProgressSuppressionStore) register(paneTarget, snapshotHash string) {
	if s == nil {
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked(now)
	s.entries[paneTarget] = autoProgressSuppressionEntry{
		snapshotHash: snapshotHash,
		expiresAt:    now.Add(s.ttl),
	}
}

func (s *autoProgressSuppressionStore) consumeIfMatch(paneTarget, snapshotHash string) bool {
	if s == nil {
		return false
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredLocked(now)
	entry, ok := s.entries[paneTarget]
	if !ok {
		return false
	}
	if strings.TrimSpace(entry.snapshotHash) != snapshotHash {
		return false
	}
	delete(s.entries, paneTarget)
	return true
}

func (s *autoProgressSuppressionStore) reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = map[string]autoProgressSuppressionEntry{}
}

func (s *autoProgressSuppressionStore) evictExpiredLocked(now time.Time) {
	if s == nil {
		return
	}
	for target, entry := range s.entries {
		if entry.expiresAt.IsZero() || entry.expiresAt.After(now) {
			continue
		}
		delete(s.entries, target)
	}
}
