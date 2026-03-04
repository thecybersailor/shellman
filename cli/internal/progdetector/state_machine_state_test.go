package progdetector

import (
	"context"
	"testing"
)

type stateAwareFakeDetector struct {
	id      string
	match   func(RuntimeState) bool
	exited  func(RuntimeState) bool
	prompts []PromptStep
}

func (f stateAwareFakeDetector) ProgramID() string { return f.id }

func (f stateAwareFakeDetector) IsAvailable(context.Context) (bool, error) { return true, nil }

func (f stateAwareFakeDetector) MatchCurrentCommand(currentCommand string) bool {
	if f.match == nil {
		return false
	}
	return f.match(RuntimeState{CurrentCommand: currentCommand})
}

func (f stateAwareFakeDetector) MatchRuntimeState(state RuntimeState) bool {
	if f.match == nil {
		return false
	}
	return f.match(state)
}

func (f stateAwareFakeDetector) HasExitedMode(_ context.Context, state RuntimeState) (bool, error) {
	if f.exited == nil {
		return true, nil
	}
	return f.exited(state), nil
}

func (f stateAwareFakeDetector) BuildInputPromptSteps(string) ([]PromptStep, error) {
	return append([]PromptStep{}, f.prompts...), nil
}

func TestRegistryDetectByState_UsesRuntimeFields(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(stateAwareFakeDetector{
		id: "cursor",
		match: func(state RuntimeState) bool {
			return state.CurrentBinary == "node" && len(state.CurrentArgs) > 0 && state.CurrentArgs[0] == "cursor-agent"
		},
	}); err != nil {
		t.Fatalf("register detector failed: %v", err)
	}

	got, ok := r.DetectByState(RuntimeState{
		CurrentCommand: "index",
		CurrentBinary:  "node",
		CurrentArgs:    []string{"cursor-agent", "--trust"},
	})
	if !ok || got == nil || got.ProgramID() != "cursor" {
		t.Fatalf("detect by state failed: ok=%v detector=%v", ok, got)
	}
}

func TestResolveActiveAdapterByState_EnterAndExitByRuntimeState(t *testing.T) {
	orig := ProgramDetectorRegistry
	defer func() { ProgramDetectorRegistry = orig }()

	ProgramDetectorRegistry = NewRegistry()
	ProgramDetectorRegistry.MustRegister(stateAwareFakeDetector{
		id: "codex",
		match: func(state RuntimeState) bool {
			return state.CurrentBinary == "node" && len(state.CurrentArgs) > 0 && state.CurrentArgs[0] == "codex"
		},
		exited: func(state RuntimeState) bool {
			return !(state.CurrentBinary == "node" && len(state.CurrentArgs) > 0 && state.CurrentArgs[0] == "codex")
		},
	})

	active := ResolveActiveAdapterByState("", RuntimeState{
		CurrentCommand: "node",
		CurrentBinary:  "node",
		CurrentArgs:    []string{"codex"},
	})
	if active != "codex" {
		t.Fatalf("expected enter codex, got %q", active)
	}

	active = ResolveActiveAdapterByState(active, RuntimeState{
		CurrentCommand: "node",
		CurrentBinary:  "node",
		CurrentArgs:    []string{"codex"},
	})
	if active != "codex" {
		t.Fatalf("expected keep codex, got %q", active)
	}

	active = ResolveActiveAdapterByState(active, RuntimeState{
		CurrentCommand: "zsh",
		CurrentBinary:  "zsh",
		CurrentArgs:    nil,
	})
	if active != "" {
		t.Fatalf("expected clear active adapter after exit, got %q", active)
	}
}
