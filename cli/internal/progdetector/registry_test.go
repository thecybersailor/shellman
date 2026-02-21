package progdetector

import (
	"context"
	"testing"
)

type fakeDetector struct {
	id      string
	matcher func(string) bool
}

func (f fakeDetector) ProgramID() string { return f.id }

func (f fakeDetector) IsAvailable(context.Context) (bool, error) { return true, nil }

func (f fakeDetector) MatchCurrentCommand(currentCommand string) bool {
	if f.matcher == nil {
		return false
	}
	return f.matcher(currentCommand)
}

func (f fakeDetector) HasExitedMode(context.Context, RuntimeState) (bool, error) { return false, nil }

func (f fakeDetector) BuildInputPromptSteps(string) ([]PromptStep, error) { return nil, nil }

func TestRegistryRegisterGetDetect(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fakeDetector{
		id: "codex",
		matcher: func(cmd string) bool {
			return cmd == "codex"
		},
	}); err != nil {
		t.Fatalf("register codex failed: %v", err)
	}
	if err := r.Register(fakeDetector{
		id: "cursor",
		matcher: func(cmd string) bool {
			return cmd == "cursor"
		},
	}); err != nil {
		t.Fatalf("register cursor failed: %v", err)
	}

	got, ok := r.Get("codex")
	if !ok || got.ProgramID() != "codex" {
		t.Fatalf("get codex failed: ok=%v id=%q", ok, got.ProgramID())
	}

	matched, ok := r.DetectByCurrentCommand("cursor")
	if !ok || matched.ProgramID() != "cursor" {
		t.Fatalf("detect cursor failed: ok=%v id=%q", ok, matched.ProgramID())
	}
}

func TestRegistryRejectsDuplicateProgramID(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(fakeDetector{id: "codex"}); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := r.Register(fakeDetector{id: "codex"}); err == nil {
		t.Fatal("expected duplicate id register error")
	}
}
