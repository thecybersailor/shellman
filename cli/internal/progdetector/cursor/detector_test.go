package cursor

import (
	"context"
	"errors"
	"testing"

	"shellman/cli/internal/progdetector"
)

func TestDetectorMatchCurrentCommand(t *testing.T) {
	d := New()
	if !d.MatchCurrentCommand("cursor-agent --json") {
		t.Fatal("expected cursor-agent command matched")
	}
	if !d.MatchCurrentCommand("cursor --ask") {
		t.Fatal("expected cursor command matched")
	}
	if d.MatchCurrentCommand("zsh") {
		t.Fatal("expected non-cursor command not matched")
	}
}

func TestDetectorMatchRuntimeState_NodeArgsCursorAgent(t *testing.T) {
	d := New()
	if !d.MatchRuntimeState(progdetector.RuntimeState{
		CurrentCommand: "node",
		CurrentBinary:  "node",
		CurrentArgs: []string{
			"/Users/wanglei/.local/bin/cursor-agent",
			"--use-system-ca",
		},
	}) {
		t.Fatal("expected runtime state match for cursor-agent node argv")
	}
}

func TestDetectorHasExitedMode_RuntimeArgsControl(t *testing.T) {
	d := New()
	exited, err := d.HasExitedMode(context.Background(), progdetector.RuntimeState{
		CurrentCommand: "node",
		CurrentBinary:  "node",
		CurrentArgs: []string{
			"/Users/wanglei/.local/bin/cursor-agent",
		},
	})
	if err != nil {
		t.Fatalf("has exited failed: %v", err)
	}
	if exited {
		t.Fatal("expected exited=false when args still point to cursor-agent")
	}

	exited, err = d.HasExitedMode(context.Background(), progdetector.RuntimeState{
		CurrentCommand: "node",
		CurrentBinary:  "node",
		CurrentArgs: []string{
			"/Users/wanglei/.nvm/versions/node/v20.19.0/bin/codex",
		},
	})
	if err != nil {
		t.Fatalf("has exited failed: %v", err)
	}
	if !exited {
		t.Fatal("expected exited=true when node args no longer match cursor-agent")
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
