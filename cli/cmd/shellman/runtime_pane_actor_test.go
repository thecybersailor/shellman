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
		if payload.Data != "visible_line_only\n" {
			t.Fatalf("expected reset data from visible pane snapshot, got %q", payload.Data)
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

func TestPaneActor_ReadyEdgeTriggersAutoCompleteOnce(t *testing.T) {
	oldStatusInterval := statusPumpInterval
	oldStreamInterval := streamPumpInterval
	oldDelay := statusTransitionDelay
	oldInputWindow := statusInputIgnoreWindow
	statusPumpInterval = 5 * time.Millisecond
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
			"e2e:0.0": {"boot$", "run$", "run$", "run$", "run$"},
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

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if sock.onText != nil {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if sock.onText == nil {
		t.Fatal("ws message loop handler was not installed in time")
	}

	req := protocol.Message{
		ID:   "req_1",
		Type: "req",
		Op:   "tmux.select_pane",
		Payload: protocol.MustRaw(map[string]any{
			"target": "e2e:0.0",
		}),
	}
	rawReq, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request failed: %v", err)
	}
	wrappedReq, err := protocol.WrapMuxEnvelope("conn_1", rawReq)
	if err != nil {
		t.Fatalf("wrap request failed: %v", err)
	}
	sock.EmitText(string(wrappedReq))

	time.Sleep(220 * time.Millisecond)
	cancel()
	_ = <-done

	if got := autoCompleteCalls.Load(); got != 1 {
		t.Fatalf("expected auto-complete exactly once per ready edge, got %d", got)
	}
}

func TestPaneActor_ColdStartStaticPane_DoesNotTriggerAutoComplete(t *testing.T) {
	oldStatusInterval := statusPumpInterval
	oldStreamInterval := streamPumpInterval
	oldDelay := statusTransitionDelay
	oldInputWindow := statusInputIgnoreWindow
	statusPumpInterval = 5 * time.Millisecond
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
	_ = <-done

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
