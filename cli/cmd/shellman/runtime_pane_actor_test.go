package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shellman/cli/internal/localapi"
	"shellman/cli/internal/protocol"
	"shellman/cli/internal/turn"
)

type fakePaneRealtimeSource struct {
	streams map[string]chan string
}

func (f *fakePaneRealtimeSource) Subscribe(target string) (<-chan string, func(), error) {
	ch := f.streams[target]
	if ch == nil {
		ch = make(chan string, 8)
		if f.streams == nil {
			f.streams = map[string]chan string{}
		}
		f.streams[target] = ch
	}
	return ch, func() {}, nil
}

type delayedCaptureTmux struct {
	streamPumpTmux
	captureDelay time.Duration
}

func (d *delayedCaptureTmux) CaptureHistory(target string, lines int) (string, error) {
	if d.captureDelay > 0 {
		time.Sleep(d.captureDelay)
	}
	return d.streamPumpTmux.CaptureHistory(target, lines)
}

func readNextTermOutput(t *testing.T, ch <-chan protocol.Message, timeout time.Duration) protocol.Message {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-ch:
			if msg.Op == "term.output" {
				return msg
			}
		case <-deadline:
			t.Fatal("timeout waiting term.output")
		}
	}
}

type eagerRealtimeSource struct {
	streams map[string]chan string
}

func (e *eagerRealtimeSource) Subscribe(target string) (<-chan string, func(), error) {
	ch := e.streams[target]
	if ch == nil {
		ch = make(chan string, 8)
		if e.streams == nil {
			e.streams = map[string]chan string{}
		}
		e.streams[target] = ch
	}
	go func() {
		ch <- "APPEND\n"
	}()
	return ch, func() {}, nil
}

type fakeTaskStateSink struct {
	calls atomic.Int32
	mu    sync.Mutex
	last  PaneStateReport
}

func (f *fakeTaskStateSink) OnPaneReport(r PaneStateReport) {
	f.calls.Add(1)
	f.mu.Lock()
	f.last = r
	f.mu.Unlock()
}

func (f *fakeTaskStateSink) Last() PaneStateReport {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

type commandAwareTmux struct {
	streamPumpTmux
	title   string
	command string
	mu      sync.Mutex
	inputs  []string
	options map[string]string
}

func (t *commandAwareTmux) PaneTitleAndCurrentCommand(target string) (string, string, error) {
	return t.title, t.command, nil
}

func (t *commandAwareTmux) SendInput(target, text string) error {
	t.mu.Lock()
	t.inputs = append(t.inputs, text)
	t.mu.Unlock()
	return nil
}

func (t *commandAwareTmux) InputCalls() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.inputs))
	copy(out, t.inputs)
	return out
}

func (t *commandAwareTmux) GetPaneOption(target, key string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.options == nil {
		return "", nil
	}
	return strings.TrimSpace(t.options[key]), nil
}

func (t *commandAwareTmux) SetOptions(options map[string]string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.options = map[string]string{}
	for k, v := range options {
		t.options[k] = v
	}
}

func TestPaneActor_SelectSubscriberGetsResetFrame(t *testing.T) {
	tmuxService := &streamPumpTmux{
		history:       "hello\n",
		paneSnapshots: []string{"hello\n"},
		cursors:       [][2]int{{6, 0}},
	}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	out := make(chan protocol.Message, 8)
	actor.Subscribe("conn_1", out)

	select {
	case msg := <-out:
		if msg.Op != "term.output" {
			t.Fatalf("expected term.output, got %s", msg.Op)
		}
		var payload struct {
			Target string `json:"target"`
			Mode   string `json:"mode"`
			Data   string `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}
		if payload.Target != "e2e:0.0" {
			t.Fatalf("expected target e2e:0.0, got %s", payload.Target)
		}
		if payload.Mode != "reset" {
			t.Fatalf("expected mode reset, got %s", payload.Mode)
		}
		if !strings.Contains(payload.Data, "hello") {
			t.Fatalf("unexpected reset data: %q", payload.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("expected reset frame for first subscribe")
	}
}

func TestPaneActor_SubscribePrefersVisiblePaneSnapshotOverHistory(t *testing.T) {
	tmuxService := &streamPumpTmux{
		history:       "history_line_1\nhistory_line_2\n",
		paneSnapshots: []string{"visible_line_only\n"},
		cursors:       [][2]int{{0, 0}},
	}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	out := make(chan protocol.Message, 8)
	actor.Subscribe("conn_1", out)

	select {
	case msg := <-out:
		if msg.Op != "term.output" {
			t.Fatalf("expected term.output, got %s", msg.Op)
		}
		var payload struct {
			Mode string `json:"mode"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}
		if payload.Mode != "reset" {
			t.Fatalf("expected mode reset, got %s", payload.Mode)
		}
		if payload.Data != "visible_line_only" {
			t.Fatalf("expected reset data from visible pane snapshot (trailing newline stripped), got %q", payload.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("expected reset frame for subscribe")
	}
}

func TestPaneActor_SubscribeMissingPaneEmitsPaneEndedEvenIfHistoryExists(t *testing.T) {
	tmuxService := &streamPumpTmux{
		history:       "stale_history_should_not_be_used\n",
		historyErr:    nil,
		paneSnapshots: []string{},
		paneErr:       errors.New("can't find pane"),
	}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	out := make(chan protocol.Message, 8)
	actor.Subscribe("conn_1", out)

	select {
	case msg := <-out:
		if msg.Op != "pane.ended" {
			t.Fatalf("expected pane.ended, got %s", msg.Op)
		}
		var payload struct {
			Target string `json:"target"`
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}
		if payload.Target != "e2e:0.0" {
			t.Fatalf("unexpected ended target: %q", payload.Target)
		}
		if !strings.Contains(payload.Reason, "can't find pane") {
			t.Fatalf("unexpected ended reason: %q", payload.Reason)
		}
	case <-time.After(time.Second):
		t.Fatal("expected pane.ended event for missing pane")
	}
}

func TestPaneActor_Subscribe_BuffersAppendUntilResetSent(t *testing.T) {
	tmuxService := &delayedCaptureTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "BASE\nAPPEND\n",
			paneSnapshots: []string{"BASE\n", "BASE\nAPPEND\n", "BASE\nAPPEND\n"},
			cursors:       [][2]int{{0, 1}, {0, 1}, {0, 1}},
		},
		captureDelay: 45 * time.Millisecond,
	}
	actor := NewPaneActor("e2e:0.0", tmuxService, 10*time.Millisecond, nil, nil, nil, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	// Seed last snapshot before subscribe so tick emits append-delta during delayed capture.
	time.Sleep(15 * time.Millisecond)

	out := make(chan protocol.Message, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		actor.Subscribe("conn_1", out, paneSubscribeOptions{
			GapRecover:   true,
			HistoryLines: 4000,
		})
	}()

	first := readNextTermOutput(t, out, time.Second)
	var firstPayload struct {
		Mode string `json:"mode"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(first.Payload, &firstPayload); err != nil {
		t.Fatalf("decode first payload failed: %v", err)
	}
	if firstPayload.Mode != "reset" {
		t.Fatalf("expected first frame reset, got %s with data=%q", firstPayload.Mode, firstPayload.Data)
	}
	if firstPayload.Data != "BASE\nAPPEND" {
		t.Fatalf("unexpected reset payload data (trailing newline stripped): %q", firstPayload.Data)
	}

	second := readNextTermOutput(t, out, time.Second)
	var secondPayload struct {
		Mode string `json:"mode"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(second.Payload, &secondPayload); err != nil {
		t.Fatalf("decode second payload failed: %v", err)
	}
	if secondPayload.Mode != "append" {
		t.Fatalf("expected buffered append after reset, got %s", secondPayload.Mode)
	}
	if !strings.Contains(secondPayload.Data, "APPEND") {
		t.Fatalf("unexpected append payload data: %q", secondPayload.Data)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscribe did not return in time")
	}
}

func TestPaneActor_Subscribe_GapRecoverUsesHistoryLines(t *testing.T) {
	tmuxService := &streamPumpTmux{
		history:       "history_line_1\nhistory_line_2\n",
		paneSnapshots: []string{"visible_line_only\n"},
		cursors:       [][2]int{{0, 0}},
	}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	out := make(chan protocol.Message, 8)
	actor.Subscribe("conn_1", out, paneSubscribeOptions{
		GapRecover:   true,
		HistoryLines: 4000,
	})

	select {
	case msg := <-out:
		if msg.Op != "term.output" {
			t.Fatalf("expected term.output, got %s", msg.Op)
		}
		var payload struct {
			Mode string `json:"mode"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}
		if payload.Mode != "reset" {
			t.Fatalf("expected mode reset, got %s", payload.Mode)
		}
		if payload.Data != "history_line_1\nhistory_line_2" {
			t.Fatalf("expected reset data from capture history (trailing newline stripped), got %q", payload.Data)
		}
		if tmuxService.historyLines != 4000 {
			t.Fatalf("expected history lines 4000, got %d", tmuxService.historyLines)
		}
	case <-time.After(time.Second):
		t.Fatal("expected reset frame for gap recover subscribe")
	}
}

func TestPaneActor_CaptureResetSnapshot_StableSnapshotKeepsCursor(t *testing.T) {
	tmuxService := &streamPumpTmux{
		paneSnapshots: []string{"line-1\nline-2\n", "line-1\nline-2\n"},
		cursors:       [][2]int{{4, 1}},
	}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger())

	snapshot, cursorX, cursorY, hasCursor, err := actor.captureResetSnapshot()
	if err != nil {
		t.Fatalf("captureResetSnapshot failed: %v", err)
	}
	if snapshot != "line-1\nline-2\n" {
		t.Fatalf("unexpected snapshot: %q", snapshot)
	}
	if !hasCursor {
		t.Fatal("expected cursor to be preserved for stable snapshot")
	}
	if cursorX != 4 || cursorY != 1 {
		t.Fatalf("unexpected cursor: %d,%d", cursorX, cursorY)
	}
}

func TestPaneActor_CaptureResetSnapshot_SnapshotChangedDropsCursor(t *testing.T) {
	tmuxService := &streamPumpTmux{
		paneSnapshots: []string{"line-1\n", "line-1\nline-2\n"},
		cursors:       [][2]int{{2, 0}},
	}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger())

	snapshot, cursorX, cursorY, hasCursor, err := actor.captureResetSnapshot()
	if err != nil {
		t.Fatalf("captureResetSnapshot failed: %v", err)
	}
	if snapshot != "line-1\nline-2\n" {
		t.Fatalf("expected latest snapshot after drift, got %q", snapshot)
	}
	if hasCursor {
		t.Fatal("expected cursor to be dropped when snapshot drift is detected")
	}
	if cursorX != 0 || cursorY != 0 {
		t.Fatalf("unexpected cursor fallback: %d,%d", cursorX, cursorY)
	}
}

func TestPaneActor_ControlModeOutputBypassesSnapshotDiff(t *testing.T) {
	tmuxService := &streamPumpTmux{
		history:       "hello\n",
		paneSnapshots: []string{"hello\nls\n", "hello\nls\n", "hello\nls\n"},
		cursors:       [][2]int{{0, 0}, {0, 0}, {0, 0}},
	}
	realtime := &fakePaneRealtimeSource{
		streams: map[string]chan string{"e2e:0.0": make(chan string, 8)},
	}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, realtime, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	out := make(chan protocol.Message, 32)
	actor.Subscribe("conn_1", out)

	select {
	case <-out: // initial reset
	case <-time.After(time.Second):
		t.Fatal("expected initial reset frame")
	}

	realtime.streams["e2e:0.0"] <- "ls\n"
	time.Sleep(220 * time.Millisecond)

	appendCount := 0
	appendWithLS := 0
	for {
		select {
		case msg := <-out:
			if msg.Op != "term.output" {
				continue
			}
			var payload struct {
				Mode string `json:"mode"`
				Data string `json:"data"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				continue
			}
			if payload.Mode == "append" {
				appendCount++
				if strings.Contains(payload.Data, "ls") {
					appendWithLS++
				}
			}
		default:
			if appendWithLS != 1 {
				t.Fatalf("expected exactly one append carrying realtime data, total_append=%d append_with_ls=%d", appendCount, appendWithLS)
			}
			return
		}
	}
}

func TestPaneActor_SubscribeSendsResetBeforeRealtimeAppend(t *testing.T) {
	tmuxService := &delayedCaptureTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "hello\n",
			paneSnapshots: []string{"hello\n"},
			cursors:       [][2]int{{6, 0}},
		},
		captureDelay: 80 * time.Millisecond,
	}
	realtime := &eagerRealtimeSource{
		streams: map[string]chan string{"e2e:0.0": make(chan string, 8)},
	}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, realtime, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	out := make(chan protocol.Message, 16)
	actor.Subscribe("conn_1", out)

	select {
	case msg := <-out:
		if msg.Op != "term.output" {
			t.Fatalf("expected term.output, got %s", msg.Op)
		}
		var payload struct {
			Mode string `json:"mode"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}
		if payload.Mode != "reset" {
			t.Fatalf("expected first frame to be reset, got mode=%s data=%q", payload.Mode, payload.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("expected first output frame after subscribe")
	}
}

func TestPaneActor_SendToConn_PrioritizesResetWhenQueueFull(t *testing.T) {
	out := make(chan protocol.Message, 1)
	appendMsg := termOutputMessages("e2e:0.0", "append", "old-append", 0, 0, false)[0]
	out <- appendMsg

	actor := &PaneActor{
		target:      "e2e:0.0",
		logger:      testLogger(),
		subscribers: map[string]chan protocol.Message{"conn_1": out},
	}
	resetMsg := termOutputMessages("e2e:0.0", "reset", "fresh-reset", 0, 0, false)[0]

	actor.sendToConn("conn_1", resetMsg)

	select {
	case got := <-out:
		if got.Op != "term.output" {
			t.Fatalf("expected term.output, got %q", got.Op)
		}
		var payload struct {
			Mode string `json:"mode"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(got.Payload, &payload); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}
		if payload.Mode != "reset" {
			t.Fatalf("expected reset prioritized over append, got mode=%q data=%q", payload.Mode, payload.Data)
		}
		if payload.Data != "fresh-reset" {
			t.Fatalf("unexpected reset data: %q", payload.Data)
		}
	default:
		t.Fatal("expected one queued message")
	}
}

func TestTermOutputMessages_OversizedResetCarriesCursorOnlyOnLastChunk(t *testing.T) {
	oversized := strings.Repeat("x", maxTermFrameDataBytes*2+11)
	msgs := termOutputMessages("e2e:0.0", "reset", oversized, 7, 9, true)
	if len(msgs) < 3 {
		t.Fatalf("expected chunked reset frames, got %d", len(msgs))
	}
	cursorFrames := 0
	lastPayloadMode := ""
	for i, msg := range msgs {
		var payload struct {
			Mode   string         `json:"mode"`
			Cursor map[string]int `json:"cursor"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("decode payload[%d] failed: %v", i, err)
		}
		lastPayloadMode = payload.Mode
		if payload.Cursor != nil {
			cursorFrames++
			if i != len(msgs)-1 {
				t.Fatalf("cursor should appear only on last frame, got on frame %d/%d", i, len(msgs))
			}
			if payload.Cursor["x"] != 7 || payload.Cursor["y"] != 9 {
				t.Fatalf("unexpected cursor payload: %#v", payload.Cursor)
			}
		}
	}
	if cursorFrames != 1 {
		t.Fatalf("expected exactly one cursor-bearing frame, got %d", cursorFrames)
	}
	if lastPayloadMode != "append" {
		t.Fatalf("expected last chunk mode append after reset split, got %q", lastPayloadMode)
	}
}

func TestTermOutputMessages_ResetTrimsTrailingNewlineOnCursorChunk(t *testing.T) {
	msgs := termOutputMessages("e2e:0.0", "reset", "root# \n", 6, 0, true)
	if len(msgs) != 1 {
		t.Fatalf("expected one message, got %d", len(msgs))
	}
	var payload struct {
		Mode string         `json:"mode"`
		Data string         `json:"data"`
		Cur  map[string]int `json:"cursor"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if payload.Mode != "reset" {
		t.Fatalf("expected mode reset, got %q", payload.Mode)
	}
	if payload.Data != "root# " {
		t.Fatalf("expected trailing newline trimmed, got %q", payload.Data)
	}
	if payload.Cur == nil || payload.Cur["x"] != 6 || payload.Cur["y"] != 0 {
		t.Fatalf("unexpected cursor payload: %#v", payload.Cur)
	}
}

func TestTermOutputMessages_AppendKeepsTrailingNewline(t *testing.T) {
	msgs := termOutputMessages("e2e:0.0", "append", "line\n", 0, 0, true)
	if len(msgs) != 1 {
		t.Fatalf("expected one message, got %d", len(msgs))
	}
	var payload struct {
		Mode string `json:"mode"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(msgs[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if payload.Mode != "append" {
		t.Fatalf("expected mode append, got %q", payload.Mode)
	}
	if payload.Data != "line\n" {
		t.Fatalf("append payload should keep trailing newline, got %q", payload.Data)
	}
}

func TestPaneActor_ReadyEdgeTriggersAutoCompleteOnce(t *testing.T) {
	resetAutoProgressSuppressionForTest()
	tmuxService := &commandAwareTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "boot$\nrun$\n",
			paneSnapshots: []string{"boot$\n", "run$\n", "run$\n"},
			cursors:       [][2]int{{0, 0}, {0, 0}, {0, 0}},
		},
		title:   "e2e",
		command: "codex",
	}
	var autoCompleteCalls atomic.Int32
	autoComplete := func(paneTarget string, observedLastActiveAt time.Time) (localapi.AutoCompleteByPaneResult, error) {
		autoCompleteCalls.Add(1)
		return localapi.AutoCompleteByPaneResult{
			Triggered: true,
			Status:    "completed",
			Reason:    "ok",
		}, nil
	}

	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, autoComplete, nil, testLogger())
	base := time.Now().UTC()
	actor.emitStatus("boot$\n", base)
	actor.emitStatus("run$\n", base.Add(20*time.Millisecond))
	actor.emitStatus("run$\n", base.Add(2400*time.Millisecond))
	actor.emitStatus("run$\n", base.Add(5*time.Second))

	if got := autoCompleteCalls.Load(); got != 1 {
		t.Fatalf("expected auto-complete exactly once per ready edge, got %d", got)
	}
}

func TestPaneActor_ColdStartStaticPane_DoesNotTriggerAutoComplete(t *testing.T) {
	oldStatusInterval := statusPumpInterval
	oldStreamInterval := streamPumpInterval
	oldDelay := statusTransitionDelay
	oldInputWindow := statusInputIgnoreWindow
	// Keep status pump out of this test window so pane actor snapshot sequence is deterministic.
	statusPumpInterval = 5 * time.Second
	streamPumpInterval = 5 * time.Millisecond
	statusTransitionDelay = 10 * time.Millisecond
	statusInputIgnoreWindow = 10 * time.Millisecond
	defer func() {
		statusPumpInterval = oldStatusInterval
		streamPumpInterval = oldStreamInterval
		statusTransitionDelay = oldDelay
		statusInputIgnoreWindow = oldInputWindow
	}()

	tmuxService := &statusPumpTmux{
		lists: [][]string{
			{"e2e:0.0"}, {"e2e:0.0"}, {"e2e:0.0"}, {"e2e:0.0"}, {"e2e:0.0"},
		},
		shots: map[string][]string{
			"e2e:0.0": {"bash$", "bash$", "bash$", "bash$", "bash$"},
		},
		shotIdx: map[string]int{},
	}

	var autoCompleteCalls atomic.Int32
	autoComplete := func(paneTarget string, observedLastActiveAt time.Time) (localapi.AutoCompleteByPaneResult, error) {
		autoCompleteCalls.Add(1)
		return localapi.AutoCompleteByPaneResult{
			Triggered: true,
			Status:    "completed",
			Reason:    "ok",
		}, nil
	}

	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runWSRuntime(ctx, wsClient, tmuxService, nil, autoComplete, testLogger())
	}()

	time.Sleep(120 * time.Millisecond)
	cancel()
	<-done

	if got := autoCompleteCalls.Load(); got != 0 {
		t.Fatalf("expected no auto-complete on cold-start static pane, got %d", got)
	}
}

func TestPaneActor_ReportsToTaskStateActor_AtMostOncePerSecond(t *testing.T) {
	tmuxService := &streamPumpTmux{
		history:       "hello\n",
		paneSnapshots: []string{"hello\n", "hello1\n", "hello2\n", "hello3\n", "hello4\n"},
		cursors:       [][2]int{{0, 0}, {0, 1}, {0, 2}, {0, 3}, {0, 4}},
	}
	sink := &fakeTaskStateSink{}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger(), sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	time.Sleep(250 * time.Millisecond)
	got := sink.calls.Load()
	if got < 1 {
		t.Fatalf("expected at least one report, got %d", got)
	}
	if got > 1 {
		t.Fatalf("want <=1 per second, got %d", got)
	}
}

func TestPaneActor_ReportTaskState_IncludesCurrentCommand(t *testing.T) {
	tmuxService := &commandAwareTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "hello\n",
			paneSnapshots: []string{"hello\n", "hello\n"},
			cursors:       [][2]int{{0, 0}, {0, 0}},
		},
		title:   "e2e",
		command: "codex (/Users/wanglei/.)",
	}
	sink := &fakeTaskStateSink{}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger(), sink)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	time.Sleep(120 * time.Millisecond)
	if sink.calls.Load() < 1 {
		t.Fatal("expected at least one task state report")
	}
	if got := strings.TrimSpace(sink.Last().CurrentCommand); got != "codex (/Users/wanglei/.)" {
		t.Fatalf("expected current command from tmux metadata, got %q", got)
	}
}

func TestPaneActor_ReportTaskState_UsesLastActiveAtAsUpdatedAt(t *testing.T) {
	tmuxService := &commandAwareTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "hello\n",
			paneSnapshots: []string{"hello\n", "hello\n"},
			cursors:       [][2]int{{0, 0}, {0, 0}},
		},
		title:   "e2e",
		command: "bash",
	}
	sink := &fakeTaskStateSink{}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger(), sink)
	actor.SetRuntimeBaseline(paneRuntimeBaseline{
		LastActiveAt:  1704067200,
		RuntimeStatus: SessionStatusReady,
		SnapshotHash:  sha1Text(normalizeTermSnapshot("hello\n")),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	time.Sleep(120 * time.Millisecond)
	if sink.calls.Load() < 1 {
		t.Fatal("expected at least one task state report")
	}
	if got := sink.Last().UpdatedAt; got != 1704067200 {
		t.Fatalf("expected updated_at from baseline last_active_at, got %d", got)
	}
}

func TestPaneActor_ReportTaskState_KeepsBaselineUpdatedAtOnFirstTickEvenWhenHashDiffers(t *testing.T) {
	tmuxService := &commandAwareTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "hello\n",
			paneSnapshots: []string{"hello\n", "hello\n"},
			cursors:       [][2]int{{0, 0}, {0, 0}},
		},
		title:   "e2e",
		command: "bash",
	}
	sink := &fakeTaskStateSink{}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger(), sink)
	actor.SetRuntimeBaseline(paneRuntimeBaseline{
		LastActiveAt:  1704067200,
		RuntimeStatus: SessionStatusReady,
		SnapshotHash:  "hash_from_old_process",
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	time.Sleep(120 * time.Millisecond)
	if sink.calls.Load() < 1 {
		t.Fatal("expected at least one task state report")
	}
	if got := sink.Last().UpdatedAt; got != 1704067200 {
		t.Fatalf("expected first report keep baseline updated_at, got %d", got)
	}
}

func TestPaneActor_ReportTaskState_SubscribeThenFirstTickHashDiff_StillKeepsBaselineUpdatedAt(t *testing.T) {
	tmuxService := &commandAwareTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "hello\n",
			paneSnapshots: []string{"snap_a\n", "snap_b\n", "snap_b\n"},
			cursors:       [][2]int{{0, 0}, {0, 0}, {0, 0}},
		},
		title:   "e2e",
		command: "bash",
	}
	sink := &fakeTaskStateSink{}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger(), sink)
	actor.SetRuntimeBaseline(paneRuntimeBaseline{
		LastActiveAt:  1704067200,
		RuntimeStatus: SessionStatusReady,
		SnapshotHash:  "hash_from_old_process",
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor.Start(ctx)

	out := make(chan protocol.Message, 8)
	actor.Subscribe("conn_1", out)
	time.Sleep(140 * time.Millisecond)
	if sink.calls.Load() < 1 {
		t.Fatal("expected at least one task state report")
	}
	if got := sink.Last().UpdatedAt; got != 1704067200 {
		t.Fatalf("expected first report keep baseline updated_at after subscribe+tick hash diff, got %d", got)
	}
}

func TestPaneActor_HashSamplingUsesFastStabilizationWindow(t *testing.T) {
	tmuxService := &commandAwareTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "hello\n",
			paneSnapshots: []string{"same\n", "same\n", "same\n"},
			cursors:       [][2]int{{0, 0}, {0, 0}, {0, 0}},
		},
		title:   "e2e",
		command: "bash",
	}
	actor := NewPaneActor("e2e:0.0", tmuxService, 20*time.Millisecond, nil, nil, nil, testLogger())
	base := time.Now().UTC()

	actor.emitStatus("same\n", base)
	if got := normalizeSessionStatus(actor.statusState.Emitted); got != SessionStatusRunning {
		t.Fatalf("expected running on first sample, got %s", got)
	}

	actor.emitStatus("same\n", base.Add(1100*time.Millisecond))
	if got := normalizeSessionStatus(actor.statusState.Emitted); got != SessionStatusRunning {
		t.Fatalf("expected still running before fast delay, got %s", got)
	}

	actor.emitStatus("same\n", base.Add(2400*time.Millisecond))
	if got := normalizeSessionStatus(actor.statusState.Emitted); got != SessionStatusReady {
		t.Fatalf("expected ready after fast delay, got %s", got)
	}
}

func TestPaneActor_ReadyEdgeSuppressedWhenHashAlreadyConsumedByTool(t *testing.T) {
	resetAutoProgressSuppressionForTest()
	target := "e2e:0.0"
	tmuxService := &commandAwareTmux{
		streamPumpTmux: streamPumpTmux{
			history:       "hello\n",
			paneSnapshots: []string{"boot$\n", "run$\n", "run$\n", "next$\n", "next$\n"},
			cursors:       [][2]int{{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}},
		},
		title:   "e2e",
		command: "codex",
	}
	var autoCompleteCalls atomic.Int32
	autoComplete := func(paneTarget string, observedLastActiveAt time.Time) (localapi.AutoCompleteByPaneResult, error) {
		autoCompleteCalls.Add(1)
		return localapi.AutoCompleteByPaneResult{Triggered: true, Status: "completed"}, nil
	}
	actor := NewPaneActor(target, tmuxService, 20*time.Millisecond, nil, autoComplete, nil, testLogger())
	base := time.Now().UTC()

	actor.emitStatus("boot$\n", base)
	actor.emitStatus("run$\n", base.Add(20*time.Millisecond))
	registerAutoProgressSuppression(target, sha1Text(normalizeTermSnapshot("run$\n")), true)
	actor.emitStatus("run$\n", base.Add(2400*time.Millisecond))
	actor.emitStatus("run$\n", base.Add(5000*time.Millisecond))
	if got := autoCompleteCalls.Load(); got != 0 {
		t.Fatalf("expected suppressed ready edge not to trigger auto-complete, got %d", got)
	}
	if consumeAutoProgressSuppression(target, sha1Text(normalizeTermSnapshot("run$\n"))) {
		t.Fatal("expected suppression entry consumed by first ready edge check")
	}
}
