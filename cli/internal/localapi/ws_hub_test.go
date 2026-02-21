package localapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"termteam/cli/internal/global"
	"termteam/cli/internal/protocol"
)

func TestWSHub(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: &fakePaneService{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + ts.URL[len("http"):] + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	srv.hub.Publish("task.tree.updated", "p1", "t1", map[string]any{"version": 1})
	_, msg, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read ws failed: %v", err)
	}
	var evt protocol.Message
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("decode ws event failed: %v", err)
	}
	if evt.Type != "event" || evt.Op != "task.tree.updated" {
		t.Fatalf("expected protocol event op=task.tree.updated, got %s", string(msg))
	}
	if json.Valid(msg) {
		var top map[string]any
		_ = json.Unmarshal(msg, &top)
		if _, ok := top["topic"]; ok {
			t.Fatalf("expected no legacy topic field in protocol envelope, got %s", string(msg))
		}
	}

	srv.hub.Publish("pane.created", "p1", "t2", map[string]any{"relation": "sibling"})
	_, msg, err = conn.Read(ctx)
	if err != nil {
		t.Fatalf("read ws failed: %v", err)
	}
	if err := json.Unmarshal(msg, &evt); err != nil {
		t.Fatalf("decode ws event failed: %v", err)
	}
	if evt.Type != "event" || evt.Op != "pane.created" {
		t.Fatalf("expected protocol event op=pane.created, got %s", string(msg))
	}
}
