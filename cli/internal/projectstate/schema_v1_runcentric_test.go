package projectstate

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestInitGlobalDB_CreatesRunCentricSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shellman.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mustHave := []string{"tasks", "task_runs", "run_bindings", "run_events", "completion_inbox", "action_outbox", "tmux_servers"}
	for _, name := range mustHave {
		var got string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&got); err != nil {
			t.Fatalf("missing table %s: %v", name, err)
		}
	}

	rows, err := db.Query(`PRAGMA table_info(tasks)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(tasks) failed: %v", err)
	}
	defer rows.Close()
	hasLastAutoProgressAt := false
	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table_info(tasks) failed: %v", err)
		}
		if name == "last_auto_progress_at" {
			hasLastAutoProgressAt = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table_info(tasks) failed: %v", err)
	}
	if !hasLastAutoProgressAt {
		t.Fatal("expected tasks.last_auto_progress_at column")
	}
}
