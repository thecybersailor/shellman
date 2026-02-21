package bridge

import (
	"encoding/json"
	"testing"

	"shellman/cli/internal/protocol"
)

func TestHandle_TmuxList(t *testing.T) {
	h := NewHandler(&FakeTmux{Sessions: []string{"s1"}})
	resp := h.Handle(protocol.Message{ID: "1", Type: "req", Op: "tmux.list"})
	if resp.Type != "res" || resp.Op != "tmux.list" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestHandle_TermInput(t *testing.T) {
	fake := &FakeTmux{Sessions: []string{"e2e"}}
	h := NewHandler(fake)
	msg := protocol.Message{
		ID:      "9",
		Type:    "event",
		Op:      "term.input",
		Payload: protocol.MustRaw(map[string]any{"target": "e2e:0.0", "text": "printf __TT_E2E_OK__\\n"}),
	}
	resp := h.Handle(msg)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if fake.InputTarget != "e2e:0.0" {
		t.Fatalf("unexpected target: %s", fake.InputTarget)
	}
	if fake.InputText == "" {
		t.Fatal("expected input text to be forwarded")
	}
}

func TestHandle_TermResize(t *testing.T) {
	fake := &FakeTmux{Sessions: []string{"e2e"}}
	h := NewHandler(fake)
	msg := protocol.Message{
		ID:      "10",
		Type:    "event",
		Op:      "term.resize",
		Payload: protocol.MustRaw(map[string]any{"target": "e2e:0.0", "cols": 120, "rows": 40}),
	}
	resp := h.Handle(msg)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if fake.ResizeTarget != "e2e:0.0" {
		t.Fatalf("unexpected resize target: %s", fake.ResizeTarget)
	}
	if fake.ResizeCols != 120 || fake.ResizeRows != 40 {
		t.Fatalf("unexpected resize size: %dx%d", fake.ResizeCols, fake.ResizeRows)
	}
}

func TestHandle_SelectPane_ReturnsPaneNotFoundCodeWhenMissing(t *testing.T) {
	fake := &FakeTmux{
		Sessions:      []string{"e2e:0.0"},
		PaneExistsMap: map[string]bool{"missing:e2e": false},
	}
	h := NewHandler(fake)
	msg := protocol.Message{
		ID:      "sel_missing",
		Type:    "req",
		Op:      "tmux.select_pane",
		Payload: protocol.MustRaw(map[string]any{"target": "missing:e2e", "cols": 80, "rows": 24}),
	}
	resp := h.Handle(msg)
	if resp.Error == nil {
		t.Fatalf("expected error, got nil")
	}
	if resp.Error.Code != "TMUX_PANE_NOT_FOUND" {
		t.Fatalf("expected TMUX_PANE_NOT_FOUND, got %s", resp.Error.Code)
	}
}

func TestHandle_GatewayHTTP(t *testing.T) {
	fake := &FakeTmux{Sessions: []string{"e2e:0.0"}}
	h := NewHandler(fake)
	h.SetHTTPExecutor(func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
		if method != "GET" || path != "/api/v1/system/capabilities" {
			t.Fatalf("unexpected request: %s %s", method, path)
		}
		return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true}`, nil
	})

	msg := protocol.Message{
		ID:   "gw_1",
		Type: "req",
		Op:   "gateway.http",
		Payload: protocol.MustRaw(map[string]any{
			"method": "GET",
			"path":   "/api/v1/system/capabilities",
		}),
	}
	resp := h.Handle(msg)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var payload struct {
		Status int               `json:"status"`
		Body   string            `json:"body"`
		Header map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if payload.Status != 200 {
		t.Fatalf("expected status 200, got %d", payload.Status)
	}
	if payload.Body != `{"ok":true}` {
		t.Fatalf("unexpected body: %s", payload.Body)
	}
}

func TestHandle_GatewayHTTP_AppProgramsEndpoint(t *testing.T) {
	fake := &FakeTmux{Sessions: []string{"e2e:0.0"}}
	h := NewHandler(fake)
	h.SetHTTPExecutor(func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
		if method != "GET" || path != "/api/v1/system/app-programs" {
			t.Fatalf("unexpected request: %s %s", method, path)
		}
		return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true,"data":{"version":1,"providers":[{"id":"codex"}]}}`, nil
	})

	msg := protocol.Message{
		ID:   "gw_2",
		Type: "req",
		Op:   "gateway.http",
		Payload: protocol.MustRaw(map[string]any{
			"method": "GET",
			"path":   "/api/v1/system/app-programs",
		}),
	}
	resp := h.Handle(msg)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	var payload struct {
		Status int    `json:"status"`
		Body   string `json:"body"`
	}
	if err := json.Unmarshal(resp.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if payload.Status != 200 {
		t.Fatalf("expected status 200, got %d", payload.Status)
	}
	if payload.Body == "" {
		t.Fatalf("expected body")
	}
}

type FakeTmux struct {
	Sessions      []string
	SelectErr     error
	PaneExistsMap map[string]bool
	InputText     string
	InputTarget   string
	ResizeTarget  string
	ResizeCols    int
	ResizeRows    int
}

func (f *FakeTmux) ListSessions() ([]string, error) {
	return f.Sessions, nil
}

func (f *FakeTmux) SelectPane(target string) error {
	return f.SelectErr
}

func (f *FakeTmux) PaneExists(target string) (bool, error) {
	if f.PaneExistsMap == nil {
		return true, nil
	}
	exists, ok := f.PaneExistsMap[target]
	if !ok {
		return true, nil
	}
	return exists, nil
}

func (f *FakeTmux) SendInput(target, text string) error {
	f.InputTarget = target
	f.InputText = text
	return nil
}

func (f *FakeTmux) CapturePane(target string) (string, error) {
	return "", nil
}

func (f *FakeTmux) CaptureHistory(target string, lines int) (string, error) {
	return "", nil
}

func (f *FakeTmux) StartPipePane(target, shellCmd string) error {
	return nil
}

func (f *FakeTmux) StopPipePane(target string) error {
	return nil
}

func (f *FakeTmux) Resize(target string, cols, rows int) error {
	f.ResizeTarget = target
	f.ResizeCols = cols
	f.ResizeRows = rows
	return nil
}

func (f *FakeTmux) CursorPosition(target string) (int, int, error) {
	return 0, 0, nil
}

func (f *FakeTmux) CreateSiblingPane(target string) (string, error) {
	return "e2e:0.2", nil
}

func (f *FakeTmux) CreateChildPane(target string) (string, error) {
	return "e2e:0.3", nil
}
