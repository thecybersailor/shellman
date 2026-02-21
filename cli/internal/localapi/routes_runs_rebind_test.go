package localapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"termteam/cli/internal/global"
	"termteam/cli/internal/projectstate"
)

func TestRunBinding_BecomesStaleWhenServerInstanceChanges(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	var createOut struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createOut); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}

	runID := "r_srv_mismatch"
	store := projectstate.NewStore(repo)
	if err := store.InsertRun(projectstate.RunRecord{RunID: runID, TaskID: createOut.Data.TaskID, RunStatus: projectstate.RunStatusRunning}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}
	if err := store.UpsertRunBinding(projectstate.RunBinding{
		RunID:            runID,
		ServerInstanceID: "srv_old",
		PaneID:           "e2e:0.1",
		PaneTarget:       "e2e:0.1",
		BindingStatus:    projectstate.BindingStatusLive,
	}); err != nil {
		t.Fatalf("UpsertRunBinding failed: %v", err)
	}

	old := os.Getenv("MUXT_SERVER_INSTANCE_ID")
	defer os.Setenv("MUXT_SERVER_INSTANCE_ID", old)
	if err := os.Setenv("MUXT_SERVER_INSTANCE_ID", "srv_new"); err != nil {
		t.Fatalf("Setenv failed: %v", err)
	}

	resp, err := http.Post(ts.URL+"/api/v1/runs/"+runID+"/resume", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("POST resume failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode resume response failed: %v", err)
	}
	if out.Data.Status != projectstate.RunStatusNeedsRebind {
		t.Fatalf("expected needs_rebind status, got %q", out.Data.Status)
	}

	binding, ok, err := store.GetBindingByRunID(runID)
	if err != nil {
		t.Fatalf("GetBindingByRunID failed: %v", err)
	}
	if !ok {
		t.Fatal("expected binding row")
	}
	if binding.BindingStatus != projectstate.BindingStatusStale || binding.StaleReason != "tmux_restarted" {
		t.Fatalf("expected stale binding with tmux_restarted, got %#v", binding)
	}

	run, err := store.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if run.RunStatus != projectstate.RunStatusNeedsRebind {
		t.Fatalf("expected run status needs_rebind, got %q", run.RunStatus)
	}

	eventCount, err := store.CountRunEventsByType(runID, "tmux_restarted")
	if err != nil {
		t.Fatalf("CountRunEventsByType failed: %v", err)
	}
	if eventCount == 0 {
		t.Fatal("expected tmux_restarted run event")
	}
}
