package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	dbmodel "shellman/cli/internal/db"
	"shellman/cli/internal/projectstate"
)

func TestListProjectsForPaneBaseline_PrefersGlobalDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shellman.db")
	if err := projectstate.InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}
	gdb, err := projectstate.GlobalDBGORM()
	if err != nil {
		t.Fatalf("GlobalDBGORM failed: %v", err)
	}
	now := time.Now().UTC().UnixNano()
	if err := gdb.Create(&dbmodel.Project{
		ProjectID:   "p1",
		RepoRoot:    "/tmp/repo1",
		DisplayName: "p1",
		SortOrder:   1,
		Collapsed:   false,
		UpdatedAt:   now,
	}).Error; err != nil {
		t.Fatalf("insert project failed: %v", err)
	}

	invalidConfigDir := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(invalidConfigDir, []byte("x"), 0o644); err != nil {
		t.Fatalf("write invalid config dir marker failed: %v", err)
	}

	projects, err := listProjectsForPaneBaseline(invalidConfigDir, testLogger())
	if err != nil {
		t.Fatalf("listProjectsForPaneBaseline failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected one project from global db, got %d", len(projects))
	}
	if projects[0].ProjectID != "p1" || projects[0].RepoRoot != "/tmp/repo1" {
		t.Fatalf("unexpected project row: %#v", projects[0])
	}
}

func TestLoadPaneRuntimeBaselineIndex_MapsPaneIDAndTarget(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shellman.db")
	if err := projectstate.InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}

	store := projectstate.NewStore("/tmp/repo1")
	if err := store.BatchUpsertRuntime(projectstate.RuntimeBatchUpdate{
		Panes: []projectstate.PaneRuntimeRecord{
			{
				PaneID:        "%10",
				PaneTarget:    "e2e:0.0",
				RuntimeStatus: "running",
				SnapshotHash:  "h1",
				UpdatedAt:     10,
			},
			{
				PaneID:        "%11",
				PaneTarget:    "e2e:0.0",
				RuntimeStatus: "ready",
				SnapshotHash:  "h2",
				UpdatedAt:     20,
			},
		},
	}); err != nil {
		t.Fatalf("BatchUpsertRuntime failed: %v", err)
	}

	index, err := loadPaneRuntimeBaselineIndex(testLogger())
	if err != nil {
		t.Fatalf("loadPaneRuntimeBaselineIndex failed: %v", err)
	}

	rowByID, ok := index["%10"]
	if !ok {
		t.Fatal("expected pane id key %10 in runtime index")
	}
	if rowByID.SnapshotHash != "h1" || rowByID.UpdatedAt != 10 {
		t.Fatalf("unexpected pane id row: %#v", rowByID)
	}

	rowByTarget, ok := index["e2e:0.0"]
	if !ok {
		t.Fatal("expected pane target key e2e:0.0 in runtime index")
	}
	if rowByTarget.SnapshotHash != "h2" || rowByTarget.UpdatedAt != 20 {
		t.Fatalf("expected target key keep newest row, got %#v", rowByTarget)
	}
}

func TestLoadPaneBindingsByRepo_ParsesPanesJSON(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shellman.db")
	if err := projectstate.InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}

	storeA := projectstate.NewStore("/tmp/repoA")
	storeB := projectstate.NewStore("/tmp/repoB")
	if err := storeA.SavePanes(projectstate.PanesIndex{
		"t1": {TaskID: "t1", PaneID: "%1", PaneTarget: "e2e:0.0"},
	}); err != nil {
		t.Fatalf("SavePanes repoA failed: %v", err)
	}
	if err := storeB.SavePanes(projectstate.PanesIndex{
		"t2": {TaskID: "t2", PaneID: "%2", PaneTarget: "e2e:0.1"},
	}); err != nil {
		t.Fatalf("SavePanes repoB failed: %v", err)
	}

	index, err := loadPaneBindingsByRepo(testLogger())
	if err != nil {
		t.Fatalf("loadPaneBindingsByRepo failed: %v", err)
	}
	if len(index) != 2 {
		t.Fatalf("expected 2 repos in pane binding index, got %d", len(index))
	}
	if got := index["/tmp/repoA"]["t1"].PaneTarget; got != "e2e:0.0" {
		t.Fatalf("unexpected repoA pane target: %q", got)
	}
	if got := index["/tmp/repoB"]["t2"].PaneID; got != "%2" {
		t.Fatalf("unexpected repoB pane id: %q", got)
	}
}

func TestLoadTaskLastModifiedIndex_SkipsArchivedTasks(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shellman.db")
	if err := projectstate.InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}

	store := projectstate.NewStore("/tmp/repoA")
	titleLive := "live"
	statusLive := projectstate.StatusRunning
	archivedLive := false
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:       "t_live",
		ProjectID:    "p1",
		Title:        &titleLive,
		Status:       &statusLive,
		Archived:     &archivedLive,
		LastModified: 111,
	}); err != nil {
		t.Fatalf("UpsertTaskMeta t_live failed: %v", err)
	}
	titleArchived := "archived"
	statusArchived := projectstate.StatusCompleted
	archived := true
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:       "t_archived",
		ProjectID:    "p1",
		Title:        &titleArchived,
		Status:       &statusArchived,
		Archived:     &archived,
		LastModified: 222,
	}); err != nil {
		t.Fatalf("UpsertTaskMeta t_archived failed: %v", err)
	}

	index, err := loadTaskLastModifiedIndex(testLogger())
	if err != nil {
		t.Fatalf("loadTaskLastModifiedIndex failed: %v", err)
	}
	if got := index[taskLastModifiedKey("/tmp/repoA", "p1", "t_live")]; got != 111 {
		t.Fatalf("expected last_modified 111 for live task, got %d", got)
	}
	if _, ok := index[taskLastModifiedKey("/tmp/repoA", "p1", "t_archived")]; ok {
		t.Fatal("archived task should be excluded from task last_modified index")
	}
}
