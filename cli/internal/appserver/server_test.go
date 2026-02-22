package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"shellman/cli/internal/global"
	"shellman/cli/internal/localapi"
	"shellman/cli/internal/protocol"
)

type fakeConfigStore struct{}

func (f *fakeConfigStore) LoadOrInit() (global.GlobalConfig, error) {
	return global.GlobalConfig{LocalPort: 4621}, nil
}
func (f *fakeConfigStore) Save(cfg global.GlobalConfig) error { return nil }

type fakeProjectsStore struct{}

func (f *fakeProjectsStore) ListProjects() ([]global.ActiveProject, error) { return nil, nil }
func (f *fakeProjectsStore) AddProject(project global.ActiveProject) error { return nil }
func (f *fakeProjectsStore) RemoveProject(projectID string) error          { return nil }

type fakePaneService struct{}

func (f *fakePaneService) CreateSiblingPaneInDir(targetTaskID, cwd string) (string, error) {
	return "pane-1", nil
}
func (f *fakePaneService) CreateChildPaneInDir(targetTaskID, cwd string) (string, error) {
	return "pane-2", nil
}
func (f *fakePaneService) CreateRootPaneInDir(cwd string) (string, error) { return "pane-0", nil }
func (f *fakePaneService) ClosePane(target string) error                  { return nil }
func (f *fakePaneService) CaptureHistory(target string, lines int) (string, error) {
	return "history\n", nil
}

func makeDeps() Deps {
	return Deps{
		LocalAPI: localapi.Deps{
			ConfigStore:   &fakeConfigStore{},
			ProjectsStore: &fakeProjectsStore{},
			PaneService:   &fakePaneService{},
		},
		WebUI: WebUIConfig{Mode: "dev", DevProxyURL: "http://127.0.0.1:15173"},
	}
}

func TestServer_DevProxy_ForRootPath(t *testing.T) {
	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("vite-dev-ok"))
	}))
	defer vite.Close()

	deps := makeDeps()
	deps.WebUI.DevProxyURL = vite.URL
	srv, err := NewServer(deps)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET / failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "vite-dev-ok") {
		t.Fatalf("expected dev proxy body, got %s", string(body))
	}
}

func TestServer_LocalAPIAndMCPRoutes(t *testing.T) {
	srv, err := NewServer(makeDeps())
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	apiResp, err := http.Get(ts.URL + "/api/v1/config")
	if err != nil {
		t.Fatalf("GET api failed: %v", err)
	}
	if apiResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from api route, got %d", apiResp.StatusCode)
	}

	mcpResp, err := http.Get(ts.URL + "/mcp/health")
	if err != nil {
		t.Fatalf("GET mcp failed: %v", err)
	}
	if mcpResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from mcp route, got %d", mcpResp.StatusCode)
	}
}

func TestServer_EdgeWSBridge(t *testing.T) {
	srv, err := NewServer(makeDeps())
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	baseWS := "ws" + strings.TrimPrefix(ts.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agent, _, err := websocket.Dial(ctx, baseWS+"/ws/agent/u1", nil)
	if err != nil {
		t.Fatalf("dial agent failed: %v", err)
	}
	agent.SetReadLimit(-1)
	defer func() { _ = agent.Close(websocket.StatusNormalClosure, "") }()

	client, _, err := websocket.Dial(ctx, baseWS+"/ws/client/u1", nil)
	if err != nil {
		t.Fatalf("dial client failed: %v", err)
	}
	client.SetReadLimit(-1)
	defer func() { _ = client.Close(websocket.StatusNormalClosure, "") }()

	reqRaw := []byte(`{"id":"req_1","type":"req","op":"tmux.list","payload":{"scope":"all"}}`)
	if err := client.Write(ctx, websocket.MessageText, reqRaw); err != nil {
		t.Fatalf("client write failed: %v", err)
	}
	_, msg, err := agent.Read(ctx)
	if err != nil {
		t.Fatalf("agent read failed: %v", err)
	}
	connID, inner, err := protocol.UnwrapMuxEnvelope(msg)
	if err != nil {
		t.Fatalf("agent unwrap failed: %v", err)
	}
	if string(inner) != string(reqRaw) {
		t.Fatalf("expected wrapped client payload, got %s", string(inner))
	}

	resRaw := []byte(`{"id":"req_1","type":"res","op":"tmux.list","payload":{"sessions":["s1"]}}`)
	out, err := protocol.WrapMuxEnvelope(connID, resRaw)
	if err != nil {
		t.Fatalf("agent wrap failed: %v", err)
	}
	if err := agent.Write(ctx, websocket.MessageText, out); err != nil {
		t.Fatalf("agent write failed: %v", err)
	}
	_, msg, err = client.Read(ctx)
	if err != nil {
		t.Fatalf("client read failed: %v", err)
	}
	if string(msg) != string(resRaw) {
		t.Fatalf("expected forwarded agent msg, got %s", string(msg))
	}
}

func TestServer_EdgeWSBridge_AllowsLargeAgentMessage(t *testing.T) {
	srv, err := NewServer(makeDeps())
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	baseWS := "ws" + strings.TrimPrefix(ts.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agent, _, err := websocket.Dial(ctx, baseWS+"/ws/agent/u1", nil)
	if err != nil {
		t.Fatalf("dial agent failed: %v", err)
	}
	agent.SetReadLimit(-1)
	defer func() { _ = agent.Close(websocket.StatusNormalClosure, "") }()

	client, _, err := websocket.Dial(ctx, baseWS+"/ws/client/u1", nil)
	if err != nil {
		t.Fatalf("dial client failed: %v", err)
	}
	client.SetReadLimit(-1)
	defer func() { _ = client.Close(websocket.StatusNormalClosure, "") }()

	msg := map[string]any{
		"id":   "evt_status_big",
		"type": "event",
		"op":   "tmux.status",
		"payload": map[string]any{
			"mode": "full",
			"items": []map[string]any{
				{
					"target":          "u1:0.0",
					"title":           strings.Repeat("x", 40000),
					"current_command": "zsh",
					"status":          "running",
					"updated_at":      1,
				},
			},
		},
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal big status msg failed: %v", err)
	}

	if err := agent.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatalf("agent write failed: %v", err)
	}

	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()
	_, got, err := client.Read(readCtx)
	if err != nil {
		t.Fatalf("client read failed for big agent msg: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("expected forwarded big agent msg, got len=%d want=%d", len(got), len(raw))
	}
}

func TestEdgeWSHub_MultiClientRoutesByConnID(t *testing.T) {
	srv, err := NewServer(makeDeps())
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	baseWS := "ws" + strings.TrimPrefix(ts.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	agent, _, err := websocket.Dial(ctx, baseWS+"/ws/agent/u1", nil)
	if err != nil {
		t.Fatalf("dial agent failed: %v", err)
	}
	defer func() { _ = agent.Close(websocket.StatusNormalClosure, "") }()

	client1, _, err := websocket.Dial(ctx, baseWS+"/ws/client/u1", nil)
	if err != nil {
		t.Fatalf("dial client1 failed: %v", err)
	}
	defer func() { _ = client1.Close(websocket.StatusNormalClosure, "") }()

	client2, _, err := websocket.Dial(ctx, baseWS+"/ws/client/u1", nil)
	if err != nil {
		t.Fatalf("dial client2 failed: %v", err)
	}
	defer func() { _ = client2.Close(websocket.StatusNormalClosure, "") }()

	reqRaw := []byte(`{"id":"1","type":"req","op":"tmux.list","payload":{"scope":"all"}}`)
	if err := client1.Write(ctx, websocket.MessageText, reqRaw); err != nil {
		t.Fatalf("client1 write failed: %v", err)
	}

	_, agentIn, err := agent.Read(ctx)
	if err != nil {
		t.Fatalf("agent read failed: %v", err)
	}
	connID, inner, err := protocol.UnwrapMuxEnvelope(agentIn)
	if err != nil {
		t.Fatalf("unwrap mux envelope failed: %v", err)
	}
	if string(inner) != string(reqRaw) {
		t.Fatalf("unexpected inner payload: %s", string(inner))
	}

	resRaw := []byte(`{"id":"1","type":"res","op":"tmux.list","payload":{"sessions":["s1"]}}`)
	toClient1, err := protocol.WrapMuxEnvelope(connID, resRaw)
	if err != nil {
		t.Fatalf("wrap mux envelope failed: %v", err)
	}
	if err := agent.Write(ctx, websocket.MessageText, toClient1); err != nil {
		t.Fatalf("agent write failed: %v", err)
	}

	readCtx1, cancel1 := context.WithTimeout(ctx, time.Second)
	defer cancel1()
	_, got1, err := client1.Read(readCtx1)
	if err != nil {
		t.Fatalf("client1 read failed: %v", err)
	}
	if string(got1) != string(resRaw) {
		t.Fatalf("client1 got unexpected payload: %s", string(got1))
	}

	readCtx2, cancel2 := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel2()
	if _, _, err := client2.Read(readCtx2); err == nil {
		t.Fatal("client2 should not receive response routed to client1 conn_id")
	}
}

func TestServer_PublishClientEvent_DeliversToWSClient(t *testing.T) {
	srv, err := NewServer(makeDeps())
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	baseWS := "ws" + strings.TrimPrefix(ts.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, _, err := websocket.Dial(ctx, baseWS+"/ws/client/local", nil)
	if err != nil {
		t.Fatalf("dial client failed: %v", err)
	}
	client.SetReadLimit(-1)
	defer func() { _ = client.Close(websocket.StatusNormalClosure, "") }()

	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			srv.PublishClientEvent("local", "task.messages.updated", "p1", "t1", map[string]any{"source": "auto_progress"})
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}()

	var msg protocol.Message
	for {
		_, raw, err := client.Read(ctx)
		if err != nil {
			t.Fatalf("client read failed: %v", err)
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("unmarshal client event failed: %v", err)
		}
		if msg.Type == "event" && msg.Op == "task.messages.updated" {
			break
		}
	}
	var payload map[string]any
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if strings.TrimSpace(fmt.Sprint(payload["task_id"])) != "t1" {
		t.Fatalf("expected payload.task_id=t1, got %#v", payload["task_id"])
	}
	if strings.TrimSpace(fmt.Sprint(payload["project_id"])) != "p1" {
		t.Fatalf("expected payload.project_id=p1, got %#v", payload["project_id"])
	}
	if strings.TrimSpace(fmt.Sprint(payload["source"])) != "auto_progress" {
		t.Fatalf("expected payload.source=auto_progress, got %#v", payload["source"])
	}
}

func TestServer_ProdStaticFallbackToIndex(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"), []byte("<html>index</html>"), 0o644); err != nil {
		t.Fatalf("write index failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dist, "main.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatalf("write main.js failed: %v", err)
	}

	deps := makeDeps()
	deps.WebUI.Mode = "prod"
	deps.WebUI.DistDir = dist
	srv, err := NewServer(deps)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/unknown/path")
	if err != nil {
		t.Fatalf("GET unknown failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), "index") {
		t.Fatalf("expected index fallback, got %s", string(body))
	}
}
