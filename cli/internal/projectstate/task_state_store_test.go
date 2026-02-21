package projectstate

import (
	"path/filepath"
	"testing"
)

func TestTaskStateStore_ListTasksByProject_BuildsTreeSourceRows(t *testing.T) {
	st := newTaskStateStore(t)
	seedTasks(t, st)

	rows, err := st.ListTasksByProject("p1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3, got %d", len(rows))
	}
}

func TestTaskStateStore_ListTasksByProject_OrdersByCreatedAtAsc(t *testing.T) {
	st := newTaskStateStore(t)
	if err := st.InsertTask(TaskRecord{
		TaskID:       "t_new",
		ProjectID:    "p1",
		Title:        "new",
		Status:       StatusPending,
		LastModified: 99,
	}); err != nil {
		t.Fatalf("InsertTask(t_new) failed: %v", err)
	}
	if err := st.InsertTask(TaskRecord{
		TaskID:       "t_old",
		ProjectID:    "p1",
		Title:        "old",
		Status:       StatusPending,
		LastModified: 1,
	}); err != nil {
		t.Fatalf("InsertTask(t_old) failed: %v", err)
	}

	rows, err := st.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2, got %d", len(rows))
	}
	if rows[0].TaskID != "t_new" || rows[1].TaskID != "t_old" {
		t.Fatalf("expected created-order [t_new,t_old], got [%s,%s]", rows[0].TaskID, rows[1].TaskID)
	}
}

func TestTaskStateStore_BatchUpsertRuntime_OnlyUpdatesGivenRecords(t *testing.T) {
	st := newTaskStateStore(t)

	err := st.BatchUpsertRuntime(RuntimeBatchUpdate{Panes: []PaneRuntimeRecord{{PaneID: "p1", SnapshotHash: "h1"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := st.GetPaneRuntimeByPaneID("p1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected pane runtime")
	}
	if got.SnapshotHash != "h1" {
		t.Fatalf("unexpected snapshot hash: %s", got.SnapshotHash)
	}

	err = st.BatchUpsertRuntime(RuntimeBatchUpdate{Panes: []PaneRuntimeRecord{{PaneID: "p2", SnapshotHash: "h2"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err = st.GetPaneRuntimeByPaneID("p1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected pane runtime after unrelated upsert")
	}
	if got.SnapshotHash != "h1" {
		t.Fatalf("p1 should stay h1, got %s", got.SnapshotHash)
	}
}

func TestTaskStateStore_BatchUpsertRuntime_UpdatesTaskCurrentCommand(t *testing.T) {
	st := newTaskStateStore(t)
	if err := st.InsertTask(TaskRecord{
		TaskID:    "t1",
		ProjectID: "p1",
		Title:     "root",
		Status:    StatusRunning,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}

	if err := st.BatchUpsertRuntime(RuntimeBatchUpdate{
		Tasks: []TaskRuntimeRecord{{
			TaskID:         "t1",
			SourcePaneID:   "e2e:0.0",
			CurrentCommand: "npm test",
			RuntimeStatus:  "running",
			SnapshotHash:   "h1",
		}},
	}); err != nil {
		t.Fatalf("BatchUpsertRuntime failed: %v", err)
	}

	rows, err := st.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one task row, got %d", len(rows))
	}
	if rows[0].CurrentCommand != "npm test" {
		t.Fatalf("expected current_command updated, got %q", rows[0].CurrentCommand)
	}
}

func TestTaskStateStore_ListTasksByProject_IncludesFlagReaded(t *testing.T) {
	st := newTaskStateStore(t)
	if err := st.InsertTask(TaskRecord{
		TaskID:    "t1",
		ProjectID: "p1",
		Title:     "root",
		Status:    StatusPending,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}

	flagReaded := true
	if err := st.UpsertTaskMeta(TaskMetaUpsert{
		TaskID:     "t1",
		ProjectID:  "p1",
		FlagReaded: &flagReaded,
	}); err != nil {
		t.Fatalf("UpsertTaskMeta failed: %v", err)
	}

	rows, err := st.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one task row, got %d", len(rows))
	}
	if !rows[0].FlagReaded {
		t.Fatalf("expected flag_readed=true, got %#v", rows[0])
	}
}

func newTaskStateStore(t *testing.T) *Store {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "shellman.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}
	return NewStore(t.TempDir())
}

func seedTasks(t *testing.T, st *Store) {
	t.Helper()

	tasks := []TaskRecord{
		{TaskID: "t1", ProjectID: "p1", Title: "root", Status: StatusPending},
		{TaskID: "t2", ProjectID: "p1", ParentTaskID: "t1", Title: "child-1", Status: StatusRunning},
		{TaskID: "t3", ProjectID: "p1", ParentTaskID: "t1", Title: "child-2", Status: StatusCompleted},
	}
	for _, task := range tasks {
		if err := st.InsertTask(task); err != nil {
			t.Fatalf("InsertTask(%s) failed: %v", task.TaskID, err)
		}
	}
}
