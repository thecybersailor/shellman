package projectstate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestStore_ListTasksByProject_FromSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "muxt.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}

	repo := t.TempDir()
	s := NewStore(repo)
	if err := s.InsertTask(TaskRecord{
		TaskID:    "t1",
		ProjectID: "p1",
		Title:     "root",
		Status:    StatusRunning,
	}); err != nil {
		t.Fatalf("InsertTask root failed: %v", err)
	}
	if err := s.InsertTask(TaskRecord{
		TaskID:       "t2",
		ProjectID:    "p1",
		ParentTaskID: "t1",
		Title:        "child",
		Status:       StatusPending,
	}); err != nil {
		t.Fatalf("InsertTask child failed: %v", err)
	}
	got, err := NewStore(repo).ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 task rows, got %d", len(got))
	}
	byID := map[string]TaskRecordRow{}
	for _, row := range got {
		byID[row.TaskID] = row
	}
	if byID["t1"].Title != "root" || byID["t1"].Status != StatusRunning {
		t.Fatalf("unexpected root row: %#v", byID["t1"])
	}
	if byID["t2"].ParentTaskID != "t1" || byID["t2"].Title != "child" || byID["t2"].Status != StatusPending {
		t.Fatalf("unexpected child row: %#v", byID["t2"])
	}
	if _, err := os.Stat(filepath.Join(repo, ".muxt", "state", "task-tree.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy task-tree.json should not be created, err=%v", err)
	}
}

func TestStore_IsolatesByRepoRoot_InSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "muxt.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}

	repo1 := t.TempDir()
	repo2 := t.TempDir()
	s1 := NewStore(repo1)
	s2 := NewStore(repo2)

	if err := s1.SavePanes(PanesIndex{"t1": {TaskID: "t1", PaneID: "p1", PaneTarget: "p1"}}); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}

	got2, err := s2.LoadPanes()
	if err != nil {
		t.Fatalf("LoadPanes repo2 failed: %v", err)
	}
	if len(got2) != 0 {
		t.Fatalf("repo2 panes should be empty, got %#v", got2)
	}

	got1, err := s1.LoadPanes()
	if err != nil {
		t.Fatalf("LoadPanes repo1 failed: %v", err)
	}
	if got1["t1"].PaneTarget != "p1" {
		t.Fatalf("repo1 panes mismatch: %#v", got1)
	}
}

func TestStore_SaveAndLoadPaneSnapshots_FromSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "muxt.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}

	repo := t.TempDir()
	s := NewStore(repo)
	in := PaneSnapshotsIndex{
		"pane-1": {
			Output:    "hello\n",
			FrameMode: "reset",
			FrameData: "hello\n",
			HasCursor: true,
			CursorX:   1,
			CursorY:   2,
		},
	}
	if err := s.SavePaneSnapshots(in); err != nil {
		t.Fatalf("SavePaneSnapshots failed: %v", err)
	}

	got, err := s.LoadPaneSnapshots()
	if err != nil {
		t.Fatalf("LoadPaneSnapshots failed: %v", err)
	}
	if got["pane-1"].Output != "hello\n" || !got["pane-1"].HasCursor {
		t.Fatalf("unexpected pane snapshots: %#v", got)
	}
}

func TestStore_SavePaneSnapshots_DoesNotUpdateTaskLastModified(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "muxt.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}

	repo := t.TempDir()
	s := NewStore(repo)
	if err := s.InsertTask(TaskRecord{
		TaskID:    "task_1",
		ProjectID: "p1",
		Title:     "root",
		Status:    StatusRunning,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}

	db, release, err := s.db()
	if err != nil {
		t.Fatalf("open db failed: %v", err)
	}
	defer release()
	if _, err := db.Exec(`UPDATE tasks SET last_modified = 1 WHERE task_id = ?`, "task_1"); err != nil {
		t.Fatalf("seed last_modified failed: %v", err)
	}

	if err := s.SavePaneSnapshots(PaneSnapshotsIndex{
		"task_1": {Output: "hello"},
	}); err != nil {
		t.Fatalf("SavePaneSnapshots failed: %v", err)
	}

	var lastModified int64
	if err := db.QueryRow(`SELECT last_modified FROM tasks WHERE task_id = ?`, "task_1").Scan(&lastModified); err != nil {
		t.Fatalf("query last_modified failed: %v", err)
	}
	if lastModified != 1 {
		t.Fatalf("expected task.last_modified unchanged, got %d", lastModified)
	}
}

func TestStore_UpsertTaskMeta_WithFlagFields(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "muxt.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}

	repo := t.TempDir()
	s := NewStore(repo)
	if err := s.InsertTask(TaskRecord{
		TaskID:    "t1",
		ProjectID: "p1",
		Title:     "root",
		Status:    StatusCompleted,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}
	flag := "notify"
	flagDesc := "needs follow-up"
	flagReaded := true
	if err := s.UpsertTaskMeta(TaskMetaUpsert{
		TaskID:       "t1",
		ProjectID:    "p1",
		Flag:         &flag,
		FlagDesc:     &flagDesc,
		FlagReaded:   &flagReaded,
		LastModified: 9,
	}); err != nil {
		t.Fatalf("UpsertTaskMeta failed: %v", err)
	}
	got, err := NewStore(repo).ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("unexpected task rows: %#v", got)
	}
	if got[0].Flag != "notify" || got[0].FlagDesc != "needs follow-up" {
		t.Fatalf("unexpected flag fields: %#v", got[0])
	}
	if !got[0].FlagReaded {
		t.Fatalf("expected flag_readed=true, got %#v", got[0])
	}
	if got[0].LastModified != 9 {
		t.Fatalf("expected last_modified=9, got %d", got[0].LastModified)
	}
}

func TestNoLegacyTaskTreeJSONPathRemaining(t *testing.T) {
	cliRoot := filepath.Clean(filepath.Join("..", ".."))
	forbidden := []string{
		"task_tree_json",
		"task_index_json",
		"SaveTaskTree",
		"LoadTaskTree",
		"SaveTaskIndex",
		"LoadTaskIndex",
	}

	hits := make([]string, 0)
	err := filepath.WalkDir(cliRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(raw)
		for _, token := range forbidden {
			if strings.Contains(content, token) {
				hits = append(hits, fmt.Sprintf("%s contains %q", path, token))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir failed: %v", err)
	}
	if len(hits) != 0 {
		sort.Strings(hits)
		t.Fatalf("legacy task tree/index path remains:\n%s", strings.Join(hits, "\n"))
	}
}
