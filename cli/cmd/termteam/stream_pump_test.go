package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"termteam/cli/internal/protocol"
	"termteam/cli/internal/turn"
)

type streamPumpTmux struct {
	history       string
	historyErr    error
	paneSnapshots []string
	paneErr       error
	cursors       [][2]int
	historyLines  int
	idx           int
	cursorIdx     int
	lastTarget    string
}

func (s *streamPumpTmux) ListSessions() ([]string, error) { return []string{"e2e"}, nil }
func (s *streamPumpTmux) PaneExists(target string) (bool, error) {
	s.lastTarget = target
	return true, nil
}
func (s *streamPumpTmux) SelectPane(target string) error { return nil }
func (s *streamPumpTmux) SendInput(target, text string) error {
	return nil
}
func (s *streamPumpTmux) Resize(target string, cols, rows int) error { return nil }
func (s *streamPumpTmux) CapturePane(target string) (string, error) {
	s.lastTarget = target
	if s.paneErr != nil {
		return "", s.paneErr
	}
	if len(s.paneSnapshots) == 0 {
		return "", nil
	}
	if s.idx >= len(s.paneSnapshots) {
		return s.paneSnapshots[len(s.paneSnapshots)-1], nil
	}
	out := s.paneSnapshots[s.idx]
	s.idx++
	return out, nil
}
func (s *streamPumpTmux) CaptureHistory(target string, lines int) (string, error) {
	s.lastTarget = target
	s.historyLines = lines
	if s.historyErr != nil {
		return "", s.historyErr
	}
	return s.history, nil
}
func (s *streamPumpTmux) StartPipePane(target, shellCmd string) error { return nil }
func (s *streamPumpTmux) StopPipePane(target string) error            { return nil }
func (s *streamPumpTmux) CursorPosition(target string) (int, int, error) {
	s.lastTarget = target
	if len(s.cursors) == 0 {
		return 0, 0, nil
	}
	if s.cursorIdx >= len(s.cursors) {
		last := s.cursors[len(s.cursors)-1]
		return last[0], last[1], nil
	}
	out := s.cursors[s.cursorIdx]
	s.cursorIdx++
	return out[0], out[1], nil
}
func (s *streamPumpTmux) CreateSiblingPane(target string) (string, error) { return "e2e:0.1", nil }
func (s *streamPumpTmux) CreateChildPane(target string) (string, error)   { return "e2e:0.2", nil }

func TestStreamPump_SendResetThenAppend(t *testing.T) {
	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &streamPumpTmux{
		history:       "bash-5.3$ ",
		paneSnapshots: []string{"bash-5.3$ ", "bash-5.3$ ls\r\n"},
		cursors:       [][2]int{{0, 0}, {3, 0}},
	}
	target := newActiveTarget("e2e:0.0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runStreamPump(ctx, wsClient, tmuxService, target, 5*time.Millisecond, testLogger())
	time.Sleep(30 * time.Millisecond)
	cancel()

	var outputEvents []map[string]any
	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Op != "term.output" {
			continue
		}
		var payload map[string]any
		_ = json.Unmarshal(msg.Payload, &payload)
		outputEvents = append(outputEvents, payload)
	}

	if len(outputEvents) < 2 {
		t.Fatalf("expected at least 2 term.output events, got %d", len(outputEvents))
	}
	if outputEvents[0]["mode"] != "reset" {
		t.Fatalf("first event should be reset, got %#v", outputEvents[0]["mode"])
	}
	if !strings.Contains(outputEvents[0]["data"].(string), "bash-5.3$") {
		t.Fatalf("unexpected reset data: %#v", outputEvents[0]["data"])
	}
	cursor0, ok := outputEvents[0]["cursor"].(map[string]any)
	if !ok || int(cursor0["x"].(float64)) != 0 || int(cursor0["y"].(float64)) != 0 {
		t.Fatalf("first event cursor mismatch: %#v", outputEvents[0]["cursor"])
	}
	foundAppendWithLS := false
	for _, evt := range outputEvents {
		if evt["mode"] == "append" && strings.Contains(evt["data"].(string), "ls") {
			cursor, ok := evt["cursor"].(map[string]any)
			if !ok || int(cursor["x"].(float64)) != 3 || int(cursor["y"].(float64)) != 0 {
				t.Fatalf("append ls event cursor mismatch: %#v", evt["cursor"])
			}
			foundAppendWithLS = true
			break
		}
	}
	if !foundAppendWithLS {
		t.Fatalf("expected append event containing ls, events=%#v", outputEvents)
	}
	if tmuxService.lastTarget != "e2e:0.0" {
		t.Fatalf("expected target e2e:0.0, got %s", tmuxService.lastTarget)
	}
}

func TestStreamPump_EmitsWhenOnlyCursorMoves(t *testing.T) {
	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &streamPumpTmux{
		history:       "bash-5.3$ ",
		paneSnapshots: []string{"bash-5.3$ ", "bash-5.3$ "},
		cursors:       [][2]int{{0, 0}, {4, 0}},
	}
	target := newActiveTarget("e2e:0.0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runStreamPump(ctx, wsClient, tmuxService, target, 5*time.Millisecond, testLogger())
	time.Sleep(35 * time.Millisecond)
	cancel()

	seen := map[string]bool{}
	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Op != "term.output" {
			continue
		}
		var payload struct {
			Cursor struct {
				X int `json:"x"`
				Y int `json:"y"`
			} `json:"cursor"`
		}
		_ = json.Unmarshal(msg.Payload, &payload)
		seen[fmt.Sprintf("%d,%d", payload.Cursor.X, payload.Cursor.Y)] = true
	}

	if !seen["0,0"] || !seen["4,0"] {
		t.Fatalf("expected cursor updates for 0,0 and 4,0, got %#v", seen)
	}
}

func TestStreamPump_UsesAppendForFullscreenRedraw(t *testing.T) {
	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &streamPumpTmux{
		history:       "Header\nRow A ...\nFooter",
		paneSnapshots: []string{"Header\nRow A ...\nFooter", "Header\nRow B ...\nFooter"},
		cursors:       [][2]int{{0, 0}, {0, 0}},
	}
	target := newActiveTarget("e2e:0.0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runStreamPump(ctx, wsClient, tmuxService, target, 5*time.Millisecond, testLogger())
	time.Sleep(35 * time.Millisecond)
	cancel()

	foundRepaintAppend := false
	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Op != "term.output" {
			continue
		}
		var payload struct {
			Mode string `json:"mode"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			continue
		}
		if payload.Mode == "append" && strings.HasPrefix(payload.Data, "\x1b[0m\x1b[H\x1b[2J") {
			foundRepaintAppend = true
			break
		}
	}
	if !foundRepaintAppend {
		t.Fatal("expected repaint append frame with \\x1b[0m\\x1b[H\\x1b[2J prefix")
	}
}

func TestStreamPump_TargetSwitchResetUsesHistorySnapshot(t *testing.T) {
	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &streamPumpTmux{
		history:       "history_snapshot\n$ ",
		paneSnapshots: []string{"pane_snapshot", "pane_snapshot"},
		cursors:       [][2]int{{3, 1}, {3, 1}},
	}
	target := newActiveTarget("e2e:0.0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runStreamPump(ctx, wsClient, tmuxService, target, 5*time.Millisecond, testLogger())
	time.Sleep(20 * time.Millisecond)
	cancel()

	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Op != "term.output" {
			continue
		}
		var payload struct {
			Mode string `json:"mode"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			continue
		}
		if payload.Mode != "reset" {
			continue
		}
		if payload.Data != "history_snapshot\n$ " {
			t.Fatalf("target switch reset should use history snapshot, got %q", payload.Data)
		}
		if tmuxService.historyLines <= 0 {
			t.Fatalf("expected positive history line request, got %d", tmuxService.historyLines)
		}
		return
	}
	t.Fatal("expected reset term.output event")
}

func TestStreamPump_ReselectSameTargetForcesReset(t *testing.T) {
	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &streamPumpTmux{
		history:       "bash-5.3$ ",
		paneSnapshots: []string{"bash-5.3$ ", "bash-5.3$ ", "bash-5.3$ "},
		cursors:       [][2]int{{0, 0}, {0, 0}, {0, 0}},
	}
	target := newActiveTarget("e2e:0.0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runStreamPump(ctx, wsClient, tmuxService, target, 5*time.Millisecond, testLogger())
	time.Sleep(30 * time.Millisecond)
	target.Select("e2e:0.0")
	time.Sleep(30 * time.Millisecond)
	cancel()

	resetCount := 0
	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Op != "term.output" {
			continue
		}
		var payload struct {
			Mode string `json:"mode"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			continue
		}
		if payload.Mode == "reset" {
			resetCount++
		}
	}
	if resetCount < 2 {
		t.Fatalf("expected at least two reset frames (initial + reselect), got %d", resetCount)
	}
}

func TestSendTermFrame_SplitsOversizedPayload(t *testing.T) {
	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	oversized := strings.Repeat("x", maxTermFrameDataBytes+64)

	sendTermFrame(context.Background(), wsClient, "botworks:3.0", "reset", oversized, 0, 0, true, testLogger())

	if len(sock.writes) != 2 {
		t.Fatalf("expected two ws writes, got %d", len(sock.writes))
	}

	type payloadT struct {
		Mode string `json:"mode"`
		Data string `json:"data"`
	}
	payloads := make([]payloadT, 0, 2)
	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}
		var payload payloadT
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("payload decode failed: %v", err)
		}
		payloads = append(payloads, payload)
	}
	if payloads[0].Mode != "reset" {
		t.Fatalf("first frame should be reset, got %q", payloads[0].Mode)
	}
	if payloads[1].Mode != "append" {
		t.Fatalf("second frame should be append, got %q", payloads[1].Mode)
	}
	if len(payloads[0].Data) != maxTermFrameDataBytes {
		t.Fatalf("first frame len expected %d, got %d", maxTermFrameDataBytes, len(payloads[0].Data))
	}
	if payloads[0].Data+payloads[1].Data != oversized {
		t.Fatal("payload should be split without truncation")
	}
}

func TestStreamPump_ClearsTargetWhenPaneMissing(t *testing.T) {
	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &streamPumpTmux{
		historyErr: fmt.Errorf("exit status 1: can't find window: 4"),
		paneErr:    fmt.Errorf("exit status 1: can't find window: 4"),
	}
	target := newActiveTarget("e2e:4.0")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go runStreamPump(ctx, wsClient, tmuxService, target, 5*time.Millisecond, testLogger())
	time.Sleep(30 * time.Millisecond)
	cancel()

	if got := target.Get(); got != "" {
		t.Fatalf("expected target cleared on missing pane error, got %q", got)
	}
	foundPaneEnded := false
	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.Op == "pane.ended" {
			foundPaneEnded = true
			break
		}
	}
	if !foundPaneEnded {
		t.Fatal("expected pane.ended event when pane is missing")
	}
}
