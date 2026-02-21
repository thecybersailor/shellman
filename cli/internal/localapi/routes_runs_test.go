package localapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"termteam/cli/internal/global"
	"termteam/cli/internal/projectstate"
)

func TestRunRoutes_CreateAndBindPersistForAutoComplete(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: repo}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createTaskResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	defer createTaskResp.Body.Close()
	var created struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createTaskResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create task failed: %v", err)
	}
	if created.Data.TaskID == "" {
		t.Fatal("expected task_id")
	}

	createRunReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tasks/"+created.Data.TaskID+"/runs", bytes.NewBufferString(`{}`))
	createRunReq.Header.Set("Content-Type", "application/json")
	createRunResp, err := http.DefaultClient.Do(createRunReq)
	if err != nil {
		t.Fatalf("create run request failed: %v", err)
	}
	defer createRunResp.Body.Close()
	if createRunResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from create run, got %d", createRunResp.StatusCode)
	}
	var createRunBody struct {
		Data struct {
			RunID string `json:"run_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRunResp.Body).Decode(&createRunBody); err != nil {
		t.Fatalf("decode create run failed: %v", err)
	}
	if createRunBody.Data.RunID == "" {
		t.Fatal("expected run_id")
	}

	store := projectstate.NewStore(filepath.Clean(repo))
	run, err := store.GetRun(createRunBody.Data.RunID)
	if err != nil {
		t.Fatalf("expected run persisted, got err: %v", err)
	}
	if run.RunStatus != projectstate.RunStatusRunning {
		t.Fatalf("expected running run, got %q", run.RunStatus)
	}

	bindReq, _ := http.NewRequest(
		http.MethodPost,
		ts.URL+"/api/v1/runs/"+createRunBody.Data.RunID+"/bind-pane",
		bytes.NewBufferString(`{"pane_target":"botworks:6.0"}`),
	)
	bindReq.Header.Set("Content-Type", "application/json")
	bindResp, err := http.DefaultClient.Do(bindReq)
	if err != nil {
		t.Fatalf("bind pane request failed: %v", err)
	}
	defer bindResp.Body.Close()
	if bindResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from bind pane, got %d", bindResp.StatusCode)
	}

	binding, ok, err := store.GetLiveBindingByRunID(createRunBody.Data.RunID)
	if err != nil {
		t.Fatalf("get live binding failed: %v", err)
	}
	if !ok {
		t.Fatal("expected live binding persisted")
	}
	if binding.PaneTarget != "botworks:6.0" {
		t.Fatalf("expected pane_target botworks:6.0, got %q", binding.PaneTarget)
	}
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("LoadPanes failed: %v", err)
	}
	panes[created.Data.TaskID] = projectstate.PaneBinding{
		TaskID:     created.Data.TaskID,
		PaneUUID:   "pane-uuid-botworks-6-0",
		PaneID:     "botworks:6.0",
		PaneTarget: "botworks:6.0",
	}
	if err := store.SavePanes(panes); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}

	autoResult, autoErr := srv.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget: "botworks:6.0",
	})
	if autoErr != nil {
		t.Fatalf("AutoCompleteByPane failed: %v", autoErr)
	}
	if !autoResult.Triggered {
		t.Fatalf("expected triggered=true, got status=%q reason=%q", autoResult.Status, autoResult.Reason)
	}
}

func TestRunRoutes_AreRegistered(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: repo}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	var created struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create task failed: %v", err)
	}
	runID := "r_route_1"
	store := projectstate.NewStore(filepath.Clean(repo))
	if err := store.InsertRun(projectstate.RunRecord{RunID: runID, TaskID: created.Data.TaskID, RunStatus: projectstate.RunStatusRunning}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/tasks/" + created.Data.TaskID + "/runs", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/runs/" + runID + "/bind-pane", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/runs/" + runID + "/resume", body: `{}`},
		{method: http.MethodGet, path: "/api/v1/runs/" + runID, body: ""},
		{method: http.MethodGet, path: "/api/v1/runs/" + runID + "/events", body: ""},
	}

	for _, tc := range cases {
		req, _ := http.NewRequest(tc.method, ts.URL+tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s failed: %v", tc.method, tc.path, err)
		}
		if resp.StatusCode == http.StatusNotFound {
			t.Fatalf("route missing: %s %s", tc.method, tc.path)
		}
		_ = resp.Body.Close()
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/runs/auto-complete-by-pane", bytes.NewBufferString(`{"pane_target":"missing:0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/v1/runs/auto-complete-by-pane failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected removed route to return 404, got %d", resp.StatusCode)
	}
}

func TestRunAutoCompleteByPane_SkipsOnlyPaneActorSourceWhenAutopilotDisabled(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: repo}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createTaskResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	defer createTaskResp.Body.Close()
	var created struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createTaskResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create task failed: %v", err)
	}
	store := projectstate.NewStore(filepath.Clean(repo))
	panes, loadErr := store.LoadPanes()
	if loadErr != nil {
		t.Fatalf("LoadPanes failed: %v", loadErr)
	}
	panes[created.Data.TaskID] = projectstate.PaneBinding{
		TaskID:     created.Data.TaskID,
		PaneUUID:   "pane-uuid-auto-0",
		PaneID:     "botworks:9.0",
		PaneTarget: "botworks:9.0",
	}
	if err := store.SavePanes(panes); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}

	out, runErr := srv.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget:    "botworks:9.0",
		TriggerSource: "pane-actor",
	})
	if runErr != nil {
		t.Fatalf("AutoCompleteByPane failed: %v", runErr)
	}
	if out.Triggered {
		t.Fatalf("expected triggered=false when autopilot=false for pane-actor source")
	}
	if out.Status != "skipped" {
		t.Fatalf("expected skipped status, got %q", out.Status)
	}
	if out.Reason != "autopilot-disabled" {
		t.Fatalf("expected reason autopilot-disabled, got %q", out.Reason)
	}

	outManual, manualErr := srv.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget: "missing:0.0",
	})
	if manualErr != nil {
		t.Fatalf("manual AutoCompleteByPane failed: %v", manualErr)
	}
	if outManual.Reason != "no-task-pane-binding" {
		t.Fatalf("expected manual request unchanged behavior, got reason=%q", outManual.Reason)
	}
}

func TestRunAutoCompleteByPane_DedupesPaneActorByObservedLastActiveAt(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: repo}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createTaskResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	defer createTaskResp.Body.Close()
	var created struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createTaskResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create task failed: %v", err)
	}
	if created.Data.TaskID == "" {
		t.Fatal("expected task_id")
	}

	store := projectstate.NewStore(filepath.Clean(repo))
	runID := "r_dedupe_1"
	if err := store.InsertRun(projectstate.RunRecord{RunID: runID, TaskID: created.Data.TaskID, RunStatus: projectstate.RunStatusRunning}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}
	if err := store.UpsertRunBinding(projectstate.RunBinding{
		RunID:            runID,
		ServerInstanceID: detectServerInstanceID(),
		PaneID:           "botworks:6.0",
		PaneTarget:       "botworks:6.0",
		BindingStatus:    projectstate.BindingStatusLive,
	}); err != nil {
		t.Fatalf("UpsertRunBinding failed: %v", err)
	}
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("LoadPanes failed: %v", err)
	}
	panes[created.Data.TaskID] = projectstate.PaneBinding{
		TaskID:     created.Data.TaskID,
		PaneUUID:   "pane-uuid-botworks-6-0",
		PaneID:     "botworks:6.0",
		PaneTarget: "botworks:6.0",
	}
	if err := store.SavePanes(panes); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}
	if err := srv.taskAgentSupervisor.SetAutopilot(created.Data.TaskID, true); err != nil {
		t.Fatalf("SetAutopilot failed: %v", err)
	}

	observed := time.Date(2026, 2, 19, 10, 0, 0, 0, time.UTC).Unix()

	out1, runErr := srv.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget:           "botworks:6.0",
		TriggerSource:        "pane-actor",
		ObservedLastActiveAt: observed,
	})
	if runErr != nil {
		t.Fatalf("first AutoCompleteByPane failed: %v", runErr)
	}
	if !out1.Triggered {
		t.Fatalf("expected first request triggered=true, got status=%q reason=%q", out1.Status, out1.Reason)
	}

	out2, runErr := srv.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget:           "botworks:6.0",
		TriggerSource:        "pane-actor",
		ObservedLastActiveAt: observed,
	})
	if runErr != nil {
		t.Fatalf("second AutoCompleteByPane failed: %v", runErr)
	}
	if out2.Triggered {
		t.Fatalf("expected second request triggered=false, got status=%q reason=%q", out2.Status, out2.Reason)
	}
	if out2.Status != "skipped" {
		t.Fatalf("expected second status skipped, got %q", out2.Status)
	}
	if out2.Reason != "duplicate-observed-last-active-at" {
		t.Fatalf("expected second reason duplicate-observed-last-active-at, got %q", out2.Reason)
	}
}
