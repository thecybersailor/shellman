package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"shellman/cli/internal/localapi"
	"shellman/cli/internal/turn"
)

type statusPumpTmux struct {
	mu      sync.Mutex
	lists   [][]string
	listIdx int
	shots   map[string][]string
	shotIdx map[string]int
}

func (s *statusPumpTmux) ListSessions() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.lists) == 0 {
		return []string{}, nil
	}
	if s.listIdx >= len(s.lists) {
		return s.lists[len(s.lists)-1], nil
	}
	out := s.lists[s.listIdx]
	s.listIdx++
	return out, nil
}

func (s *statusPumpTmux) CapturePane(target string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.shotIdx[target]
	arr := s.shots[target]
	if len(arr) == 0 {
		return "", nil
	}
	if idx >= len(arr) {
		return arr[len(arr)-1], nil
	}
	s.shotIdx[target] = idx + 1
	return arr[idx], nil
}

func (s *statusPumpTmux) PaneExists(target string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sessions := range s.lists {
		for _, pane := range sessions {
			if pane == target {
				return true, nil
			}
		}
	}
	if _, ok := s.shots[target]; ok {
		return true, nil
	}
	return false, nil
}

func (s *statusPumpTmux) SelectPane(string) error                    { return nil }
func (s *statusPumpTmux) SendInput(string, string) error             { return nil }
func (s *statusPumpTmux) Resize(string, int, int) error              { return nil }
func (s *statusPumpTmux) CaptureHistory(string, int) (string, error) { return "", nil }
func (s *statusPumpTmux) StartPipePane(string, string) error         { return nil }
func (s *statusPumpTmux) StopPipePane(string) error                  { return nil }
func (s *statusPumpTmux) CursorPosition(string) (int, int, error)    { return 0, 0, nil }
func (s *statusPumpTmux) CreateSiblingPane(string) (string, error)   { return "e2e:0.1", nil }
func (s *statusPumpTmux) CreateChildPane(string) (string, error)     { return "e2e:0.2", nil }
func (s *statusPumpTmux) CreateRootPane() (string, error)            { return "e2e:0.0", nil }
func (s *statusPumpTmux) CreateRootPaneInDir(string) (string, error) { return "e2e:0.0", nil }
func (s *statusPumpTmux) CreateRootPaneInDirLoginShell(string) (string, error) {
	return "e2e:0.0", nil
}
func (s *statusPumpTmux) CreateSiblingPaneInDir(string, string) (string, error) {
	return "e2e:0.1", nil
}
func (s *statusPumpTmux) CreateSiblingPaneInDirLoginShell(string, string) (string, error) {
	return "e2e:0.1", nil
}
func (s *statusPumpTmux) CreateChildPaneInDir(string, string) (string, error) { return "e2e:0.2", nil }
func (s *statusPumpTmux) ClosePane(string) error                                { return nil }

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestStatusPump_Disabled_WaitsContextDoneWithoutPolling(t *testing.T) {
	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &statusPumpTmux{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		runStatusPump(ctx, wsClient, tmuxService, nil, 5*time.Millisecond, newInputActivityTracker(), testLogger())
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	if got := len(sock.writes); got != 0 {
		t.Fatalf("expected no polled tmux.status frames, got %d", got)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected status pump to return after context cancel")
	}
}

func TestStatusPump_LogsFailedAutoCompleteWithStructuredFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	fakeAutoComplete := func(paneTarget string, observedLastActiveAt time.Time) (localapi.AutoCompleteByPaneResult, error) {
		return localapi.AutoCompleteByPaneResult{}, errors.New("trigger failed")
	}

	triggerAutoCompletionByPane(fakeAutoComplete, "botworks:1.0", logger)

	got := buf.String()
	if !strings.Contains(got, `"msg":"auto-complete trigger failed"`) {
		t.Fatalf("expected structured reject log, got %s", got)
	}
	if !strings.Contains(got, `"pane_target":"botworks:1.0"`) {
		t.Fatalf("expected pane_target field, got %s", got)
	}
}

func TestTriggerAutoCompletionByPane_ForwardsPaneTarget(t *testing.T) {
	capturedPaneTarget := ""
	capturedObserved := time.Unix(1, 0).UTC()
	fakeAutoComplete := func(paneTarget string, observedLastActiveAt time.Time) (localapi.AutoCompleteByPaneResult, error) {
		capturedPaneTarget = paneTarget
		capturedObserved = observedLastActiveAt
		return localapi.AutoCompleteByPaneResult{Triggered: true, Status: "completed"}, nil
	}

	triggerAutoCompletionByPane(fakeAutoComplete, "botworks:2.0", testLogger())

	if capturedPaneTarget != "botworks:2.0" {
		t.Fatalf("expected pane_target=botworks:2.0, got %q", capturedPaneTarget)
	}
	if !capturedObserved.IsZero() {
		t.Fatalf("expected observed_last_active_at to be zero, got %v", capturedObserved)
	}
}

func TestTriggerAutoCompletionByPaneWithObservedAt_IncludesObservedTimestamp(t *testing.T) {
	var capturedObserved time.Time
	fakeAutoComplete := func(paneTarget string, observedLastActiveAt time.Time) (localapi.AutoCompleteByPaneResult, error) {
		capturedObserved = observedLastActiveAt
		return localapi.AutoCompleteByPaneResult{Triggered: true, Status: "completed"}, nil
	}

	observedAt := time.Date(2026, 2, 19, 9, 30, 0, 0, time.UTC)
	triggerAutoCompletionByPaneWithObservedAt(fakeAutoComplete, "botworks:3.0", observedAt, testLogger())

	if !capturedObserved.Equal(observedAt) {
		t.Fatalf("expected observed_last_active_at=%d, got %d", observedAt.Unix(), capturedObserved.Unix())
	}
}

func TestBuildTmuxStatusMessages_SplitsLargePayload(t *testing.T) {
	items := make([]sessionStatusItem, 0, 8)
	for i := 0; i < 8; i++ {
		items = append(items, sessionStatusItem{
			Target:         fmt.Sprintf("e2e:%d.0", i),
			Title:          strings.Repeat("x", 800),
			CurrentCommand: "zsh",
			Status:         SessionStatusRunning,
			UpdatedAt:      1771563545,
		})
	}

	msgs, err := buildTmuxStatusMessages(items, 2200, func() time.Time {
		return time.Unix(1771563545, 0).UTC()
	})
	if err != nil {
		t.Fatalf("buildTmuxStatusMessages failed: %v", err)
	}
	if len(msgs) < 2 {
		t.Fatalf("expected split into at least 2 messages, got %d", len(msgs))
	}

	seen := map[string]struct{}{}
	for _, msg := range msgs {
		if msg.Op != "tmux.status" {
			t.Fatalf("expected tmux.status op, got %s", msg.Op)
		}
		var payload struct {
			Items []sessionStatusItem `json:"items"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}
		if len(payload.Items) == 0 {
			t.Fatal("expected each chunk to carry at least one item")
		}
		for _, item := range payload.Items {
			seen[item.Target] = struct{}{}
		}
	}
	if len(seen) != len(items) {
		t.Fatalf("expected all items preserved, seen=%d want=%d", len(seen), len(items))
	}
}
