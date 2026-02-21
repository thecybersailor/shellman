package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"shellman/cli/internal/appserver"
	"shellman/cli/internal/global"
	"shellman/cli/internal/localapi"
	"shellman/cli/internal/protocol"
	"shellman/cli/internal/turn"
)

type localWSConfigStore struct{}

func (s *localWSConfigStore) LoadOrInit() (global.GlobalConfig, error) {
	return global.GlobalConfig{LocalPort: 4621}, nil
}

func (s *localWSConfigStore) Save(cfg global.GlobalConfig) error { return nil }

type localWSProjectsStore struct{}

func (s *localWSProjectsStore) ListProjects() ([]global.ActiveProject, error) { return nil, nil }
func (s *localWSProjectsStore) AddProject(project global.ActiveProject) error { return nil }
func (s *localWSProjectsStore) RemoveProject(projectID string) error          { return nil }

type localWSPaneService struct{}

func (s *localWSPaneService) CreateSiblingPane(targetTaskID string) (string, error) {
	return "e2e:0.1", nil
}
func (s *localWSPaneService) CreateChildPane(targetTaskID string) (string, error) {
	return "e2e:0.2", nil
}
func (s *localWSPaneService) CreateRootPane() (string, error) { return "e2e:0.0", nil }

func TestStartLocalAgentLoop_RespondsToTmuxList(t *testing.T) {
	srv, err := appserver.NewServer(appserver.Deps{
		LocalAPI: localapi.Deps{
			ConfigStore:   &localWSConfigStore{},
			ProjectsStore: &localWSProjectsStore{},
			PaneService:   &localWSPaneService{},
		},
		WebUI: appserver.WebUIConfig{Mode: "dev", DevProxyURL: "http://127.0.0.1:15173"},
	})
	if err != nil {
		t.Fatalf("new app server failed: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse url failed: %v", err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse port failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = startLocalAgentLoop(ctx, port, turn.RealDialer{}, &fakeTmux{}, nil, nil, testLogger())
	}()

	wsURL := "ws" + ts.URL[len("http"):] + "/ws/client/local"
	clientCtx, clientCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer clientCancel()
	client, _, err := websocket.Dial(clientCtx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial client ws failed: %v", err)
	}
	defer client.Close(websocket.StatusNormalClosure, "")

	req := protocol.Message{
		ID:      "req_1",
		Type:    "req",
		Op:      "tmux.list",
		Payload: protocol.MustRaw(map[string]any{"scope": "all"}),
	}
	raw, _ := json.Marshal(req)
	if err := client.Write(clientCtx, websocket.MessageText, raw); err != nil {
		t.Fatalf("write tmux.list failed: %v", err)
	}

	_, msgRaw, err := client.Read(clientCtx)
	if err != nil {
		t.Fatalf("read tmux.list response failed: %v", err)
	}
	var msg protocol.Message
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if msg.Type != "res" || msg.Op != "tmux.list" {
		t.Fatalf("unexpected response: %s", string(msgRaw))
	}
}

func TestBuildGatewayHTTPExecutor_ProvidesFSRoutes(t *testing.T) {
	cfgDir := t.TempDir()
	exec := buildGatewayHTTPExecutor(cfgDir, &fakeTmux{}, nil)

	status, _, body, err := exec("GET", "/api/v1/fs/roots", nil, "")
	if err != nil {
		t.Fatalf("exec failed: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d body=%s", status, body)
	}
	if !strings.Contains(body, "\"ok\":true") {
		t.Fatalf("unexpected body: %s", body)
	}
}
