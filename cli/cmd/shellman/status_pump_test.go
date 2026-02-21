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
	"shellman/cli/internal/protocol"
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

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestStatusPump_EmitsTmuxStatusEvent(t *testing.T) {
	oldDelay := statusTransitionDelay
	oldInputWindow := statusInputIgnoreWindow
	statusTransitionDelay = 10 * time.Millisecond
	statusInputIgnoreWindow = 10 * time.Millisecond
	defer func() {
		statusTransitionDelay = oldDelay
		statusInputIgnoreWindow = oldInputWindow
	}()

	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &statusPumpTmux{
		lists:   [][]string{{"e2e:0.0"}, {"e2e:0.0"}},
		shots:   map[string][]string{"e2e:0.0": {"bash$", "bash$ ls"}},
		shotIdx: map[string]int{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runStatusPump(ctx, wsClient, tmuxService, nil, 5*time.Millisecond, newInputActivityTracker(), testLogger())
	time.Sleep(35 * time.Millisecond)
	cancel()

	found := false
	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Op != "tmux.status" {
			continue
		}
		if strings.Contains(string(msg.Payload), `"target":"e2e:0.0"`) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected tmux.status event")
	}
}

func TestStatusPump_EmitsRunningThenReady(t *testing.T) {
	oldDelay := statusTransitionDelay
	oldInputWindow := statusInputIgnoreWindow
	statusTransitionDelay = 10 * time.Millisecond
	statusInputIgnoreWindow = 10 * time.Millisecond
	defer func() {
		statusTransitionDelay = oldDelay
		statusInputIgnoreWindow = oldInputWindow
	}()

	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &statusPumpTmux{
		lists:   [][]string{{"e2e:0.0"}, {"e2e:0.0"}, {"e2e:0.0"}},
		shots:   map[string][]string{"e2e:0.0": {"bash$", "bash$ ls", "bash$ ls"}},
		shotIdx: map[string]int{},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runStatusPump(ctx, wsClient, tmuxService, nil, 5*time.Millisecond, newInputActivityTracker(), testLogger())
	time.Sleep(50 * time.Millisecond)
	cancel()

	statuses := make([]string, 0)
	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Op != "tmux.status" {
			continue
		}
		var payload struct {
			Items []struct {
				Status string `json:"status"`
			} `json:"items"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			continue
		}
		if len(payload.Items) > 0 {
			statuses = append(statuses, payload.Items[0].Status)
		}
	}
	if len(statuses) < 2 {
		t.Fatalf("expected at least 2 status frames, got %d", len(statuses))
	}
	if statuses[0] != "running" {
		t.Fatalf("expected first status running, got %s", statuses[0])
	}
	foundReady := false
	for _, s := range statuses[1:] {
		if s == "ready" {
			foundReady = true
			break
		}
	}
	if !foundReady {
		t.Fatalf("expected at least one ready status after first frame, got %#v", statuses)
	}
}

func TestStatusPump_UsesDatabaseBaselineOnFirstFrame(t *testing.T) {
	oldDelay := statusTransitionDelay
	oldInputWindow := statusInputIgnoreWindow
	statusTransitionDelay = 10 * time.Millisecond
	statusInputIgnoreWindow = 10 * time.Millisecond
	defer func() {
		statusTransitionDelay = oldDelay
		statusInputIgnoreWindow = oldInputWindow
	}()

	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &statusPumpTmux{
		lists:   [][]string{{"e2e:0.0"}, {"e2e:0.0"}},
		shots:   map[string][]string{"e2e:0.0": {"hash_stable", "hash_stable"}},
		shotIdx: map[string]int{},
	}
	baseline := map[string]paneRuntimeBaseline{
		"e2e:0.0": {
			LastActiveAt:  1771561411,
			RuntimeStatus: SessionStatusReady,
			SnapshotHash:  sha1Text(normalizeTermSnapshot("hash_stable")),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runStatusPump(ctx, wsClient, tmuxService, nil, 5*time.Millisecond, newInputActivityTracker(), testLogger(), baseline)
	time.Sleep(30 * time.Millisecond)
	cancel()

	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Op != "tmux.status" {
			continue
		}
		var payload struct {
			Items []struct {
				Target    string `json:"target"`
				Status    string `json:"status"`
				UpdatedAt int64  `json:"updated_at"`
			} `json:"items"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil || len(payload.Items) == 0 {
			continue
		}
		if payload.Items[0].Target != "e2e:0.0" {
			continue
		}
		if payload.Items[0].Status != "ready" {
			t.Fatalf("expected first frame status ready from baseline, got %s", payload.Items[0].Status)
		}
		if payload.Items[0].UpdatedAt != 1771561411 {
			t.Fatalf("expected baseline updated_at 1771561411, got %d", payload.Items[0].UpdatedAt)
		}
		return
	}
	t.Fatal("expected tmux.status frame for e2e:0.0")
}

func TestStatusPump_InputChangeDoesNotFlipToRunningImmediately(t *testing.T) {
	oldDelay := statusTransitionDelay
	oldInputWindow := statusInputIgnoreWindow
	statusTransitionDelay = 20 * time.Millisecond
	statusInputIgnoreWindow = 200 * time.Millisecond
	defer func() {
		statusTransitionDelay = oldDelay
		statusInputIgnoreWindow = oldInputWindow
	}()

	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &statusPumpTmux{
		lists: [][]string{
			{"e2e:0.0"}, {"e2e:0.0"}, {"e2e:0.0"}, {"e2e:0.0"},
		},
		shots: map[string][]string{
			"e2e:0.0": {"bash$", "bash$", "bash$ l", "bash$ l"},
		},
		shotIdx: map[string]int{},
	}
	inputTracker := newInputActivityTracker()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runStatusPump(ctx, wsClient, tmuxService, nil, 10*time.Millisecond, inputTracker, testLogger())

	time.Sleep(70 * time.Millisecond) // first stabilize to ready
	inputTracker.Mark("e2e:0.0", time.Now())
	time.Sleep(60 * time.Millisecond) // within ignore window
	cancel()

	var lastStatus string
	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Op != "tmux.status" {
			continue
		}
		var payload struct {
			Items []struct {
				Status string `json:"status"`
			} `json:"items"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil || len(payload.Items) == 0 {
			continue
		}
		lastStatus = payload.Items[0].Status
	}
	if lastStatus != "ready" {
		t.Fatalf("expected last status ready after input-echo change, got %s", lastStatus)
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
