package localapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskCompletionAudit_UsesSlogJSONLine(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("SHELLMAN_CONFIG_DIR", configDir); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	lgr := newTaskCompletionAuditLogger()
	if lgr == nil {
		t.Fatal("expected logger")
	}
	defer lgr.Close()

	lgr.Log("trigger.received", map[string]any{"task_id": "t1"})

	b, err := os.ReadFile(filepath.Join(configDir, "logs", "task-completion-automation.log"))
	if err != nil {
		t.Fatalf("read log failed: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"msg":"trigger.received"`) {
		t.Fatalf("expected slog msg field, got %s", got)
	}
}
