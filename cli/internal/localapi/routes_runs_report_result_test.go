package localapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"shellman/cli/internal/global"
	"shellman/cli/internal/projectstate"
)

func TestRunAutoCompleteByPane_FirstAndSecondBothTrigger(t *testing.T) {
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
	if createOut.Data.TaskID == "" {
		t.Fatal("expected task_id")
	}

	runID := "r_1"
	store := projectstate.NewStore(repo)
	if err := store.InsertRun(projectstate.RunRecord{RunID: runID, TaskID: createOut.Data.TaskID, RunStatus: projectstate.RunStatusRunning}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	if err := store.UpsertRunBinding(projectstate.RunBinding{
		RunID:            runID,
		ServerInstanceID: detectServerInstanceID(),
		PaneID:           "e2e:1.1",
		PaneTarget:       "e2e:1.1",
		BindingStatus:    projectstate.BindingStatusLive,
	}); err != nil {
		t.Fatalf("UpsertRunBinding failed: %v", err)
	}
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("LoadPanes failed: %v", err)
	}
	panes[createOut.Data.TaskID] = projectstate.PaneBinding{
		TaskID:     createOut.Data.TaskID,
		PaneUUID:   "pane-uuid-1",
		PaneID:     "e2e:1.1",
		PaneTarget: "e2e:1.1",
	}
	if err := store.SavePanes(panes); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}

	_, runErr := srv.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget: "e2e:1.1",
		Summary:    "done",
	})
	if runErr != nil {
		t.Fatalf("first AutoCompleteByPane failed: %v", runErr)
	}
	out2, runErr := srv.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget: "e2e:1.1",
		Summary:    "done",
	})
	if runErr != nil {
		t.Fatalf("second AutoCompleteByPane failed: %v", runErr)
	}
	if !out2.Triggered {
		t.Fatalf("expected triggered=true on second request, got false (status=%q reason=%q)", out2.Status, out2.Reason)
	}
	if out2.Status != projectstate.StatusCompleted {
		t.Fatalf("expected completed status on second request, got %q", out2.Status)
	}

	run, err := store.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if run.RunStatus != projectstate.RunStatusCompleted {
		t.Fatalf("expected completed run, got %q", run.RunStatus)
	}
	count, err := store.CountOutboxByRunID(runID)
	if err != nil {
		t.Fatalf("CountOutboxByRunID failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one outbox row, got %d", count)
	}
}

func TestAutoCompleteByPane_CompletesTaskWithoutRunningRun(t *testing.T) {
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
	if createOut.Data.TaskID == "" {
		t.Fatal("expected task_id")
	}

	store := projectstate.NewStore(repo)
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("LoadPanes failed: %v", err)
	}
	panes[createOut.Data.TaskID] = projectstate.PaneBinding{
		TaskID:     createOut.Data.TaskID,
		PaneUUID:   "pane-uuid-no-run",
		PaneID:     "e2e:1.1",
		PaneTarget: "e2e:1.1",
	}
	if err := store.SavePanes(panes); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}

	out, runErr := srv.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget: "e2e:1.1",
		Summary:    "done",
	})
	if runErr != nil {
		t.Fatalf("AutoCompleteByPane failed: %v", runErr)
	}
	if !out.Triggered {
		t.Fatalf("expected triggered=true, got false (status=%q reason=%q)", out.Status, out.Reason)
	}
	if out.Status != projectstate.StatusCompleted {
		t.Fatalf("expected completed status, got %q", out.Status)
	}
	if out.TaskID != createOut.Data.TaskID {
		t.Fatalf("expected task_id=%q, got %q", createOut.Data.TaskID, out.TaskID)
	}
	if out.RunID != "" {
		t.Fatalf("expected empty run_id when no running run, got %q", out.RunID)
	}
	if out.Reason == "no-live-running-run" {
		t.Fatalf("unexpected legacy reason no-live-running-run: %#v", out)
	}

	rows, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	byID := map[string]projectstate.TaskRecordRow{}
	for _, row := range rows {
		byID[row.TaskID] = row
	}
	if got := byID[createOut.Data.TaskID].Status; got != projectstate.StatusCompleted {
		t.Fatalf("expected task status completed, got %q", got)
	}
}

func TestAutoCompleteByPane_TriggersWhenTaskAlreadyCompleted(t *testing.T) {
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
	if createOut.Data.TaskID == "" {
		t.Fatal("expected task_id")
	}

	store := projectstate.NewStore(repo)
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("LoadPanes failed: %v", err)
	}
	panes[createOut.Data.TaskID] = projectstate.PaneBinding{
		TaskID:     createOut.Data.TaskID,
		PaneUUID:   "pane-uuid-done",
		PaneID:     "e2e:1.1",
		PaneTarget: "e2e:1.1",
	}
	if err := store.SavePanes(panes); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}

	patchReq, _ := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/api/v1/tasks/"+createOut.Data.TaskID+"/status",
		bytes.NewBufferString(`{"status":"completed"}`),
	)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("PATCH status failed: %v", err)
	}
	_ = patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from PATCH status, got %d", patchResp.StatusCode)
	}

	time.Sleep(20 * time.Millisecond)
	out, runErr := srv.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget: "e2e:1.1",
		Summary:    "done",
	})
	if runErr != nil {
		t.Fatalf("AutoCompleteByPane failed: %v", runErr)
	}
	if !out.Triggered {
		t.Fatalf("expected triggered=true, got false (status=%q reason=%q)", out.Status, out.Reason)
	}
	if out.Status != projectstate.StatusCompleted {
		t.Fatalf("expected completed status, got %q", out.Status)
	}
	if out.TaskID != createOut.Data.TaskID {
		t.Fatalf("expected task_id=%q, got %q", createOut.Data.TaskID, out.TaskID)
	}
}
