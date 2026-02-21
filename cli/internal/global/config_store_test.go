package global

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigStore_LoadOrInit_CreatesDefaultFiles(t *testing.T) {
	dir := t.TempDir()
	store := NewConfigStore(dir)

	cfg, err := store.LoadOrInit()
	if err != nil {
		t.Fatalf("LoadOrInit failed: %v", err)
	}
	if cfg.LocalPort != 4621 {
		t.Fatalf("expected default local port 4621, got %d", cfg.LocalPort)
	}

	path := filepath.Join(dir, "config.toml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config.toml to exist: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config.toml failed: %v", err)
	}
	text := string(b)
	if !strings.Contains(text, "local_port = 4621") {
		t.Fatalf("expected local_port in toml, got: %s", text)
	}
	if !strings.Contains(text, "[defaults]") {
		t.Fatalf("expected defaults table in toml, got: %s", text)
	}
	if !strings.Contains(text, "[task_completion]") {
		t.Fatalf("expected task_completion table in toml, got: %s", text)
	}
	if !strings.Contains(text, "session_program = 'shell'") && !strings.Contains(text, "session_program = \"shell\"") {
		t.Fatalf("expected defaults.session_program in toml, got: %s", text)
	}
	if !strings.Contains(text, "helper_program = 'codex'") && !strings.Contains(text, "helper_program = \"codex\"") {
		t.Fatalf("expected defaults.helper_program in toml, got: %s", text)
	}
	if !strings.Contains(text, "notify_enabled = false") {
		t.Fatalf("expected task_completion.notify_enabled=false in toml, got: %s", text)
	}
	if !strings.Contains(text, "notify_idle_duration_seconds = 0") {
		t.Fatalf("expected task_completion.notify_idle_duration_seconds=0 in toml, got: %s", text)
	}
	if cfg.Defaults.SessionProgram != "shell" {
		t.Fatalf("expected session_program=shell, got %q", cfg.Defaults.SessionProgram)
	}
	if cfg.Defaults.HelperProgram != "codex" {
		t.Fatalf("expected helper_program=codex, got %q", cfg.Defaults.HelperProgram)
	}
}
