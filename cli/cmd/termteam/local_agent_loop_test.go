package main

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"termteam/cli/internal/bridge"
	"termteam/cli/internal/turn"
)

type reconnectDialer struct {
	mu    sync.Mutex
	calls int
}

func (d *reconnectDialer) Dial(ctx context.Context, url string) (turn.Socket, error) {
	d.mu.Lock()
	d.calls++
	d.mu.Unlock()
	return &eofSocket{}, nil
}

func (d *reconnectDialer) Calls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

type eofSocket struct{}

func (s *eofSocket) ReadText(ctx context.Context) (string, error) { return "", io.EOF }
func (s *eofSocket) WriteText(ctx context.Context, text string) error {
	return nil
}
func (s *eofSocket) Close() error { return nil }

type noOpTmux struct{}

func (t *noOpTmux) ListSessions() ([]string, error)            { return []string{"botworks:3.0"}, nil }
func (t *noOpTmux) PaneExists(string) (bool, error)            { return true, nil }
func (t *noOpTmux) SelectPane(string) error                    { return nil }
func (t *noOpTmux) SendInput(string, string) error             { return nil }
func (t *noOpTmux) Resize(string, int, int) error              { return nil }
func (t *noOpTmux) CapturePane(string) (string, error)         { return "", nil }
func (t *noOpTmux) CaptureHistory(string, int) (string, error) { return "", nil }
func (t *noOpTmux) StartPipePane(string, string) error         { return nil }
func (t *noOpTmux) StopPipePane(string) error                  { return nil }
func (t *noOpTmux) CursorPosition(string) (int, int, error)    { return 0, 0, nil }
func (t *noOpTmux) CreateSiblingPane(string) (string, error)   { return "botworks:3.1", nil }
func (t *noOpTmux) CreateChildPane(string) (string, error)     { return "botworks:3.2", nil }

var _ bridge.TmuxService = (*noOpTmux)(nil)

func TestStartLocalAgentLoop_ReconnectsUntilContextCanceled(t *testing.T) {
	d := &reconnectDialer{}
	ctx, cancel := context.WithTimeout(context.Background(), 260*time.Millisecond)
	defer cancel()

	err := startLocalAgentLoop(ctx, 4621, d, &noOpTmux{}, nil, nil, testLogger())
	if err != nil {
		t.Fatalf("expected nil when context canceled, got %v", err)
	}
	if d.Calls() < 2 {
		t.Fatalf("expected reconnect attempts >=2, got %d", d.Calls())
	}
}
