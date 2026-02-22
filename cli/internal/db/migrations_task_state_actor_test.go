package db

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

func TestMigrateUp_AddsTaskRuntimeAndTaskColumns(t *testing.T) {
	db := openTestDB(t)
	if err := MigrateUp(db); err != nil {
		t.Fatal(err)
	}

	mustHaveColumns(t, db, "tasks", []string{
		"task_id", "project_id", "parent_task_id", "title", "current_command", "status", "sidecar_mode", "task_role", "description", "flag", "flag_desc", "checked", "archived", "last_modified",
	})
	mustHaveTable(t, db, "pane_runtime")
	mustHaveTable(t, db, "task_runtime")
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := openSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	return db
}

func mustHaveColumns(t *testing.T, db *gorm.DB, table string, columns []string) {
	t.Helper()

	for _, column := range columns {
		if !db.Migrator().HasColumn(table, column) {
			t.Fatalf("table %s missing column %s", table, column)
		}
	}
}

func mustHaveTable(t *testing.T, db *gorm.DB, table string) {
	t.Helper()

	if !db.Migrator().HasTable(table) {
		t.Fatalf("missing table %s", table)
	}
}
