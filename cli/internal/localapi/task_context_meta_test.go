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
