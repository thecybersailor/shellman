package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"shellman/cli/internal/bridge"
	"shellman/cli/internal/protocol"
	"shellman/cli/internal/turn"
)

type fakeRegisterClient struct {
	resp turn.RegisterResponse
}

func (f *fakeRegisterClient) Register() (turn.RegisterResponse, error) {
	return f.resp, nil
}

type fakeDialer struct {
	called bool
	url    string
	sock   turn.Socket
}

func (f *fakeDialer) Dial(_ context.Context, url string) (turn.Socket, error) {
	f.called = true
	f.url = url
	return f.sock, nil
}

type fakeSocket struct {
	readCalled bool
	writes     []string
	onText     func(string)
}

func (f *fakeSocket) ReadText(ctx context.Context) (string, error) {
	f.readCalled = true
	<-ctx.Done()
	return "", ctx.Err()
}

func (f *fakeSocket) WriteText(ctx context.Context, text string) error {
	f.writes = append(f.writes, text)
	return nil
}

func (f *fakeSocket) SetOnText(fn func(string)) {
	f.onText = fn
}

func (f *fakeSocket) EmitText(text string) {
	if f.onText != nil {
		f.onText(text)
	}
}

func (f *fakeSocket) Close() error {
	return nil
}

type fakeTmux struct{}

func (f *fakeTmux) ListSessions() ([]string, error) {
	return []string{"s1"}, nil
}

func (f *fakeTmux) PaneExists(target string) (bool, error) {
	return true, nil
}

func (f *fakeTmux) SelectPane(target string) error {
	return nil
}

func (f *fakeTmux) SendInput(target, text string) error {
	return nil
}

func (f *fakeTmux) Resize(target string, cols, rows int) error {
	return nil
}

func (f *fakeTmux) CapturePane(target string) (string, error) {
	return "", nil
}

func (f *fakeTmux) CaptureHistory(target string, lines int) (string, error) {
	return "", nil
}

func (f *fakeTmux) StartPipePane(target, shellCmd string) error {
	return nil
}

func (f *fakeTmux) StopPipePane(target string) error {
	return nil
}

func (f *fakeTmux) CursorPosition(target string) (int, int, error) {
	return 0, 0, nil
}

func (f *fakeTmux) CreateSiblingPane(target string) (string, error) {
	return "e2e:0.1", nil
}

func (f *fakeTmux) CreateChildPane(target string) (string, error) {
	return "e2e:0.2", nil
}

func TestRun_ConnectsAgentAndRunsLoop(t *testing.T) {
	sock := &fakeSocket{}
	dialer := &fakeDialer{sock: sock}
	register := &fakeRegisterClient{resp: turn.RegisterResponse{
		TurnUUID:   "u1",
		VisitURL:   "http://localhost/t/u1",
		AgentWSURL: "ws://localhost/ws/agent/u1",
	}}
	tmuxService := &fakeTmux{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, io.Discard, io.Discard, register, dialer, tmuxService)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-done
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("run failed: %v", err)
	}
	if !dialer.called {
		t.Fatal("dialer was not called")
	}
	if dialer.url != register.resp.AgentWSURL {
		t.Fatalf("unexpected dial url: %s", dialer.url)
	}
	if !sock.readCalled {
		t.Fatal("message loop did not start")
	}
}

func TestRun_EmitsTmuxStatusEvent(t *testing.T) {
	sock := &fakeSocket{}
	dialer := &fakeDialer{sock: sock}
	register := &fakeRegisterClient{resp: turn.RegisterResponse{
		TurnUUID:   "u1",
		VisitURL:   "http://localhost/t/u1",
		AgentWSURL: "ws://localhost/ws/agent/u1",
	}}
	tmuxService := &fakeTmux{}

	oldStatusInterval := statusPumpInterval
	statusPumpInterval = 5 * time.Millisecond
	defer func() { statusPumpInterval = oldStatusInterval }()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, io.Discard, io.Discard, register, dialer, tmuxService)
	}()
	time.Sleep(35 * time.Millisecond)
	cancel()
	<-done

	found := false
	for _, line := range sock.writes {
		var msg protocol.Message
		if err := json.Unmarshal([]byte(line), &msg); err == nil && msg.Op == "tmux.status" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected tmux.status event from run()")
	}
}

func TestEnrichGatewayHTTPMessage_AddsActivePaneHeader(t *testing.T) {
	msg := protocol.Message{
		ID:   "req_1",
		Type: "req",
		Op:   "gateway.http",
		Payload: protocol.MustRaw(map[string]any{
			"method":  "POST",
			"path":    "/api/v1/runs/r_1/report-result",
			"headers": map[string]string{},
			"body":    `{"summary":"done"}`,
		}),
	}
	got := enrichGatewayHTTPMessage(msg, "botworks:3.0")
	var payload struct {
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if payload.Headers["X-Shellman-Active-Pane-Target"] != "botworks:3.0" {
		t.Fatalf("expected active pane header, got %#v", payload.Headers)
	}
}

func TestEnrichGatewayHTTPMessage_RespectsExistingActivePaneHeader(t *testing.T) {
	msg := protocol.Message{
		ID:   "req_1",
		Type: "req",
		Op:   "gateway.http",
		Payload: protocol.MustRaw(map[string]any{
			"method": "POST",
			"path":   "/api/v1/runs/r_1/report-result",
			"headers": map[string]string{
				"X-Shellman-Active-Pane-Target": "botworks:1.0",
			},
			"body": `{"summary":"done"}`,
		}),
	}
	got := enrichGatewayHTTPMessage(msg, "botworks:3.0")
	var payload struct {
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if payload.Headers["X-Shellman-Active-Pane-Target"] != "botworks:1.0" {
		t.Fatalf("expected existing header preserved, got %#v", payload.Headers)
	}
}

func TestBindMessageLoop_SelectPaneSameTargetBumpsVersion(t *testing.T) {
	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	handler := bridge.NewHandler(&fakeTmux{})
	registry := NewRegistryActor(nil)
	registry.ConfigureRuntime(context.Background(), wsClient, &fakeTmux{}, nil, nil, nil, 10*time.Millisecond)
	inputTracker := newInputActivityTracker()

	bindMessageLoop(wsClient, handler, registry, inputTracker, nil)

	req := func(id string) string {
		raw, err := json.Marshal(protocol.Message{
			ID:   id,
			Type: "req",
			Op:   "tmux.select_pane",
			Payload: protocol.MustRaw(map[string]any{
				"target": "e2e:0.0",
			}),
		})
		if err != nil {
			t.Fatalf("marshal request failed: %v", err)
		}
		return string(raw)
	}

	sock.EmitText(req("req_1"))
	if got := registry.GetOrCreateConn("legacy_single").Selected(); got != "e2e:0.0" {
		t.Fatalf("unexpected selected pane after first select: %q", got)
	}

	sock.EmitText(req("req_2"))
	if got := registry.GetOrCreateConn("legacy_single").Selected(); got != "e2e:0.0" {
		t.Fatalf("unexpected selected pane after second select: %q", got)
	}
}

func TestActorRuntime_ReSelectSamePaneAlwaysSendsResetToThatConn(t *testing.T) {
	sock := &fakeSocket{}
	wsClient := turn.NewWSClient(sock)
	tmuxService := &streamPumpTmux{
		history:       "hello\n",
		paneSnapshots: []string{"hello\n", "hello\n"},
		cursors:       [][2]int{{0, 0}, {0, 0}},
	}
	handler := bridge.NewHandler(tmuxService)
	registry := NewRegistryActor(nil)
	inputTracker := newInputActivityTracker()
	runtimeCtx, runtimeCancel := context.WithCancel(context.Background())
	defer runtimeCancel()
	registry.ConfigureRuntime(runtimeCtx, wsClient, tmuxService, inputTracker, nil, nil, 5*time.Millisecond)
	bindMessageLoop(wsClient, handler, registry, inputTracker, nil)

	selectReq := func(id string) string {
		msg := protocol.Message{
			ID:   id,
			Type: "req",
			Op:   "tmux.select_pane",
			Payload: protocol.MustRaw(map[string]any{
				"target": "e2e:0.0",
			}),
		}
		rawMsg, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("marshal inner message failed: %v", err)
		}
		out, err := protocol.WrapMuxEnvelope("conn_1", rawMsg)
		if err != nil {
			t.Fatalf("wrap mux envelope failed: %v", err)
		}
		return string(out)
	}

	sock.EmitText(selectReq("req_1"))
	time.Sleep(20 * time.Millisecond)
	sock.EmitText(selectReq("req_2"))
	time.Sleep(60 * time.Millisecond)

	resetCount := 0
	for _, line := range sock.writes {
		connID, inner, err := protocol.UnwrapMuxEnvelope([]byte(line))
		if err != nil {
			continue
		}
		if connID != "conn_1" {
			continue
		}
		var msg protocol.Message
		if err := json.Unmarshal(inner, &msg); err != nil || msg.Op != "term.output" {
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
		t.Fatalf("expected at least two reset frames for same conn re-select, got %d", resetCount)
	}
}
