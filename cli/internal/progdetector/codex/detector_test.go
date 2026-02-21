package codex

import (
	"context"
	"testing"
	"time"

	"shellman/cli/internal/progdetector"
)

func TestDetectorBuildInputPromptSteps(t *testing.T) {
	d := New()
	steps, err := d.BuildInputPromptSteps("hello")
	if err != nil {
		t.Fatalf("build prompt steps failed: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("unexpected step count: %d", len(steps))
	}
	if steps[0].Input != "hello" {
		t.Fatalf("unexpected first step input: %q", steps[0].Input)
	}
	if steps[1].Input != "\r" {
		t.Fatalf("unexpected second step input: %q", steps[1].Input)
	}
	if steps[1].Delay != 50*time.Millisecond {
		t.Fatalf("unexpected second step delay: %v", steps[1].Delay)
	}
}

func TestDetectorModeMatchAndExit(t *testing.T) {
	d := New()
	if !d.MatchCurrentCommand("codex --ask") {
		t.Fatal("expected codex command matched")
	}
	exited, err := d.HasExitedMode(context.Background(), progdetector.RuntimeState{CurrentCommand: "zsh"})
	if err != nil {
		t.Fatalf("has exited failed: %v", err)
	}
	if !exited {
		t.Fatal("expected exited=true when current command is zsh")
	}
}
