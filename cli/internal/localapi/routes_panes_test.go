package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"shellman/cli/internal/global"
	"shellman/cli/internal/projectstate"
)

type fakePaneService struct {
	siblingCount   int
	childCount     int
	rootCount      int
	lastSibling    string
	lastChild      string
	lastSiblingCWD string
	lastChildCWD   string
	lastRootCWD    string
	closedTargets  []string
	closeErr       error
}

func (f *fakePaneService) CreateSiblingPaneInDir(targetTaskID, cwd string) (string, error) {
	f.siblingCount++
	f.lastSibling = targetTaskID
	f.lastSiblingCWD = cwd
	return "pane_sibling_1", nil
}

func (f *fakePaneService) CreateChildPaneInDir(targetTaskID, cwd string) (string, error) {
	f.childCount++
	f.lastChild = targetTaskID
	f.lastChildCWD = cwd
	return "pane_child_1", nil
}

func (f *fakePaneService) CreateRootPaneInDir(cwd string) (string, error) {
	f.rootCount++
	f.lastRootCWD = cwd
	return "pane_root_1", nil
}

func (f *fakePaneService) ClosePane(target string) error {
	if f.closeErr != nil {
		return f.closeErr
	}
	f.closedTargets = append(f.closedTargets, target)
	return nil
}

func TestPaneCreationRoutes(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{
		ConfigStore:   &staticConfigStore{},
		ProjectsStore: projects,
		PaneService:   paneSvc,
		ExecuteCommand: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("bash\n"), nil
		},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody := bytes.NewBufferString(`{"project_id":"p1","title":"root"}`)
	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", createBody)
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	var createRes struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createRes); err != nil {
		t.Fatalf("decode create failed: %v", err)
	}
	rootID := createRes.Data.TaskID

	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+rootID+"/panes/sibling", "application/json", bytes.NewBufferString(`{"title":"sib"}`))
	if err != nil {
		t.Fatalf("POST sibling failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST sibling expected 200, got %d", resp.StatusCode)
	}
	var siblingRes struct {
		Data struct {
			TaskID   string `json:"task_id"`
			PaneUUID string `json:"pane_uuid"`
			PaneID   string `json:"pane_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&siblingRes); err != nil {
		t.Fatalf("decode sibling failed: %v", err)
	}
	if siblingRes.Data.TaskID == "" || siblingRes.Data.PaneID == "" || siblingRes.Data.PaneUUID == "" {
		t.Fatalf("expected task_id, pane_uuid and pane_id, got %#v", siblingRes)
	}
	if _, err := uuid.Parse(siblingRes.Data.PaneUUID); err != nil {
		t.Fatalf("pane_uuid is not valid UUID: %v", err)
	}

	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+rootID+"/panes/child", "application/json", bytes.NewBufferString(`{"title":"child"}`))
	if err != nil {
		t.Fatalf("POST child failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST child expected 200, got %d", resp.StatusCode)
	}
	if paneSvc.lastSiblingCWD != filepath.Clean(repo) {
		t.Fatalf("expected sibling pane cwd=%q, got %q", filepath.Clean(repo), paneSvc.lastSiblingCWD)
	}
	if paneSvc.lastChildCWD != filepath.Clean(repo) {
		t.Fatalf("expected child pane cwd=%q, got %q", filepath.Clean(repo), paneSvc.lastChildCWD)
	}

	store := projectstate.NewStore(repo)
	rows, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 task rows, got %d", len(rows))
	}
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("load panes failed: %v", err)
	}
	if len(panes) != 2 {
		t.Fatalf("expected 2 pane bindings, got %d", len(panes))
	}
}

func TestTaskPaneEndpoint_ReturnsBoundPane(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{
		ConfigStore:   &staticConfigStore{},
		ProjectsStore: projects,
		PaneService:   paneSvc,
		ExecuteCommand: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("bash\n"), nil
		},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	var createRes struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createRes); err != nil {
		t.Fatalf("decode create failed: %v", err)
	}

	// seed root pane binding so sibling/child creation has a real tmux target
	store := projectstate.NewStore(repo)
	if err := store.SavePanes(projectstate.PanesIndex{
		createRes.Data.TaskID: {
			TaskID:     createRes.Data.TaskID,
			PaneUUID:   uuid.NewString(),
			PaneID:     "e2e:0.0",
			PaneTarget: "e2e:0.0",
		},
	}); err != nil {
		t.Fatalf("seed panes failed: %v", err)
	}

	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/panes/sibling", "application/json", bytes.NewBufferString(`{"title":"sib"}`))
	if err != nil {
		t.Fatalf("POST sibling failed: %v", err)
	}
	if paneSvc.lastSibling != "e2e:0.0" {
		t.Fatalf("expected sibling target from pane binding, got %q", paneSvc.lastSibling)
	}
	var siblingRes struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&siblingRes); err != nil {
		t.Fatalf("decode sibling failed: %v", err)
	}

	resp, err = http.Get(ts.URL + "/api/v1/tasks/" + siblingRes.Data.TaskID + "/pane")
	if err != nil {
		t.Fatalf("GET pane failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET pane expected 200, got %d", resp.StatusCode)
	}
	var paneRes struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID     string `json:"task_id"`
			PaneUUID   string `json:"pane_uuid"`
			PaneID     string `json:"pane_id"`
			PaneTarget string `json:"pane_target"`
			CurrentCmd string `json:"current_command"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&paneRes); err != nil {
		t.Fatalf("decode pane failed: %v", err)
	}
	if paneRes.Data.TaskID == "" || paneRes.Data.PaneID == "" || paneRes.Data.PaneTarget == "" || paneRes.Data.PaneUUID == "" {
		t.Fatalf("expected non-empty pane binding payload, got %#v", paneRes.Data)
	}
	if paneRes.Data.CurrentCmd != "bash" {
		t.Fatalf("expected pane response current_command=bash, got %q", paneRes.Data.CurrentCmd)
	}
	if _, err := uuid.Parse(paneRes.Data.PaneUUID); err != nil {
		t.Fatalf("pane_uuid is not valid UUID: %v", err)
	}
	rows, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	found := false
	for _, row := range rows {
		if row.TaskID == siblingRes.Data.TaskID {
			found = true
			if row.CurrentCommand != "bash" {
				t.Fatalf("expected current_command persisted as bash, got %q", row.CurrentCommand)
			}
		}
	}
	if !found {
		t.Fatalf("task %s not found", siblingRes.Data.TaskID)
	}
}

func TestProjectRootPaneCreationRoute(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: paneSvc})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/projects/p1/panes/root", "application/json", bytes.NewBufferString(`{"title":"root pane"}`))
	if err != nil {
		t.Fatalf("POST root pane failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST root pane expected 200, got %d", resp.StatusCode)
	}
	if paneSvc.rootCount != 1 {
		t.Fatalf("expected root pane service called once, got %d", paneSvc.rootCount)
	}
	if paneSvc.lastRootCWD != filepath.Clean(repo) {
		t.Fatalf("expected root pane cwd=%q, got %q", filepath.Clean(repo), paneSvc.lastRootCWD)
	}

	store := projectstate.NewStore(repo)
	rows, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 task row, got %d", len(rows))
	}
	if rows[0].Status != projectstate.StatusRunning {
		t.Fatalf("expected root pane task status running, got %q", rows[0].Status)
	}
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("load panes failed: %v", err)
	}
	if len(panes) != 1 {
		t.Fatalf("expected 1 pane binding, got %d", len(panes))
	}
}

func TestPaneCreate_RejectsPlannerSpawnPlanner(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: paneSvc})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	store := projectstate.NewStore(repo)
	parentTaskID := "t_parent_" + uuid.NewString()
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:    parentTaskID,
		ProjectID: "p1",
		Title:     "planner parent",
		Status:    projectstate.StatusRunning,
		TaskRole:  projectstate.TaskRolePlanner,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}
	if err := store.SavePanes(projectstate.PanesIndex{
		parentTaskID: {
			TaskID:     parentTaskID,
			PaneUUID:   uuid.NewString(),
			PaneID:     "e2e:0.0",
			PaneTarget: "e2e:0.0",
		},
	}); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}

	resp, err := http.Post(
		ts.URL+"/api/v1/tasks/"+parentTaskID+"/panes/child",
		"application/json",
		bytes.NewBufferString(`{"title":"child","task_role":"planner"}`),
	)
	if err != nil {
		t.Fatalf("POST child failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if out.Error.Code != "PLANNER_ONLY_SPAWN_EXECUTOR" {
		t.Fatalf("unexpected error code: %q", out.Error.Code)
	}
}

func TestPaneCreate_RejectsExecutorDelegation(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: paneSvc})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	store := projectstate.NewStore(repo)
	parentTaskID := "t_parent_" + uuid.NewString()
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:    parentTaskID,
		ProjectID: "p1",
		Title:     "executor parent",
		Status:    projectstate.StatusRunning,
		TaskRole:  projectstate.TaskRoleExecutor,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}
	if err := store.SavePanes(projectstate.PanesIndex{
		parentTaskID: {
			TaskID:     parentTaskID,
			PaneUUID:   uuid.NewString(),
			PaneID:     "e2e:0.0",
			PaneTarget: "e2e:0.0",
		},
	}); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}

	resp, err := http.Post(
		ts.URL+"/api/v1/tasks/"+parentTaskID+"/panes/child",
		"application/json",
		bytes.NewBufferString(`{"title":"child","task_role":"executor"}`),
	)
	if err != nil {
		t.Fatalf("POST child failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	var out struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if out.Error.Code != "EXECUTOR_CANNOT_DELEGATE" {
		t.Fatalf("unexpected error code: %q", out.Error.Code)
	}
}

func TestProjectRootPaneCreationRoute_PersistsCurrentCommand(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{
		ConfigStore:   &staticConfigStore{},
		ProjectsStore: projects,
		PaneService:   paneSvc,
		ExecuteCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			return []byte("bash\n"), nil
		},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/projects/p1/panes/root", "application/json", bytes.NewBufferString(`{"title":"root pane"}`))
	if err != nil {
		t.Fatalf("POST root pane failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST root pane expected 200, got %d", resp.StatusCode)
	}

	store := projectstate.NewStore(repo)
	rows, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 task row, got %d", len(rows))
	}
	if rows[0].CurrentCommand != "bash" {
		t.Fatalf("expected current_command persisted as bash, got %q", rows[0].CurrentCommand)
	}
}

func TestPaneCreate_CreatesRunAndLiveBinding(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: paneSvc})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/projects/p1/panes/root", "application/json", bytes.NewBufferString(`{"title":"root pane"}`))
	if err != nil {
		t.Fatalf("POST root pane failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST root pane expected 200, got %d", resp.StatusCode)
	}
	var out struct {
		Data struct {
			TaskID string `json:"task_id"`
			RunID  string `json:"run_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode root pane failed: %v", err)
	}
	if out.Data.TaskID == "" || out.Data.RunID == "" {
		t.Fatalf("expected task_id and run_id, got %#v", out.Data)
	}

	store := projectstate.NewStore(repo)
	run, err := store.GetRun(out.Data.RunID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if run.RunStatus != projectstate.RunStatusRunning {
		t.Fatalf("expected run running, got %q", run.RunStatus)
	}
	binding, ok, err := store.GetLiveBindingByRunID(out.Data.RunID)
	if err != nil {
		t.Fatalf("GetLiveBindingByRunID failed: %v", err)
	}
	if !ok {
		t.Fatal("expected live binding")
	}
	if binding.PaneID == "" || binding.PaneTarget == "" || binding.ServerInstanceID == "" {
		t.Fatalf("unexpected binding: %#v", binding)
	}
}

func TestPaneCreate_ChildSpawnFallbackCompletesAutopilotReadyChild(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: paneSvc})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	prevDelay := childSpawnAutoProgressFallbackDelay
	childSpawnAutoProgressFallbackDelay = 30 * time.Millisecond
	t.Cleanup(func() { childSpawnAutoProgressFallbackDelay = prevDelay })

	createTaskResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	var created struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createTaskResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create task failed: %v", err)
	}

	patchReq, _ := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/api/v1/tasks/"+created.Data.TaskID+"/sidecar-mode",
		bytes.NewBufferString(`{"sidecar_mode":"autopilot"}`),
	)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch sidecar mode failed: %v", err)
	}
	_ = patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected sidecar patch 200, got %d", patchResp.StatusCode)
	}

	store := projectstate.NewStore(repo)
	if err := store.BatchUpsertRuntime(projectstate.RuntimeBatchUpdate{
		Panes: []projectstate.PaneRuntimeRecord{{
			PaneID:         "pane_child_1",
			PaneTarget:     "pane_child_1",
			RuntimeStatus:  "ready",
			Snapshot:       "",
			SnapshotHash:   "h-ready",
			CurrentCommand: "zsh",
			UpdatedAt:      time.Now().UTC().Unix(),
		}},
	}); err != nil {
		t.Fatalf("seed pane runtime failed: %v", err)
	}

	childResp, err := http.Post(ts.URL+"/api/v1/tasks/"+created.Data.TaskID+"/panes/child", "application/json", bytes.NewBufferString(`{"title":"child"}`))
	if err != nil {
		t.Fatalf("POST child failed: %v", err)
	}
	if childResp.StatusCode != http.StatusOK {
		t.Fatalf("POST child expected 200, got %d", childResp.StatusCode)
	}
	var childOut struct {
		Data struct {
			TaskID string `json:"task_id"`
			RunID  string `json:"run_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(childResp.Body).Decode(&childOut); err != nil {
		t.Fatalf("decode child response failed: %v", err)
	}
	if childOut.Data.TaskID == "" || childOut.Data.RunID == "" {
		t.Fatalf("expected child task_id and run_id, got %#v", childOut.Data)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		rows, err := store.ListTasksByProject("p1")
		if err != nil {
			t.Fatalf("ListTasksByProject failed: %v", err)
		}
		completed := false
		for _, row := range rows {
			if row.TaskID == childOut.Data.TaskID && row.Status == projectstate.StatusCompleted {
				completed = true
				break
			}
		}
		if completed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("child task %s was not completed by spawn fallback", childOut.Data.TaskID)
		}
		time.Sleep(20 * time.Millisecond)
	}

	run, err := store.GetRun(childOut.Data.RunID)
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if run.RunStatus != projectstate.RunStatusCompleted {
		t.Fatalf("expected child run completed, got %q", run.RunStatus)
	}
}

func TestPaneCreate_ChildSpawnFallbackSkipsWhenRuntimeNotReady(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: paneSvc})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	prevDelay := childSpawnAutoProgressFallbackDelay
	childSpawnAutoProgressFallbackDelay = 30 * time.Millisecond
	t.Cleanup(func() { childSpawnAutoProgressFallbackDelay = prevDelay })

	createTaskResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	var created struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createTaskResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create task failed: %v", err)
	}

	patchReq, _ := http.NewRequest(
		http.MethodPatch,
		ts.URL+"/api/v1/tasks/"+created.Data.TaskID+"/sidecar-mode",
		bytes.NewBufferString(`{"sidecar_mode":"autopilot"}`),
	)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch sidecar mode failed: %v", err)
	}
	_ = patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected sidecar patch 200, got %d", patchResp.StatusCode)
	}

	store := projectstate.NewStore(repo)
	if err := store.BatchUpsertRuntime(projectstate.RuntimeBatchUpdate{
		Panes: []projectstate.PaneRuntimeRecord{{
			PaneID:         "pane_child_1",
			PaneTarget:     "pane_child_1",
			RuntimeStatus:  "running",
			Snapshot:       "",
			SnapshotHash:   "h-running",
			CurrentCommand: "zsh",
			UpdatedAt:      time.Now().UTC().Unix(),
		}},
	}); err != nil {
		t.Fatalf("seed pane runtime failed: %v", err)
	}

	childResp, err := http.Post(ts.URL+"/api/v1/tasks/"+created.Data.TaskID+"/panes/child", "application/json", bytes.NewBufferString(`{"title":"child"}`))
	if err != nil {
		t.Fatalf("POST child failed: %v", err)
	}
	if childResp.StatusCode != http.StatusOK {
		t.Fatalf("POST child expected 200, got %d", childResp.StatusCode)
	}
	var childOut struct {
		Data struct {
			TaskID string `json:"task_id"`
			RunID  string `json:"run_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(childResp.Body).Decode(&childOut); err != nil {
		t.Fatalf("decode child response failed: %v", err)
	}

	time.Sleep(220 * time.Millisecond)
	rows, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	for _, row := range rows {
		if row.TaskID == childOut.Data.TaskID {
			if row.Status != projectstate.StatusRunning {
				t.Fatalf("expected child status running when runtime not ready, got %q", row.Status)
			}
			return
		}
	}
	t.Fatalf("child task not found: %s", childOut.Data.TaskID)
}

func TestTaskPaneEndpoint_BackfillsPaneUUID(t *testing.T) {
	tid := uniqueTaskID(t, "t1")
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: &fakePaneService{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	store := projectstate.NewStore(repo)
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:    tid,
		ProjectID: "p1",
		Title:     "root",
		Status:    projectstate.StatusRunning,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}
	if err := store.SavePanes(projectstate.PanesIndex{
		tid: {TaskID: tid, PaneID: "e2e:0.0", PaneTarget: "e2e:0.0"},
	}); err != nil {
		t.Fatalf("save panes failed: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/v1/tasks/" + tid + "/pane")
	if err != nil {
		t.Fatalf("GET pane failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET pane expected 200, got %d", resp.StatusCode)
	}
	var paneRes struct {
		Data struct {
			PaneUUID string `json:"pane_uuid"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&paneRes); err != nil {
		t.Fatalf("decode pane failed: %v", err)
	}
	if _, err := uuid.Parse(paneRes.Data.PaneUUID); err != nil {
		t.Fatalf("pane_uuid is not valid UUID: %v", err)
	}

	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("load panes failed: %v", err)
	}
	if _, err := uuid.Parse(panes[tid].PaneUUID); err != nil {
		t.Fatalf("persisted pane_uuid is invalid: %v", err)
	}
}

func TestTaskPaneSnapshot_ReadOnlyEndpoint(t *testing.T) {
	tid := uniqueTaskID(t, "t1")
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: &fakePaneService{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	store := projectstate.NewStore(repo)
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:    tid,
		ProjectID: "p1",
		Title:     "root",
		Status:    projectstate.StatusRunning,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:       tid,
		ProjectID:    "p1",
		LastModified: 1704067200,
	}); err != nil {
		t.Fatalf("seed last_modified failed: %v", err)
	}
	paneUUID := uuid.NewString()
	if err := store.SavePanes(projectstate.PanesIndex{
		tid: {TaskID: tid, PaneUUID: paneUUID, PaneID: "e2e:0.0", PaneTarget: "e2e:0.0"},
	}); err != nil {
		t.Fatalf("save panes failed: %v", err)
	}
	if err := store.BatchUpsertRuntime(projectstate.RuntimeBatchUpdate{
		Panes: []projectstate.PaneRuntimeRecord{{
			PaneID:       "e2e:0.0",
			PaneTarget:   "e2e:0.0",
			Snapshot:     "line-1\nline-2\n",
			SnapshotHash: "h1",
			HasCursor:    true,
			CursorX:      3,
			CursorY:      8,
			UpdatedAt:    1704067200,
		}},
	}); err != nil {
		t.Fatalf("BatchUpsertRuntime failed: %v", err)
	}

	patchBody := bytes.NewBufferString(`{"output":"line-1\nline-2\n","frame":{"mode":"append","data":"line-2\n"},"cursor":{"x":3,"y":8}}`)
	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+tid+"/pane-snapshot", patchBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH pane-snapshot failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("PATCH pane-snapshot expected 404, got %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/api/v1/tasks/" + tid + "/pane")
	if err != nil {
		t.Fatalf("GET pane failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET pane expected 200, got %d", resp.StatusCode)
	}
	var paneRes struct {
		Data struct {
			PaneUUID string `json:"pane_uuid"`
			Snapshot struct {
				Output string `json:"output"`
				Frame  struct {
					Mode string `json:"mode"`
					Data string `json:"data"`
				} `json:"frame"`
				Cursor *struct {
					X int `json:"x"`
					Y int `json:"y"`
				} `json:"cursor"`
			} `json:"snapshot"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&paneRes); err != nil {
		t.Fatalf("decode pane failed: %v", err)
	}
	if paneRes.Data.PaneUUID != paneUUID {
		t.Fatalf("pane uuid mismatch: got %q want %q", paneRes.Data.PaneUUID, paneUUID)
	}
	if paneRes.Data.Snapshot.Output != "line-1\nline-2\n" {
		t.Fatalf("snapshot output mismatch: %#v", paneRes.Data.Snapshot)
	}
	if paneRes.Data.Snapshot.Frame.Mode != "reset" || paneRes.Data.Snapshot.Frame.Data != "line-1\nline-2\n" {
		t.Fatalf("snapshot frame mismatch: %#v", paneRes.Data.Snapshot.Frame)
	}
	if paneRes.Data.Snapshot.Cursor == nil || paneRes.Data.Snapshot.Cursor.X != 3 || paneRes.Data.Snapshot.Cursor.Y != 8 {
		t.Fatalf("snapshot cursor mismatch: %#v", paneRes.Data.Snapshot.Cursor)
	}
	rowsAfter, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject after snapshot failed: %v", err)
	}
	if len(rowsAfter) != 1 {
		t.Fatalf("expected one task row, got %d", len(rowsAfter))
	}
	if rowsAfter[0].LastModified != 1704067200 {
		t.Fatalf("expected task last_modified unchanged, got %d", rowsAfter[0].LastModified)
	}
}

func TestTaskPaneEndpoint_ReadsSnapshotFromPaneRuntime(t *testing.T) {
	tid := uniqueTaskID(t, "t1")
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: &fakePaneService{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	store := projectstate.NewStore(repo)
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:    tid,
		ProjectID: "p1",
		Title:     "root",
		Status:    projectstate.StatusRunning,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}
	if err := store.SavePanes(projectstate.PanesIndex{
		tid: {TaskID: tid, PaneUUID: uuid.NewString(), PaneID: "e2e:0.0", PaneTarget: "e2e:0.0"},
	}); err != nil {
		t.Fatalf("save panes failed: %v", err)
	}
	if err := store.BatchUpsertRuntime(projectstate.RuntimeBatchUpdate{
		Panes: []projectstate.PaneRuntimeRecord{{
			PaneID:       "e2e:0.0",
			PaneTarget:   "e2e:0.0",
			Snapshot:     "line-1\nline-2\n",
			SnapshotHash: "h1",
			HasCursor:    true,
			CursorX:      3,
			CursorY:      8,
			UpdatedAt:    1704067200,
		}},
	}); err != nil {
		t.Fatalf("BatchUpsertRuntime failed: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/v1/tasks/" + tid + "/pane")
	if err != nil {
		t.Fatalf("GET pane failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET pane expected 200, got %d", resp.StatusCode)
	}
	defer func() { _ = resp.Body.Close() }()

	var paneRes struct {
		Data struct {
			Snapshot struct {
				Output string `json:"output"`
				Frame  struct {
					Mode string `json:"mode"`
					Data string `json:"data"`
				} `json:"frame"`
				Cursor *struct {
					X int `json:"x"`
					Y int `json:"y"`
				} `json:"cursor"`
				UpdatedAt int64 `json:"updated_at"`
			} `json:"snapshot"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&paneRes); err != nil {
		t.Fatalf("decode pane response failed: %v", err)
	}
	if paneRes.Data.Snapshot.Output != "line-1\nline-2\n" {
		t.Fatalf("unexpected snapshot output: %#v", paneRes.Data.Snapshot)
	}
	if paneRes.Data.Snapshot.Frame.Mode != "reset" || paneRes.Data.Snapshot.Frame.Data != "line-1\nline-2\n" {
		t.Fatalf("unexpected snapshot frame: %#v", paneRes.Data.Snapshot.Frame)
	}
	if paneRes.Data.Snapshot.Cursor == nil || paneRes.Data.Snapshot.Cursor.X != 3 || paneRes.Data.Snapshot.Cursor.Y != 8 {
		t.Fatalf("unexpected snapshot cursor: %#v", paneRes.Data.Snapshot.Cursor)
	}
	if paneRes.Data.Snapshot.UpdatedAt != 1704067200 {
		t.Fatalf("unexpected snapshot updated_at: %d", paneRes.Data.Snapshot.UpdatedAt)
	}
}

func TestPaneReopenRoute_RebindsExistingTask(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: paneSvc})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	var createRes struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createRes); err != nil {
		t.Fatalf("decode create failed: %v", err)
	}

	store := projectstate.NewStore(repo)
	if err := store.SavePanes(projectstate.PanesIndex{
		createRes.Data.TaskID: {
			TaskID:     createRes.Data.TaskID,
			PaneUUID:   uuid.NewString(),
			PaneID:     "e2e:0.0",
			PaneTarget: "e2e:0.0",
		},
	}); err != nil {
		t.Fatalf("seed panes failed: %v", err)
	}

	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/panes/reopen", "application/json", nil)
	if err != nil {
		t.Fatalf("POST reopen failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST reopen expected 200, got %d", resp.StatusCode)
	}
	if paneSvc.lastSibling != "e2e:0.0" {
		t.Fatalf("expected reopen to use existing pane target, got %q", paneSvc.lastSibling)
	}
	var reopenRes struct {
		Data struct {
			TaskID   string `json:"task_id"`
			PaneUUID string `json:"pane_uuid"`
			PaneID   string `json:"pane_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reopenRes); err != nil {
		t.Fatalf("decode reopen failed: %v", err)
	}
	if reopenRes.Data.TaskID != createRes.Data.TaskID {
		t.Fatalf("expected same task id, got %q", reopenRes.Data.TaskID)
	}
	if _, err := uuid.Parse(reopenRes.Data.PaneUUID); err != nil {
		t.Fatalf("pane_uuid is not valid UUID: %v", err)
	}

	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("load panes failed: %v", err)
	}
	if panes[createRes.Data.TaskID].PaneID != "pane_sibling_1" {
		t.Fatalf("expected pane binding replaced, got %#v", panes[createRes.Data.TaskID])
	}
}

func TestTaskPaneReopen_DoesNotDependOnTaskTreeJSON(t *testing.T) {
	tid := uniqueTaskID(t, "t1")
	repo := t.TempDir()
	store := projectstate.NewStore(repo)
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:    tid,
		ProjectID: "p1",
		Title:     "root",
		Status:    projectstate.StatusPending,
	}); err != nil {
		t.Fatalf("seed task failed: %v", err)
	}
	if err := store.SavePanes(projectstate.PanesIndex{
		tid: {
			TaskID:     tid,
			PaneUUID:   uuid.NewString(),
			PaneID:     "e2e:0.0",
			PaneTarget: "e2e:0.0",
		},
	}); err != nil {
		t.Fatalf("seed panes failed: %v", err)
	}

	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	paneSvc := &fakePaneService{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: paneSvc})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/tasks/"+tid+"/panes/reopen", "application/json", nil)
	if err != nil {
		t.Fatalf("POST reopen failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST reopen expected 200, got %d", resp.StatusCode)
	}
	if paneSvc.lastSibling != "e2e:0.0" {
		t.Fatalf("expected reopen to use existing pane target, got %q", paneSvc.lastSibling)
	}

	rows, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	foundRunning := false
	for _, row := range rows {
		if row.TaskID == tid && row.Status == projectstate.StatusRunning {
			foundRunning = true
			break
		}
	}
	if !foundRunning {
		t.Fatalf("expected task t1 status running after reopen, rows=%#v", rows)
	}
}
