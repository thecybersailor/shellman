package localapi

import (
	"path/filepath"
	"strings"
	"testing"

	"shellman/cli/internal/global"
	"shellman/cli/internal/projectstate"
)

func TestBuildUserPromptWithMeta_ReturnsHistoryMetrics(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{
		projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}},
	}
	srv := NewServer(Deps{
		ConfigStore:   &staticConfigStore{},
		ProjectsStore: projects,
	})
	taskID, err := srv.createTask("p1", "", "root")
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	store := projectstate.NewStore(filepath.Clean(repo))
	if _, err := store.InsertTaskMessage(taskID, "user", "hello", projectstate.StatusCompleted, ""); err != nil {
		t.Fatalf("insert user message failed: %v", err)
	}
	if _, err := store.InsertTaskMessage(taskID, "assistant", `{"text":"world"}`, projectstate.StatusCompleted, ""); err != nil {
		t.Fatalf("insert assistant message failed: %v", err)
	}

	prompt, meta := srv.buildUserPromptWithMeta(taskID, "next")
	if strings.TrimSpace(prompt) == "" {
		t.Fatal("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "conversation_history:") {
		t.Fatalf("expected prompt contains conversation_history, got %q", prompt)
	}
	if meta.TotalMessages < 2 {
		t.Fatalf("expected total messages >=2, got %d", meta.TotalMessages)
	}
	if meta.Included <= 0 {
		t.Fatalf("expected included messages > 0, got %d", meta.Included)
	}
	if meta.TotalMessages < meta.Included {
		t.Fatalf("invalid metrics total=%d included=%d", meta.TotalMessages, meta.Included)
	}
}

func TestBuildUserPromptWithMeta_IncludesCursorFromPaneRuntime(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{
		projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}},
	}
	srv := NewServer(Deps{
		ConfigStore:   &staticConfigStore{},
		ProjectsStore: projects,
	})
	taskID, err := srv.createTask("p1", "", "root")
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	store := projectstate.NewStore(filepath.Clean(repo))
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("LoadPanes failed: %v", err)
	}
	panes[taskID] = projectstate.PaneBinding{
		TaskID:     taskID,
		PaneUUID:   "pane-uuid-1",
		PaneID:     "pane-1",
		PaneTarget: "pane-1",
	}
	if err := store.SavePanes(panes); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}
	if err := store.BatchUpsertRuntime(projectstate.RuntimeBatchUpdate{
		Panes: []projectstate.PaneRuntimeRecord{{
			PaneID:         "pane-1",
			PaneTarget:     "pane-1",
			CurrentCommand: "bash",
			Snapshot:       "line-1\nline-2",
			SnapshotHash:   "h1",
			CursorX:        17,
			CursorY:        8,
			HasCursor:      true,
		}},
	}); err != nil {
		t.Fatalf("BatchUpsertRuntime failed: %v", err)
	}

	prompt, _ := srv.buildUserPromptWithMeta(taskID, "next")
	required := []string{
		"\"cursor\":{\"col\":17,\"row\":8,\"visible\":true}",
		"\"cursor_hint\":\"cursor_on_terminal_screen\"",
		"\"cursor_semantic\":\"command_typing\"",
		"\"terminal_screen_state\"",
	}
	for _, item := range required {
		if !strings.Contains(prompt, item) {
			t.Fatalf("expected prompt contains %q, got %q", item, prompt)
		}
	}
}
