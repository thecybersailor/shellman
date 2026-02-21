package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"termteam/cli/internal/turn"
)

type closedSocket struct{}

func (s *closedSocket) ReadText(ctx context.Context) (string, error) {
	return "", io.EOF
}

func (s *closedSocket) WriteText(ctx context.Context, text string) error {
	return errors.New("use of closed network connection")
}

func (s *closedSocket) Close() error { return nil }

type lifecycleTmux struct{}

func (t *lifecycleTmux) ListSessions() ([]string, error)            { return []string{"e2e:0.0"}, nil }
func (t *lifecycleTmux) PaneExists(string) (bool, error)            { return true, nil }
func (t *lifecycleTmux) SelectPane(string) error                    { return nil }
func (t *lifecycleTmux) SendInput(string, string) error             { return nil }
func (t *lifecycleTmux) Resize(string, int, int) error              { return nil }
func (t *lifecycleTmux) CapturePane(string) (string, error)         { return "", nil }
func (t *lifecycleTmux) CaptureHistory(string, int) (string, error) { return "", nil }
func (t *lifecycleTmux) StartPipePane(string, string) error         { return nil }
func (t *lifecycleTmux) StopPipePane(string) error                  { return nil }
func (t *lifecycleTmux) CursorPosition(string) (int, int, error)    { return 0, 0, nil }
func (t *lifecycleTmux) CreateSiblingPane(string) (string, error)   { return "e2e:0.1", nil }
func (t *lifecycleTmux) CreateChildPane(string) (string, error)     { return "e2e:0.2", nil }

func TestRunWSRuntime_StopsPumpsAfterSocketClose(t *testing.T) {
	oldStatus := statusPumpInterval
	oldStream := streamPumpInterval
	statusPumpInterval = 5 * time.Millisecond
	streamPumpInterval = 5 * time.Millisecond
	defer func() {
		statusPumpInterval = oldStatus
		streamPumpInterval = oldStream
	}()

	wsClient := turn.NewWSClient(&closedSocket{})
	errBuf := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(errBuf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := runWSRuntime(ctx, wsClient, &lifecycleTmux{}, nil, nil, logger)
	if err != nil {
		t.Fatalf("runWSRuntime should return nil on EOF, got %v", err)
	}

	time.Sleep(30 * time.Millisecond)
	if strings.Contains(errBuf.String(), "send tmux.status failed") {
		t.Fatalf("status pump should stop after socket close, got logs: %s", errBuf.String())
	}
}
