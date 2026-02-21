package db

import (
	"path/filepath"
	"testing"
)

func TestOpenSQLiteWithMigrations_SetsBusyTimeout(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "muxt.db")
	sqlDB, err := OpenSQLiteWithMigrations(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteWithMigrations failed: %v", err)
	}
	defer sqlDB.Close()

	var timeout int
	if err := sqlDB.QueryRow(`PRAGMA busy_timeout;`).Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout failed: %v", err)
	}
	if timeout < 5000 {
		t.Fatalf("expected busy_timeout >= 5000, got %d", timeout)
	}
}

