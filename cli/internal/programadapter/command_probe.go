package programadapter

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

type commandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type osExecRunner struct{}

func (osExecRunner) Run(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

func CommandExists(ctx context.Context, command string) (bool, error) {
	return commandExistsWithRunner(ctx, command, osExecRunner{})
}

func commandExistsWithRunner(ctx context.Context, command string, runner commandRunner) (bool, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return false, nil
	}
	if runner == nil {
		return false, errors.New("runner is nil")
	}

	err := runner.Run(ctx, "which", command)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}
