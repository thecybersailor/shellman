package projectstate

import (
	"fmt"
	"testing"
	"time"
)

func TestInitGlobalDBWithDSN_InMemorySQLite(t *testing.T) {
	dsn := fmt.Sprintf("file:ps_mem_%d?mode=memory&cache=shared", time.Now().UnixNano())
	if err := InitGlobalDBWithDSN(dsn); err != nil {
		t.Fatalf("InitGlobalDBWithDSN failed: %v", err)
	}

	repo := t.TempDir()
	store := NewStore(repo)
	now := time.Now().UTC().Unix()
	taskID := fmt.Sprintf("t_mem_%d", now)
	if err := store.UpsertTaskMeta(TaskMetaUpsert{
		TaskID:       taskID,
		ProjectID:    "p1",
		Title:        strPtr("memory task"),
		LastModified: now,
	}); err != nil {
		t.Fatalf("UpsertTaskMeta failed: %v", err)
	}

	rows, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 task row, got %d", len(rows))
	}
	if rows[0].TaskID != taskID {
		t.Fatalf("expected task id %q, got %q", taskID, rows[0].TaskID)
	}
}

func strPtr(v string) *string { return &v }
