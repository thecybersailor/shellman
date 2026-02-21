package projectstate

import (
	"path/filepath"
	"testing"
)

func TestTryMarkTaskAutoProgressObserved_DedupByObservedTimestamp(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shellman.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}
	repo := t.TempDir()
	store := NewStore(repo)

	if err := store.InsertTask(TaskRecord{
		TaskID:    "t1",
		ProjectID: "p1",
		Title:     "root",
		Status:    StatusRunning,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}

	ok, err := store.TryMarkTaskAutoProgressObserved(TaskRecord{TaskID: "t1"}, 1000)
	if err != nil {
		t.Fatalf("TryMarkTaskAutoProgressObserved first failed: %v", err)
	}
	if !ok {
		t.Fatal("expected first mark accepted")
	}

	ok, err = store.TryMarkTaskAutoProgressObserved(TaskRecord{TaskID: "t1"}, 1000)
	if err != nil {
		t.Fatalf("TryMarkTaskAutoProgressObserved second failed: %v", err)
	}
	if ok {
		t.Fatal("expected same timestamp to be deduped")
	}

	ok, err = store.TryMarkTaskAutoProgressObserved(TaskRecord{TaskID: "t1"}, 999)
	if err != nil {
		t.Fatalf("TryMarkTaskAutoProgressObserved lower failed: %v", err)
	}
	if ok {
		t.Fatal("expected lower timestamp to be deduped")
	}

	ok, err = store.TryMarkTaskAutoProgressObserved(TaskRecord{TaskID: "t1"}, 2000)
	if err != nil {
		t.Fatalf("TryMarkTaskAutoProgressObserved higher failed: %v", err)
	}
	if !ok {
		t.Fatal("expected higher timestamp accepted")
	}
}

func TestTryMarkTaskAutoProgressObserved_InsertsWhenTaskRowMissing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shellman.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}
	repo := t.TempDir()
	store := NewStore(repo)

	ok, err := store.TryMarkTaskAutoProgressObserved(TaskRecord{
		TaskID:       "t_missing",
		ProjectID:    "p1",
		ParentTaskID: "",
		Title:        "root",
		Status:       StatusRunning,
	}, 3000)
	if err != nil {
		t.Fatalf("TryMarkTaskAutoProgressObserved first failed: %v", err)
	}
	if !ok {
		t.Fatal("expected first mark accepted when task row is missing")
	}

	ok, err = store.TryMarkTaskAutoProgressObserved(TaskRecord{TaskID: "t_missing"}, 3000)
	if err != nil {
		t.Fatalf("TryMarkTaskAutoProgressObserved duplicate failed: %v", err)
	}
	if ok {
		t.Fatal("expected duplicate timestamp deduped after implicit insert")
	}
}

func TestInsertAndListTaskNotes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shellman.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}
	repo := t.TempDir()
	store := NewStore(repo)

	if err := store.InsertTask(TaskRecord{
		TaskID:    "t1",
		ProjectID: "p1",
		Title:     "root",
		Status:    StatusRunning,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}
	if err := store.InsertTaskNote("t1", "- first note", "success"); err != nil {
		t.Fatalf("InsertTaskNote failed: %v", err)
	}
	if err := store.InsertTaskNote("t1", "- second note", "notify"); err != nil {
		t.Fatalf("InsertTaskNote second failed: %v", err)
	}

	items, err := store.ListTaskNotes("t1", 10)
	if err != nil {
		t.Fatalf("ListTaskNotes failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(items))
	}
	if items[0].TaskID != "t1" || items[0].Notes == "" {
		t.Fatalf("unexpected latest note: %#v", items[0])
	}
	if items[0].Flag == "" {
		t.Fatalf("expected note flag persisted, got %#v", items[0])
	}
}

func TestInsertAndListTaskMessages(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shellman.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}
	repo := t.TempDir()
	store := NewStore(repo)

	if err := store.InsertTask(TaskRecord{
		TaskID:    "t1",
		ProjectID: "p1",
		Title:     "root",
		Status:    StatusRunning,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}

	msgID, err := store.InsertTaskMessage("t1", "user", "hello", StatusCompleted, "")
	if err != nil {
		t.Fatalf("InsertTaskMessage failed: %v", err)
	}
	if msgID <= 0 {
		t.Fatalf("expected positive message id, got %d", msgID)
	}

	items, err := store.ListTaskMessages("t1", 100)
	if err != nil {
		t.Fatalf("ListTaskMessages failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 message, got %d", len(items))
	}
	if items[0].Role != "user" || items[0].Content != "hello" || items[0].Status != StatusCompleted {
		t.Fatalf("unexpected messages: %#v", items)
	}
}
