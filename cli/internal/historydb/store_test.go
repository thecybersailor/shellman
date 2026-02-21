package historydb

import (
	"path/filepath"
	"testing"

	"shellman/cli/internal/projectstate"
)

func TestStore_UpsertAndListRecent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shellman.db")
	if err := projectstate.InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}
	db, err := projectstate.GlobalDBGORM()
	if err != nil {
		t.Fatalf("GlobalDBGORM failed: %v", err)
	}
	st, err := NewStore(db)
	if err != nil {
		t.Fatalf("new store failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	if err := st.Upsert("/tmp/a"); err != nil {
		t.Fatalf("upsert a: %v", err)
	}
	if err := st.Upsert("/tmp/b"); err != nil {
		t.Fatalf("upsert b: %v", err)
	}
	if err := st.Upsert("/tmp/a"); err != nil {
		t.Fatalf("upsert a again: %v", err)
	}

	rows, err := st.List(10)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	countByPath := map[string]int{}
	for _, row := range rows {
		if row.FirstAccessed.Unix() <= 0 || row.LastAccessed.Unix() <= 0 {
			t.Fatalf("expected unix-second timestamps, got row=%+v", row)
		}
		countByPath[row.Path] = row.AccessCount
	}
	if countByPath["/tmp/a"] != 2 || countByPath["/tmp/b"] != 1 {
		t.Fatalf("unexpected counts: %#v", countByPath)
	}

	if err := st.Clear(); err != nil {
		t.Fatalf("clear failed: %v", err)
	}
	rows, err = st.List(10)
	if err != nil {
		t.Fatalf("list after clear failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected empty rows after clear, got %d", len(rows))
	}
}
