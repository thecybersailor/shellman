package main

import (
	"testing"
	"time"
)

func TestSessionTitleFromTarget(t *testing.T) {
	if got := sessionTitleFromTarget("e2e:0.0"); got != "e2e" {
		t.Fatalf("unexpected title: %s", got)
	}
	if got := sessionTitleFromTarget("plain_target"); got != "plain_target" {
		t.Fatalf("unexpected fallback title: %s", got)
	}
}

func TestStatusFromHashes(t *testing.T) {
	if got := statusFromHashes("a", "b"); got != SessionStatusRunning {
		t.Fatalf("expected running, got %s", got)
	}
	if got := statusFromHashes("a", "a"); got != SessionStatusReady {
		t.Fatalf("expected ready, got %s", got)
	}
	if got := statusFromHashes("", "a"); got != SessionStatusUnknown {
		t.Fatalf("expected unknown, got %s", got)
	}
}

func TestAdvancePaneStatus_DelaysTransitionToAvoidFlapping(t *testing.T) {
	delay := 2 * time.Second
	inputIgnore := 1500 * time.Millisecond
	now := time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC)
	state := paneStatusState{}

	state = advancePaneStatus(state, "a", now, time.Time{}, delay, inputIgnore)
	if state.Emitted != SessionStatusRunning {
		t.Fatalf("expected running after first sample, got %s", state.Emitted)
	}

	state = advancePaneStatus(state, "a", now.Add(1*time.Second), time.Time{}, delay, inputIgnore)
	if state.Emitted != SessionStatusRunning {
		t.Fatalf("expected running before ready delay, got %s", state.Emitted)
	}

	state = advancePaneStatus(state, "b", now.Add(2*time.Second), time.Time{}, delay, inputIgnore)
	if state.Emitted != SessionStatusRunning {
		t.Fatalf("expected running when content changes, got %s", state.Emitted)
	}
}

func TestAdvancePaneStatus_IgnoresRecentInputTriggeredChange(t *testing.T) {
	delay := 2 * time.Second
	inputIgnore := 2 * time.Second
	now := time.Date(2026, 2, 18, 12, 0, 0, 0, time.UTC)
	state := paneStatusState{}

	state = advancePaneStatus(state, "a", now, time.Time{}, delay, inputIgnore)
	state = advancePaneStatus(state, "a", now.Add(3*time.Second), time.Time{}, delay, inputIgnore)
	state = advancePaneStatus(state, "a", now.Add(6*time.Second), time.Time{}, delay, inputIgnore)
	if state.Emitted != SessionStatusReady {
		t.Fatalf("expected ready after stable delay, got %s", state.Emitted)
	}

	lastInputAt := now.Add(6500 * time.Millisecond)
	state = advancePaneStatus(state, "b", now.Add(6600*time.Millisecond), lastInputAt, delay, inputIgnore)
	if state.Emitted != SessionStatusReady {
		t.Fatalf("expected ready while within input ignore window, got %s", state.Emitted)
	}
}

func TestAdvancePaneStatus_KeepsSeededLastActiveAtOnFirstSample(t *testing.T) {
	delay := 2 * time.Second
	inputIgnore := 2 * time.Second
	seeded := time.Date(2026, 2, 18, 10, 0, 0, 0, time.UTC)
	now := seeded.Add(30 * time.Minute)
	state := paneStatusState{
		LastActiveAt: seeded,
	}

	state = advancePaneStatus(state, "hash_1", now, time.Time{}, delay, inputIgnore)
	if state.Emitted != SessionStatusRunning {
		t.Fatalf("expected first emitted status running, got %s", state.Emitted)
	}
	if !state.LastActiveAt.Equal(seeded) {
		t.Fatalf("expected seeded last_active_at to be preserved, got %s", state.LastActiveAt.Format(time.RFC3339))
	}
}
