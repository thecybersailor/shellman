package localapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskCompletionAudit_UsesSlogJSONLine(t *testing.T) {
	repo := t.TempDir()
	lgr := newTaskCompletionAuditLogger(repo)
	if lgr == nil {
		t.Fatal("expected logger")
	}
	defer lgr.Close()

	lgr.Log("trigger.received", map[string]any{"task_id": "t1"})

	b, err := os.ReadFile(filepath.Join(repo, ".muxt", "logs", "task-completion-automation.log"))
	if err != nil {
		t.Fatalf("read log failed: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"msg":"trigger.received"`) {
		t.Fatalf("expected slog msg field, got %s", got)
	}
}
