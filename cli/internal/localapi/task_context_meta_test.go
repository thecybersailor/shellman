package localapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestBuildUserPromptWithMeta_LoadsTaskCompletionContext_RepoFirst(t *testing.T) {
	repo := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("SHELLMAN_CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(repo, "AGENTS-SIDECAR.md"), []byte("repo-context"), 0o644); err != nil {
		t.Fatalf("write repo AGENTS-SIDECAR.md failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "AGENTS-SIDECAR.md"), []byte("config-context"), 0o644); err != nil {
		t.Fatalf("write config AGENTS-SIDECAR.md failed: %v", err)
	}

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

	prompt, _ := srv.buildUserPromptWithMeta(taskID, "next")
	if !strings.Contains(prompt, `"path":"`+filepath.ToSlash(filepath.Join(repo, "AGENTS-SIDECAR.md"))+`"`) {
		t.Fatalf("expected repo context path injected, got %q", prompt)
	}
	if !strings.Contains(prompt, `"content":"repo-context"`) {
		t.Fatalf("expected repo context injected, got %q", prompt)
	}
	if strings.Contains(prompt, "config-context") {
		t.Fatalf("expected config context not used when repo context exists, got %q", prompt)
	}
}

func TestBuildAutoProgressPromptInput_LoadsTaskCompletionContext_ConfigFallback(t *testing.T) {
	repo := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("SHELLMAN_CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "AGENTS-SIDECAR.md"), []byte("config-context"), 0o644); err != nil {
		t.Fatalf("write config AGENTS-SIDECAR.md failed: %v", err)
	}

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

	input := srv.buildAutoProgressPromptInput("p1", taskID, "done", "")
	prompt := buildTaskAgentAutoProgressPrompt(input)
	if !strings.Contains(prompt, `"path":"`+filepath.ToSlash(filepath.Join(configDir, "AGENTS-SIDECAR.md"))+`"`) {
		t.Fatalf("expected config context path injected for auto progress prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, `"content":"config-context"`) {
		t.Fatalf("expected config context injected for auto progress prompt, got %q", prompt)
	}
}

func TestBuildUserPromptWithMeta_TaskCompletionContextCache_HitWhenModTimeUnchanged(t *testing.T) {
	repo := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("SHELLMAN_CONFIG_DIR", configDir)
	contextPath := filepath.Join(repo, "AGENTS-SIDECAR.md")
	if err := os.WriteFile(contextPath, []byte("repo-v1"), 0o644); err != nil {
		t.Fatalf("write AGENTS-SIDECAR.md failed: %v", err)
	}
	fixed := time.Unix(1735689600, 0)
	if err := os.Chtimes(contextPath, fixed, fixed); err != nil {
		t.Fatalf("chtimes v1 failed: %v", err)
	}

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

	firstPrompt, _ := srv.buildUserPromptWithMeta(taskID, "next")
	if !strings.Contains(firstPrompt, `"content":"repo-v1"`) {
		t.Fatalf("expected first prompt use repo-v1, got %q", firstPrompt)
	}

	if err := os.WriteFile(contextPath, []byte("repo-v2"), 0o644); err != nil {
		t.Fatalf("rewrite AGENTS-SIDECAR.md failed: %v", err)
	}
	if err := os.Chtimes(contextPath, fixed, fixed); err != nil {
		t.Fatalf("chtimes v2 failed: %v", err)
	}

	secondPrompt, _ := srv.buildUserPromptWithMeta(taskID, "next")
	if !strings.Contains(secondPrompt, `"content":"repo-v1"`) {
		t.Fatalf("expected cache hit keeps repo-v1 when modTime unchanged, got %q", secondPrompt)
	}
	if strings.Contains(secondPrompt, `"content":"repo-v2"`) {
		t.Fatalf("expected no refresh when modTime unchanged, got %q", secondPrompt)
	}
}

func TestBuildUserPromptWithMeta_TaskCompletionContextCache_RefreshWhenModTimeChanged(t *testing.T) {
	repo := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("SHELLMAN_CONFIG_DIR", configDir)
	contextPath := filepath.Join(repo, "AGENTS-SIDECAR.md")
	if err := os.WriteFile(contextPath, []byte("repo-v1"), 0o644); err != nil {
		t.Fatalf("write AGENTS-SIDECAR.md failed: %v", err)
	}
	oldTime := time.Unix(1735689600, 0)
	newTime := oldTime.Add(2 * time.Second)
	if err := os.Chtimes(contextPath, oldTime, oldTime); err != nil {
		t.Fatalf("chtimes old failed: %v", err)
	}

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

	firstPrompt, _ := srv.buildUserPromptWithMeta(taskID, "next")
	if !strings.Contains(firstPrompt, `"content":"repo-v1"`) {
		t.Fatalf("expected first prompt use repo-v1, got %q", firstPrompt)
	}

	if err := os.WriteFile(contextPath, []byte("repo-v2"), 0o644); err != nil {
		t.Fatalf("rewrite AGENTS-SIDECAR.md failed: %v", err)
	}
	if err := os.Chtimes(contextPath, newTime, newTime); err != nil {
		t.Fatalf("chtimes new failed: %v", err)
	}

	secondPrompt, _ := srv.buildUserPromptWithMeta(taskID, "next")
	if !strings.Contains(secondPrompt, `"content":"repo-v2"`) {
		t.Fatalf("expected refreshed prompt use repo-v2 when modTime changed, got %q", secondPrompt)
	}
}
