package projectstate

import (
	"path/filepath"
	"testing"
)

func TestRunStore_CreateBindMarkStaleFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "muxt.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}

	repo := t.TempDir()
	st := NewStore(repo)
	taskID := "t_1"
	runID := "r_1"
	if err := st.InsertTask(TaskRecord{TaskID: taskID, ProjectID: "p1", Title: "root"}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertRun(RunRecord{RunID: runID, TaskID: taskID, RunStatus: RunStatusRunning}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertRunBinding(RunBinding{RunID: runID, ServerInstanceID: "srvA", PaneID: "%12", PaneTarget: "botworks:1.0", BindingStatus: BindingStatusLive}); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkBindingsStaleByServer("srvA", "tmux_restarted"); err != nil {
		t.Fatal(err)
	}
	run, err := st.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.RunStatus != RunStatusNeedsRebind {
		t.Fatalf("got %s", run.RunStatus)
	}
}
