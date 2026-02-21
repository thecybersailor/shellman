package localapi

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	shellReadyPaneOptionKey   = "@shellman_ready"
	shellReadyPaneOptionValue = "1"
)

var (
	shellReadyWaitTimeout  = 8 * time.Second
	shellReadyPollInterval = 120 * time.Millisecond
)

func (s *Server) waitPaneShellReady(ctx context.Context, paneTarget string) (bool, error) {
	target := strings.TrimSpace(paneTarget)
	if target == "" {
		return false, nil
	}
	timeout := shellReadyWaitTimeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	poll := shellReadyPollInterval
	if poll <= 0 {
		poll = 120 * time.Millisecond
	}

	deadline := time.Now().Add(timeout)
	for {
		readyValue, err := s.readPaneOptionValue(ctx, target, shellReadyPaneOptionKey)
		if err != nil {
			return false, err
		}
		if strings.EqualFold(strings.TrimSpace(readyValue), shellReadyPaneOptionValue) {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(poll):
		}
	}
}

func (s *Server) readPaneOptionValue(ctx context.Context, paneTarget, key string) (string, error) {
	target := strings.TrimSpace(paneTarget)
	key = strings.TrimSpace(key)
	if target == "" || key == "" {
		return "", nil
	}
	execute := s.deps.ExecuteCommand
	if execute == nil {
		execute = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		}
	}
	execCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	args := []string{}
	if socket := strings.TrimSpace(os.Getenv("SHELLMAN_TMUX_SOCKET")); socket != "" {
		args = append(args, "-L", socket)
	}
	format := "#{@" + strings.TrimPrefix(key, "@") + "}"
	args = append(args, "display-message", "-p", "-t", target, format)
	out, err := execute(execCtx, "tmux", args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
