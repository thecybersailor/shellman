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

func TestRunRoutes_Removed_Return404(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: repo}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	var created struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create task failed: %v", err)
	}
	if created.Data.TaskID == "" {
		t.Fatal("expected task_id")
	}

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/tasks/" + created.Data.TaskID + "/runs", body: `{}`},
		{method: http.MethodGet, path: "/api/v1/runs/r_1", body: ""},
		{method: http.MethodGet, path: "/api/v1/runs/r_1/events", body: ""},
		{method: http.MethodPost, path: "/api/v1/runs/r_1/bind-pane", body: `{}`},
		{method: http.MethodPost, path: "/api/v1/runs/r_1/resume", body: `{}`},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest(tc.method, ts.URL+tc.path, bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s failed: %v", tc.method, tc.path, err)
		}
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for %s %s, got %d", tc.method, tc.path, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

func TestRunAutoCompleteByPane_SkipsOnlyPaneActorSourceWhenSidecarModeAdvisor(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: repo}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createTaskResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	defer func() { _ = createTaskResp.Body.Close() }()
	var created struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createTaskResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create task failed: %v", err)
	}
	patchReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+created.Data.TaskID+"/sidecar-mode", bytes.NewBufferString(`{"sidecar_mode":"advisor"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("set sidecar-mode advisor failed: %v", err)
	}
	defer func() { _ = patchResp.Body.Close() }()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected sidecar-mode patch 200, got %d", patchResp.StatusCode)
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
		t.Fatalf("expected triggered=false when sidecar_mode=advisor for pane-actor source")
	}
	if out.Status != "skipped" {
		t.Fatalf("expected skipped status, got %q", out.Status)
	}
	if out.Reason != "sidecar-mode-advisor" {
		t.Fatalf("expected reason sidecar-mode-advisor, got %q", out.Reason)
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
	defer func() { _ = createTaskResp.Body.Close() }()
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
	patchReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+created.Data.TaskID+"/sidecar-mode", bytes.NewBufferString(`{"sidecar_mode":"autopilot"}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("set sidecar-mode failed: %v", err)
	}
	defer func() { _ = patchResp.Body.Close() }()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected sidecar-mode patch 200, got %d", patchResp.StatusCode)
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
