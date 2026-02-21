package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenSQLiteWithMigrations_CreatesCoreTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "muxt.db")
	sqlDB, err := OpenSQLiteWithMigrations(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteWithMigrations failed: %v", err)
	}
	defer sqlDB.Close()

	mustHave := []string{
		"tasks",
		"task_runs",
		"run_bindings",
		"run_events",
		"completion_inbox",
		"notes",
		"task_messages",
		"action_outbox",
		"tmux_servers",
		"legacy_state",
		"dir_history",
		"pane_runtime",
		"task_runtime",
		"config",
	}
	for _, name := range mustHave {
		var got string
		if err := sqlDB.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&got); err != nil {
			t.Fatalf("missing table %s: %v", name, err)
		}
	}
}

func TestOpenSQLiteWithMigrations_IsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "muxt.db")
	sqlDB, err := OpenSQLiteWithMigrations(dbPath)
	if err != nil {
		t.Fatalf("first open failed: %v", err)
	}
	_ = sqlDB.Close()

	sqlDB, err = OpenSQLiteWithMigrations(dbPath)
	if err != nil {
		t.Fatalf("second open failed: %v", err)
	}
	defer sqlDB.Close()

	var n int
	if err := sqlDB.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name='tasks'`).Scan(&n); err != nil {
		t.Fatalf("count tasks table failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected tasks table after second open, got count %d", n)
	}
}

func TestOpenSQLiteWithMigrations_OpensReadableDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "muxt.db")
	sqlDB, err := OpenSQLiteWithMigrations(dbPath)
	if err != nil {
		t.Fatalf("open failed: %v", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	var value sql.NullString
	if err := sqlDB.QueryRow(`PRAGMA journal_mode;`).Scan(&value); err != nil {
		t.Fatalf("read pragma journal mode failed: %v", err)
	}
	if !value.Valid || value.String == "" {
		t.Fatal("pragma journal mode should not be empty")
	}
}
