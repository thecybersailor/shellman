package opencode

import (
	"context"
	"errors"
	"testing"

	"shellman/cli/internal/progdetector"
)

func TestDetectorMatchCurrentCommand(t *testing.T) {
	d := New()
	if !d.MatchCurrentCommand("opencode --prompt hello") {
		t.Fatal("expected opencode command matched")
	}
	if d.MatchCurrentCommand("zsh") {
		t.Fatal("expected non-opencode command not matched")
	}
}

func TestDetectorMatchRuntimeState_NodeArgsOpenCode(t *testing.T) {
	d := New()
	if !d.MatchRuntimeState(progdetector.RuntimeState{
		CurrentCommand: "node",
		CurrentBinary:  "node",
		CurrentArgs: []string{
			"/Users/wanglei/.nvm/versions/node/v20.19.0/bin/opencode",
			"--model",
			"gpt-5",
		},
	}) {
		t.Fatal("expected runtime state match for opencode node argv")
	}
}

func TestDetectorHasExitedMode_RuntimeArgsControl(t *testing.T) {
	d := New()
	exited, err := d.HasExitedMode(context.Background(), progdetector.RuntimeState{
		CurrentCommand: "node",
		CurrentBinary:  "node",
		CurrentArgs:    []string{"/Users/wanglei/.nvm/versions/node/v20.19.0/bin/opencode"},
	})
	if err != nil {
		t.Fatalf("has exited failed: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false when args still point to opencode")
	}

	exited, err = d.HasExitedMode(context.Background(), progdetector.RuntimeState{
		CurrentCommand: "node",
		CurrentBinary:  "node",
		CurrentArgs:    []string{"/Users/wanglei/.nvm/versions/node/v20.19.0/bin/codex"},
	})
	if err != nil {
		t.Fatalf("has exited failed: %v", err)
	}
	if !exited {
		t.Fatal("expected exited=true when node args no longer match opencode")
	}
}

func TestDetectorIsAvailable_RespectsCanceledContext(t *testing.T) {
	d := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok, err := d.IsAvailable(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got ok=%v err=%v", ok, err)
	}
	if ok {
		t.Fatalf("expected ok=false when context already canceled")
	}
}

func TestDetectorBuildInputPromptSteps_UsesCarriageReturnSubmit(t *testing.T) {
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
}
