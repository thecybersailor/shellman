package global

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppProgramsStore_LoadOrInit_CreatesDefaultFile(t *testing.T) {
	dir := t.TempDir()
	store := NewAppProgramsStore(dir)

	cfg, err := store.LoadOrInit()
	if err != nil {
		t.Fatalf("LoadOrInit failed: %v", err)
	}
	if cfg.Version != 1 {
		t.Fatalf("expected version=1, got %d", cfg.Version)
	}
	if len(cfg.Providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(cfg.Providers))
	}
	if _, err := os.Stat(filepath.Join(dir, "app-programs.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected app-programs.yaml not to be created, got err=%v", err)
	}
}

func TestAppProgramsStore_LoadOrInit_IgnoresDiskFileAndReturnsBuiltinProviders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app-programs.yaml")
	if err := os.WriteFile(path, []byte("invalid: [yaml"), 0o644); err != nil {
		t.Fatalf("seed file failed: %v", err)
	}

	cfg, err := NewAppProgramsStore(dir).LoadOrInit()
	if err != nil {
		t.Fatalf("LoadOrInit failed: %v", err)
	}
	if len(cfg.Providers) != 3 {
		t.Fatalf("expected 3 builtin providers, got %d", len(cfg.Providers))
	}
	if cfg.Providers[0].ID != "codex" || cfg.Providers[1].ID != "claude" || cfg.Providers[2].ID != "cursor" {
		t.Fatalf("unexpected builtin providers: %+v", cfg.Providers)
	}
}

func TestAppProgramsStore_Save_IsNoop(t *testing.T) {
	dir := t.TempDir()
	store := NewAppProgramsStore(dir)
	if err := store.Save(AppProgramsConfig{
		Version: 10,
		Providers: []AppProgramProvider{
			{ID: "custom", DisplayName: "custom", Command: "custom"},
		},
	}); err != nil {
		t.Fatalf("Save should be noop without error, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app-programs.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected no app-programs.yaml written, got err=%v", err)
	}
}
