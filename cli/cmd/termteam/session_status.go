package main

import (
	"strings"
	"time"
)

type SessionStatus string

const (
	SessionStatusRunning SessionStatus = "running"
	SessionStatusReady   SessionStatus = "ready"
	SessionStatusUnknown SessionStatus = "unknown"
)

func normalizeSessionStatus(status SessionStatus) SessionStatus {
	if status == "" {
		return SessionStatusUnknown
	}
	return status
}

func sessionTitleFromTarget(target string) string {
	idx := strings.Index(target, ":")
	if idx <= 0 {
		return target
	}
	return target[:idx]
}

func statusFromHashes(prevHash, currHash string) SessionStatus {
	if prevHash == "" || currHash == "" {
		return SessionStatusUnknown
	}
	if prevHash != currHash {
		return SessionStatusRunning
	}
	return SessionStatusReady
}

type paneStatusState struct {
	PrevHash       string
	Emitted        SessionStatus
	Candidate      SessionStatus
	CandidateSince time.Time
	LastActiveAt   time.Time
}

func advancePaneStatus(
	state paneStatusState,
	currHash string,
	now time.Time,
	lastInputAt time.Time,
	transitionDelay time.Duration,
	inputIgnoreWindow time.Duration,
) paneStatusState {
	if currHash == "" {
		state.PrevHash = ""
		state.Emitted = SessionStatusUnknown
		state.Candidate = SessionStatusUnknown
		state.CandidateSince = now
		if state.LastActiveAt.IsZero() {
			state.LastActiveAt = now
		}
		return state
	}

	if state.PrevHash == "" {
		if state.LastActiveAt.IsZero() {
			state.LastActiveAt = now
		}
	} else if state.PrevHash != currHash {
		state.LastActiveAt = now
	}
	if state.LastActiveAt.IsZero() {
		state.LastActiveAt = now
	}

	observed := statusFromHashes(state.PrevHash, currHash)
	if state.PrevHash == "" {
		observed = SessionStatusRunning
	}

	if observed == SessionStatusRunning && state.Emitted != SessionStatusRunning && !lastInputAt.IsZero() {
		if now.Sub(lastInputAt) < inputIgnoreWindow {
			observed = state.Emitted
			if observed == "" {
				observed = SessionStatusReady
			}
		}
	}

	if state.Emitted == "" {
		state.Emitted = observed
		state.Candidate = observed
		state.CandidateSince = now
		state.PrevHash = currHash
		return state
	}

	if observed == state.Emitted {
		state.Candidate = observed
		state.CandidateSince = now
		state.PrevHash = currHash
		return state
	}

	if state.Candidate != observed {
		state.Candidate = observed
		state.CandidateSince = now
		state.PrevHash = currHash
		return state
	}

	if transitionDelay <= 0 || now.Sub(state.CandidateSince) >= transitionDelay {
		state.Emitted = observed
		state.Candidate = observed
		state.CandidateSince = now
	}
	state.PrevHash = currHash
	return state
}
