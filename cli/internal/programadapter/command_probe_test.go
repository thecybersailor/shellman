package programadapter

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

type fakeRunner struct {
	err error
}

func (f fakeRunner) Run(context.Context, string, ...string) error {
	return f.err
}

func TestCommandExists_UsesWhichAndHandlesOutcomes(t *testing.T) {
	ok, err := commandExistsWithRunner(context.Background(), "codex", fakeRunner{err: nil})
	if err != nil || !ok {
		t.Fatalf("expected command exists, got ok=%v err=%v", ok, err)
	}

	ok, err = commandExistsWithRunner(context.Background(), "codex", fakeRunner{err: &exec.ExitError{}})
	if err != nil || ok {
		t.Fatalf("expected command not found without hard error, got ok=%v err=%v", ok, err)
	}
}

func TestCommandExists_ReturnsErrorForUnexpectedRunnerFailure(t *testing.T) {
	wantErr := errors.New("which unavailable")
	ok, err := commandExistsWithRunner(context.Background(), "codex", fakeRunner{err: wantErr})
	if err == nil || !errors.Is(err, wantErr) || ok {
		t.Fatalf("expected hard error with ok=false, got ok=%v err=%v", ok, err)
	}
}
