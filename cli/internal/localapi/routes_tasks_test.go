package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"os"
	"shellman/cli/internal/global"
	"shellman/cli/internal/projectstate"
	"time"
)

type staticConfigStore struct{}

func (s *staticConfigStore) LoadOrInit() (global.GlobalConfig, error) {
	return global.GlobalConfig{LocalPort: 4621}, nil
}
func (s *staticConfigStore) Save(cfg global.GlobalConfig) error { return nil }

type mutableConfigStore struct {
	cfg global.GlobalConfig
}

func (s *mutableConfigStore) LoadOrInit() (global.GlobalConfig, error) {
	return s.cfg, nil
}
func (s *mutableConfigStore) Save(cfg global.GlobalConfig) error {
	s.cfg = cfg
	return nil
}

type memProjectsStore struct {
	projects []global.ActiveProject
}

func (m *memProjectsStore) ListProjects() ([]global.ActiveProject, error) {
	return append([]global.ActiveProject{}, m.projects...), nil
}
func (m *memProjectsStore) AddProject(project global.ActiveProject) error {
	m.projects = append(m.projects, project)
	return nil
}
func (m *memProjectsStore) RemoveProject(projectID string) error { return nil }

type fakeTaskPromptSender struct {
	err   error
	calls []struct {
		target string
		text   string
	}
}

func (f *fakeTaskPromptSender) SendInput(target, text string) error {
	f.calls = append(f.calls, struct {
		target string
		text   string
	}{target: target, text: text})
	return f.err
}

type fakeTaskMessageRunner struct {
	reply string
	err   error
	calls []string
}

func (f *fakeTaskMessageRunner) Run(_ context.Context, userPrompt string) (string, error) {
	f.calls = append(f.calls, userPrompt)
	if f.err != nil {
		return "", f.err
	}
	return f.reply, nil
}

type blockingTaskMessageRunner struct {
	reply   string
	started chan struct{}
	release chan struct{}
	calls   []string
}

func (f *blockingTaskMessageRunner) Run(_ context.Context, userPrompt string) (string, error) {
	f.calls = append(f.calls, userPrompt)
	select {
	case <-f.started:
	default:
		close(f.started)
	}
	<-f.release
	return f.reply, nil
}

func postRunReportResult(t *testing.T, srv *Server, ts *httptest.Server, repo, taskID, summary string, headers map[string]string) (AutoCompleteByPaneResult, *AutoCompleteByPaneError) {
	t.Helper()
	store := projectstate.NewStore(repo)
	runID := "r_test_" + strings.ReplaceAll(time.Now().UTC().Format("150405.000000000"), ".", "") + "_" + taskID
	if err := store.InsertRun(projectstate.RunRecord{
		RunID:     runID,
		TaskID:    taskID,
		RunStatus: projectstate.RunStatusRunning,
	}); err != nil {
		t.Fatalf("InsertRun failed: %v", err)
	}
	paneTarget := ""
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("LoadPanes failed: %v", err)
	}
	if binding, ok := panes[taskID]; ok {
		paneTarget = strings.TrimSpace(binding.PaneTarget)
		if paneTarget == "" {
			paneTarget = strings.TrimSpace(binding.PaneID)
		}
	}
	if paneTarget == "" {
		paneTarget = "e2e:auto:" + taskID
	}
	if _, ok := panes[taskID]; !ok {
		panes[taskID] = projectstate.PaneBinding{
			TaskID:     taskID,
			PaneUUID:   "pane-uuid-auto-" + taskID,
			PaneID:     paneTarget,
			PaneTarget: paneTarget,
		}
		if err := store.SavePanes(panes); err != nil {
			t.Fatalf("SavePanes failed: %v", err)
		}
	}
	bindBody := bytes.NewBufferString(`{"pane_target":"` + paneTarget + `"}`)
	bindReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/runs/"+runID+"/bind-pane", bindBody)
	bindReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		bindReq.Header.Set(k, v)
	}
	bindResp, err := http.DefaultClient.Do(bindReq)
	if err != nil {
		t.Fatalf("POST run bind-pane failed: %v", err)
	}
	if bindResp.StatusCode != http.StatusOK {
		t.Fatalf("POST bind-pane expected 200, got %d", bindResp.StatusCode)
	}
	_ = bindResp.Body.Close()

	return srv.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget: paneTarget,
		Summary:    summary,
		RequestMeta: map[string]any{
			"caller_method":         "INTERNAL",
			"caller_path":           "internal:auto-progress",
			"caller_user_agent":     strings.TrimSpace(headers["User-Agent"]),
			"caller_turn_uuid":      strings.TrimSpace(headers["X-Shellman-Turn-UUID"]),
			"caller_gateway_source": strings.TrimSpace(headers["X-Shellman-Gateway-Source"]),
			"caller_active_pane":    paneTarget,
		},
		CallerPath:       "internal:auto-progress",
		CallerActivePane: paneTarget,
	})
}

func TestProjectTree_ReadsFromTasksTableOnly(t *testing.T) {
	tRoot := uniqueTaskID(t, "t_root")
	tChild := uniqueTaskID(t, "t_child")
	repo := t.TempDir()
	store := projectstate.NewStore(repo)
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:    tRoot,
		ProjectID: "p1",
		Title:     "root",
		Status:    projectstate.StatusPending,
	}); err != nil {
		t.Fatalf("seed root task failed: %v", err)
	}
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:       tChild,
		ProjectID:    "p1",
		ParentTaskID: tRoot,
		Title:        "child",
		Status:       projectstate.StatusRunning,
	}); err != nil {
		t.Fatalf("seed child task failed: %v", err)
	}

	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/p1/tree")
	if err != nil {
		t.Fatalf("GET tree failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET tree expected 200, got %d", resp.StatusCode)
	}
	defer func() { _ = resp.Body.Close() }()

	var treeRes struct {
		OK   bool `json:"ok"`
		Data struct {
			ProjectID string                  `json:"project_id"`
			Nodes     []projectstate.TaskNode `json:"nodes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&treeRes); err != nil {
		t.Fatalf("decode tree response failed: %v", err)
	}
	if treeRes.Data.ProjectID != "p1" {
		t.Fatalf("unexpected project id: %s", treeRes.Data.ProjectID)
	}
	if len(treeRes.Data.Nodes) != 2 {
		t.Fatalf("expected 2 tree nodes from tasks table, got %d", len(treeRes.Data.Nodes))
	}
}

func TestTaskAutopilotRoutes_GetAndPatch(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"autopilot"}`))
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

	getResp, err := http.Get(ts.URL + "/api/v1/tasks/" + created.Data.TaskID + "/autopilot")
	if err != nil {
		t.Fatalf("GET autopilot failed: %v", err)
	}
	defer func() { _ = getResp.Body.Close() }()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from GET, got %d", getResp.StatusCode)
	}
	var getBody struct {
		Data struct {
			Autopilot bool `json:"autopilot"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getResp.Body).Decode(&getBody); err != nil {
		t.Fatalf("decode get autopilot failed: %v", err)
	}
	if getBody.Data.Autopilot {
		t.Fatal("expected default autopilot=false")
	}

	patchReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+created.Data.TaskID+"/autopilot", bytes.NewBufferString(`{"autopilot":true}`))
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("PATCH autopilot failed: %v", err)
	}
	defer func() { _ = patchResp.Body.Close() }()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from PATCH, got %d", patchResp.StatusCode)
	}
	var patchBody struct {
		Data struct {
			Autopilot bool `json:"autopilot"`
		} `json:"data"`
	}
	if err := json.NewDecoder(patchResp.Body).Decode(&patchBody); err != nil {
		t.Fatalf("decode patch autopilot failed: %v", err)
	}
	if !patchBody.Data.Autopilot {
		t.Fatal("expected autopilot=true after patch")
	}

	getResp2, err := http.Get(ts.URL + "/api/v1/tasks/" + created.Data.TaskID + "/autopilot")
	if err != nil {
		t.Fatalf("GET autopilot (after patch) failed: %v", err)
	}
	defer func() { _ = getResp2.Body.Close() }()
	var getBody2 struct {
		Data struct {
			Autopilot bool `json:"autopilot"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getResp2.Body).Decode(&getBody2); err != nil {
		t.Fatalf("decode get autopilot (after patch) failed: %v", err)
	}
	if !getBody2.Data.Autopilot {
		t.Fatal("expected persisted in-memory autopilot=true")
	}
}

func TestTaskRoutes(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/p1/tree")
	if err != nil {
		t.Fatalf("GET tree failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET tree expected 200, got %d", resp.StatusCode)
	}

	createBody := bytes.NewBufferString(`{"project_id":"p1","title":"root"}`)
	resp, err = http.Post(ts.URL+"/api/v1/tasks", "application/json", createBody)
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST tasks expected 200, got %d", resp.StatusCode)
	}
	var createRes struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createRes); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	if createRes.Data.TaskID == "" {
		t.Fatal("expected task_id")
	}

	deriveBody := bytes.NewBufferString(`{"title":"child"}`)
	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/derive", "application/json", deriveBody)
	if err != nil {
		t.Fatalf("POST derive failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST derive expected 200, got %d", resp.StatusCode)
	}
	var deriveRes struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&deriveRes); err != nil {
		t.Fatalf("decode derive response failed: %v", err)
	}

	statusBody := bytes.NewBufferString(`{"status":"running"}`)
	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+deriveRes.Data.TaskID+"/status", statusBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH status failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH status expected 200, got %d", resp.StatusCode)
	}

	checkBody := bytes.NewBufferString(`{"checked":true}`)
	req, _ = http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+deriveRes.Data.TaskID+"/check", checkBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH check failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH check expected 200, got %d", resp.StatusCode)
	}

	titleBody := bytes.NewBufferString(`{"title":"child from prompt"}`)
	req, _ = http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+deriveRes.Data.TaskID+"/title", titleBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH title failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH title expected 200, got %d", resp.StatusCode)
	}

	descBody := bytes.NewBufferString(`{"description":"## plan\n- item 1"}`)
	req, _ = http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+deriveRes.Data.TaskID+"/description", descBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH description failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH description expected 200, got %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/api/v1/projects/p1/tree")
	if err != nil {
		t.Fatalf("GET tree after check failed: %v", err)
	}
	var treeRes struct {
		OK   bool `json:"ok"`
		Data struct {
			Nodes []struct {
				TaskID      string `json:"task_id"`
				Checked     bool   `json:"checked"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"nodes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&treeRes); err != nil {
		t.Fatalf("decode tree response failed: %v", err)
	}
	foundChecked := false
	for _, n := range treeRes.Data.Nodes {
		if n.TaskID == deriveRes.Data.TaskID && n.Checked {
			foundChecked = true
			break
		}
	}
	if !foundChecked {
		t.Fatalf("expected task %s checked in tree", deriveRes.Data.TaskID)
	}
	foundTitle := false
	for _, n := range treeRes.Data.Nodes {
		if n.TaskID == deriveRes.Data.TaskID && n.Title == "child from prompt" {
			foundTitle = true
			break
		}
	}
	if !foundTitle {
		t.Fatalf("expected task %s title updated in tree", deriveRes.Data.TaskID)
	}
	foundDescription := false
	for _, n := range treeRes.Data.Nodes {
		if n.TaskID == deriveRes.Data.TaskID && n.Description == "## plan\n- item 1" {
			foundDescription = true
			break
		}
	}
	if !foundDescription {
		t.Fatalf("expected task %s description updated in tree", deriveRes.Data.TaskID)
	}

	_, runErr := postRunReportResult(t, srv, ts, repo, deriveRes.Data.TaskID, "done", nil)
	if runErr != nil {
		t.Fatalf("POST report-result expected success, got %v", runErr)
	}
}

func TestTaskMessages_ListAndSend(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &fakeTaskMessageRunner{reply: "SHELLMAN_E2E_OK"}
	srv := NewServer(Deps{
		ConfigStore:     &staticConfigStore{},
		ProjectsStore:   projects,
		AgentLoopRunner: runner,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("POST tasks expected 200, got %d", createResp.StatusCode)
	}
	var createOut struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createOut); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	taskID := createOut.Data.TaskID

	sendResp, err := http.Post(ts.URL+"/api/v1/tasks/"+taskID+"/messages", "application/json", bytes.NewBufferString(`{"content":"Reply exactly: SHELLMAN_E2E_OK"}`))
	if err != nil {
		t.Fatalf("POST task messages failed: %v", err)
	}
	if sendResp.StatusCode != http.StatusOK {
		t.Fatalf("POST task messages expected 200, got %d", sendResp.StatusCode)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(runner.calls) == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("unexpected runner calls after wait: %#v", runner.calls)
	}
	if !strings.Contains(runner.calls[0], "Reply exactly: SHELLMAN_E2E_OK") {
		t.Fatalf("expected prompt keep user content, got: %q", runner.calls[0])
	}
	requiredPromptContext := []string{
		"\"task_context\"",
		"\"current_command\"",
		"\"output_tail\"",
		"\"cwd\"",
	}
	for _, item := range requiredPromptContext {
		if !strings.Contains(runner.calls[0], item) {
			t.Fatalf("expected user prompt contains %q, got: %q", item, runner.calls[0])
		}
	}

	var listOut struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID   string                           `json:"task_id"`
			Messages []projectstate.TaskMessageRecord `json:"messages"`
		} `json:"data"`
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		listResp, getErr := http.Get(ts.URL + "/api/v1/tasks/" + taskID + "/messages")
		if getErr != nil {
			t.Fatalf("GET task messages failed: %v", getErr)
		}
		if listResp.StatusCode != http.StatusOK {
			t.Fatalf("GET task messages expected 200, got %d", listResp.StatusCode)
		}
		if err := json.NewDecoder(listResp.Body).Decode(&listOut); err != nil {
			_ = listResp.Body.Close()
			t.Fatalf("decode list response failed: %v", err)
		}
		_ = listResp.Body.Close()
		if len(listOut.Data.Messages) == 2 &&
			listOut.Data.Messages[0].Role == "user" &&
			listOut.Data.Messages[1].Role == "assistant" &&
			listOut.Data.Messages[1].Status == projectstate.StatusCompleted {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !listOut.OK {
		t.Fatalf("expected ok=true")
	}
	if len(listOut.Data.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(listOut.Data.Messages))
	}
	if listOut.Data.Messages[0].Role != "user" || !strings.Contains(listOut.Data.Messages[0].Content, "SHELLMAN_E2E_OK") {
		t.Fatalf("unexpected user message: %#v", listOut.Data.Messages[0])
	}
	if listOut.Data.Messages[1].Role != "assistant" || listOut.Data.Messages[1].Content != "SHELLMAN_E2E_OK" || listOut.Data.Messages[1].Status != projectstate.StatusCompleted {
		t.Fatalf("unexpected assistant message: %#v", listOut.Data.Messages[1])
	}
}

func TestTaskMessages_SendEnqueuesActorAndPersistsTimeline(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &blockingTaskMessageRunner{
		reply:   "DONE",
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	srv := NewServer(Deps{
		ConfigStore:     &staticConfigStore{},
		ProjectsStore:   projects,
		AgentLoopRunner: runner,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	var createOut struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createOut); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	taskID := createOut.Data.TaskID

	startAt := time.Now()
	sendResp, err := http.Post(ts.URL+"/api/v1/tasks/"+taskID+"/messages", "application/json", bytes.NewBufferString(`{"content":"queued"}`))
	if err != nil {
		t.Fatalf("POST task messages failed: %v", err)
	}
	defer func() { _ = sendResp.Body.Close() }()
	if sendResp.StatusCode != http.StatusOK {
		t.Fatalf("POST task messages expected 200, got %d", sendResp.StatusCode)
	}
	if time.Since(startAt) > 300*time.Millisecond {
		t.Fatalf("expected enqueue response quickly, took %s", time.Since(startAt))
	}

	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		listResp, getErr := http.Get(ts.URL + "/api/v1/tasks/" + taskID + "/messages")
		if getErr != nil {
			t.Fatalf("GET task messages failed: %v", getErr)
		}
		var listOut struct {
			Data struct {
				Messages []projectstate.TaskMessageRecord `json:"messages"`
			} `json:"data"`
		}
		if err := json.NewDecoder(listResp.Body).Decode(&listOut); err != nil {
			_ = listResp.Body.Close()
			t.Fatalf("decode list failed: %v", err)
		}
		_ = listResp.Body.Close()
		if len(listOut.Data.Messages) >= 2 && listOut.Data.Messages[1].Status == "running" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	close(runner.release)

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		listResp, getErr := http.Get(ts.URL + "/api/v1/tasks/" + taskID + "/messages")
		if getErr != nil {
			t.Fatalf("GET task messages failed: %v", getErr)
		}
		var listOut struct {
			Data struct {
				Messages []projectstate.TaskMessageRecord `json:"messages"`
			} `json:"data"`
		}
		if err := json.NewDecoder(listResp.Body).Decode(&listOut); err != nil {
			_ = listResp.Body.Close()
			t.Fatalf("decode list failed: %v", err)
		}
		_ = listResp.Body.Close()
		if len(listOut.Data.Messages) >= 2 && listOut.Data.Messages[1].Status == projectstate.StatusCompleted {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("assistant message did not transition to completed")
}

func TestTaskAgentSetFlagViaMessagesSource(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{
		ConfigStore:   &staticConfigStore{},
		ProjectsStore: projects,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	var createOut struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createOut); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	taskID := createOut.Data.TaskID

	flagReqBody := bytes.NewBufferString(`{"source":"task_set_flag","flag":"notify","status_message":"need-check"}`)
	flagResp, err := http.Post(ts.URL+"/api/v1/tasks/"+taskID+"/messages", "application/json", flagReqBody)
	if err != nil {
		t.Fatalf("POST set-flag failed: %v", err)
	}
	defer func() { _ = flagResp.Body.Close() }()
	if flagResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", flagResp.StatusCode)
	}

	treeResp, err := http.Get(ts.URL + "/api/v1/projects/p1/tree")
	if err != nil {
		t.Fatalf("GET tree failed: %v", err)
	}
	defer func() { _ = treeResp.Body.Close() }()
	var treeOut struct {
		Data struct {
			Nodes []struct {
				TaskID   string `json:"task_id"`
				Flag     string `json:"flag"`
				FlagDesc string `json:"flag_desc"`
			} `json:"nodes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(treeResp.Body).Decode(&treeOut); err != nil {
		t.Fatalf("decode tree response failed: %v", err)
	}
	found := false
	for _, node := range treeOut.Data.Nodes {
		if node.TaskID != taskID {
			continue
		}
		found = true
		if node.Flag != "notify" {
			t.Fatalf("expected flag notify, got %q", node.Flag)
		}
		if node.FlagDesc != "need-check" {
			t.Fatalf("expected flag_desc need-check, got %q", node.FlagDesc)
		}
	}
	if !found {
		t.Fatalf("task %s not found in tree", taskID)
	}
}

func TestTaskAgentWriteStdinViaMessagesSource_SendsRawInput(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	sender := &fakeTaskPromptSender{}
	srv := NewServer(Deps{
		ConfigStore:      &staticConfigStore{},
		ProjectsStore:    projects,
		TaskPromptSender: sender,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	var createOut struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createOut); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	taskID := createOut.Data.TaskID

	store := projectstate.NewStore(filepath.Clean(repo))
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("LoadPanes failed: %v", err)
	}
	panes[taskID] = projectstate.PaneBinding{
		TaskID:     taskID,
		PaneUUID:   "pane_uuid_t1",
		PaneID:     "e2e:1.0",
		PaneTarget: "e2e:1.0",
	}
	if err := store.SavePanes(panes); err != nil {
		t.Fatalf("SavePanes failed: %v", err)
	}

	resp, err := http.Post(ts.URL+"/api/v1/tasks/"+taskID+"/messages", "application/json", bytes.NewBufferString(`{"source":"tty_write_stdin","input":"echo hi"}`))
	if err != nil {
		t.Fatalf("POST messages tty_write_stdin failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if len(sender.calls) != 1 {
		t.Fatalf("expected sender called once, got %d", len(sender.calls))
	}
	if sender.calls[0].target != "e2e:1.0" {
		t.Fatalf("expected target e2e:1.0, got %q", sender.calls[0].target)
	}
	if sender.calls[0].text != "echo hi" {
		t.Fatalf("expected raw input echo hi, got %q", sender.calls[0].text)
	}
}

func TestProjectArchiveDone_HidesArchivedTasksFromTree(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("POST create root failed: %v", err)
	}
	var createOut struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createOut); err != nil {
		t.Fatalf("decode create root failed: %v", err)
	}
	_ = createResp.Body.Close()

	deriveResp, err := http.Post(ts.URL+"/api/v1/tasks/"+createOut.Data.TaskID+"/derive", "application/json", bytes.NewBufferString(`{"title":"child"}`))
	if err != nil {
		t.Fatalf("POST derive failed: %v", err)
	}
	var deriveOut struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(deriveResp.Body).Decode(&deriveOut); err != nil {
		t.Fatalf("decode derive failed: %v", err)
	}
	_ = deriveResp.Body.Close()

	checkReq, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+deriveOut.Data.TaskID+"/check", bytes.NewBufferString(`{"checked":true}`))
	checkReq.Header.Set("Content-Type", "application/json")
	checkResp, err := http.DefaultClient.Do(checkReq)
	if err != nil {
		t.Fatalf("PATCH check failed: %v", err)
	}
	if checkResp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH check expected 200, got %d", checkResp.StatusCode)
	}
	_ = checkResp.Body.Close()

	archiveResp, err := http.Post(ts.URL+"/api/v1/projects/p1/archive-done", "application/json", nil)
	if err != nil {
		t.Fatalf("POST archive-done failed: %v", err)
	}
	if archiveResp.StatusCode != http.StatusOK {
		t.Fatalf("POST archive-done expected 200, got %d", archiveResp.StatusCode)
	}
	var archiveOut struct {
		OK   bool `json:"ok"`
		Data struct {
			ArchivedCount int64 `json:"archived_count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(archiveResp.Body).Decode(&archiveOut); err != nil {
		t.Fatalf("decode archive-done failed: %v", err)
	}
	_ = archiveResp.Body.Close()
	if archiveOut.Data.ArchivedCount != 1 {
		t.Fatalf("expected archived_count=1, got %d", archiveOut.Data.ArchivedCount)
	}

	treeResp, err := http.Get(ts.URL + "/api/v1/projects/p1/tree")
	if err != nil {
		t.Fatalf("GET tree failed: %v", err)
	}
	defer func() { _ = treeResp.Body.Close() }()
	if treeResp.StatusCode != http.StatusOK {
		t.Fatalf("GET tree expected 200, got %d", treeResp.StatusCode)
	}
	var treeOut struct {
		OK   bool `json:"ok"`
		Data struct {
			Nodes []struct {
				TaskID string `json:"task_id"`
			} `json:"nodes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(treeResp.Body).Decode(&treeOut); err != nil {
		t.Fatalf("decode tree failed: %v", err)
	}
	if len(treeOut.Data.Nodes) != 1 {
		t.Fatalf("expected 1 visible node after archive, got %d", len(treeOut.Data.Nodes))
	}
	if treeOut.Data.Nodes[0].TaskID != createOut.Data.TaskID {
		t.Fatalf("expected only root visible, got %s", treeOut.Data.Nodes[0].TaskID)
	}
}

func TestTaskMutations_UpdateTaskRowInSQL(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST tasks expected 200, got %d", resp.StatusCode)
	}
	var createRes struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createRes); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	taskID := strings.TrimSpace(createRes.Data.TaskID)
	if taskID == "" {
		t.Fatal("expected task_id")
	}

	store := projectstate.NewStore(repo)
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:       taskID,
		ProjectID:    "p1",
		LastModified: 1,
	}); err != nil {
		t.Fatalf("seed last_modified failed: %v", err)
	}

	patches := []struct {
		path string
		body string
	}{
		{path: "/status", body: `{"status":"running"}`},
		{path: "/check", body: `{"checked":true}`},
		{path: "/title", body: `{"title":"root-updated"}`},
		{path: "/description", body: `{"description":"desc-updated"}`},
	}
	for _, it := range patches {
		req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+taskID+it.path, bytes.NewBufferString(it.body))
		req.Header.Set("Content-Type", "application/json")
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PATCH %s failed: %v", it.path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PATCH %s expected 200, got %d", it.path, resp.StatusCode)
		}
	}

	rowsAfter, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject after patches failed: %v", err)
	}
	byID := map[string]projectstate.TaskRecordRow{}
	for _, row := range rowsAfter {
		byID[row.TaskID] = row
	}
	row, ok := byID[taskID]
	if !ok {
		t.Fatalf("expected task %s in tasks table", taskID)
	}
	if row.Status != projectstate.StatusRunning || !row.Checked || row.Title != "root-updated" || row.Description != "desc-updated" {
		t.Fatalf("unexpected task row after patches: %#v", row)
	}
	if row.LastModified <= 1 {
		t.Fatalf("expected last_modified advanced after mutations, got %d", row.LastModified)
	}
}

func TestTaskMutations_DoNotDependOnTaskTreeJSON(t *testing.T) {
	tRoot := uniqueTaskID(t, "t_root")
	tChild := uniqueTaskID(t, "t_child")
	repo := t.TempDir()
	store := projectstate.NewStore(repo)
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:    tRoot,
		ProjectID: "p1",
		Title:     "root",
		Status:    projectstate.StatusPending,
	}); err != nil {
		t.Fatalf("seed root task failed: %v", err)
	}
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:       tChild,
		ProjectID:    "p1",
		ParentTaskID: tRoot,
		Title:        "child",
		Status:       projectstate.StatusPending,
	}); err != nil {
		t.Fatalf("seed child task failed: %v", err)
	}

	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","parent_task_id":"`+tRoot+`","title":"new-child"}`))
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST tasks expected 200, got %d", resp.StatusCode)
	}
	var createRes struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createRes); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	if strings.TrimSpace(createRes.Data.TaskID) == "" {
		t.Fatal("expected created task id")
	}

	patches := []struct {
		path string
		body string
	}{
		{path: "/status", body: `{"status":"running"}`},
		{path: "/check", body: `{"checked":true}`},
		{path: "/title", body: `{"title":"child-updated"}`},
		{path: "/description", body: `{"description":"child-desc"}`},
	}
	for _, it := range patches {
		req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+tChild+it.path, bytes.NewBufferString(it.body))
		req.Header.Set("Content-Type", "application/json")
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("PATCH %s failed: %v", it.path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("PATCH %s expected 200, got %d", it.path, resp.StatusCode)
		}
	}

	if err := srv.setTaskFlagInternal(store, "p1", tChild, "notify", "need-check"); err != nil {
		t.Fatalf("setTaskFlagInternal failed: %v", err)
	}

	rows, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject failed: %v", err)
	}
	byID := map[string]projectstate.TaskRecordRow{}
	for _, row := range rows {
		byID[row.TaskID] = row
	}
	if got := byID[tChild]; got.Status != projectstate.StatusRunning || !got.Checked || got.Title != "child-updated" || got.Description != "child-desc" || got.Flag != "notify" || got.FlagDesc != "need-check" {
		t.Fatalf("unexpected child task row: %#v", got)
	}
	if _, ok := byID[createRes.Data.TaskID]; !ok {
		t.Fatalf("expected created task %s in tasks table", createRes.Data.TaskID)
	}
}

func TestSetTaskFlagInternal_UpdatesTaskMetaInSQL(t *testing.T) {
	tid := uniqueTaskID(t, "t1")
	repo := t.TempDir()
	store := projectstate.NewStore(repo)
	if err := store.InsertTask(projectstate.TaskRecord{
		TaskID:    tid,
		ProjectID: "p1",
		Title:     "root",
		Status:    projectstate.StatusRunning,
	}); err != nil {
		t.Fatalf("InsertTask failed: %v", err)
	}
	flagReaded := true
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:       tid,
		ProjectID:    "p1",
		FlagReaded:   &flagReaded,
		LastModified: 9,
	}); err != nil {
		t.Fatalf("seed last_modified failed: %v", err)
	}

	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: &memProjectsStore{}})
	if err := srv.setTaskFlagInternal(store, "p1", tid, "notify", "need-check"); err != nil {
		t.Fatalf("setTaskFlagInternal failed: %v", err)
	}

	rowsAfter, err := store.ListTasksByProject("p1")
	if err != nil {
		t.Fatalf("ListTasksByProject after flag failed: %v", err)
	}
	if len(rowsAfter) != 1 {
		t.Fatalf("expected one task row, got %d", len(rowsAfter))
	}
	row := rowsAfter[0]
	if row.LastModified <= 9 {
		t.Fatalf("expected last_modified advanced after flag update, got %d", row.LastModified)
	}
	if row.Flag != "notify" || row.FlagDesc != "need-check" {
		t.Fatalf("expected flag fields updated, got %#v", row)
	}
	if row.FlagReaded {
		t.Fatalf("expected flag_readed reset to false after set flag, got %#v", row)
	}
}

func TestTaskMarkFlagReaded_SetsFlagReadedTrue(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"root"}`))
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST tasks expected 200, got %d", resp.StatusCode)
	}
	var createRes struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createRes); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	if strings.TrimSpace(createRes.Data.TaskID) == "" {
		t.Fatal("expected created task id")
	}

	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/flag-readed", bytes.NewBufferString(`{"flag_readed":true}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH flag-readed failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH flag-readed expected 200, got %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/api/v1/projects/p1/tree")
	if err != nil {
		t.Fatalf("GET tree failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET tree expected 200, got %d", resp.StatusCode)
	}
	var treeRes struct {
		Data struct {
			Nodes []struct {
				TaskID     string `json:"task_id"`
				FlagReaded bool   `json:"flag_readed"`
			} `json:"nodes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&treeRes); err != nil {
		t.Fatalf("decode tree response failed: %v", err)
	}
	for _, node := range treeRes.Data.Nodes {
		if node.TaskID == createRes.Data.TaskID {
			if !node.FlagReaded {
				t.Fatalf("expected flag_readed=true, got %#v", node)
			}
			return
		}
	}
	t.Fatalf("expected task %s in tree", createRes.Data.TaskID)
}

func TestTaskAdoptPaneRoute(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createBody := bytes.NewBufferString(`{"project_id":"p1","title":"parent"}`)
	resp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", createBody)
	if err != nil {
		t.Fatalf("POST tasks failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST tasks expected 200, got %d", resp.StatusCode)
	}
	var createRes struct {
		Data struct {
			TaskID string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createRes); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	if createRes.Data.TaskID == "" {
		t.Fatal("expected parent task id")
	}

	adoptBody := bytes.NewBufferString(`{"pane_target":"e2e:0.9","title":"adopted"}`)
	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/adopt-pane", "application/json", adoptBody)
	if err != nil {
		t.Fatalf("POST adopt-pane failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST adopt-pane expected 200, got %d", resp.StatusCode)
	}
	var adoptRes struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID     string `json:"task_id"`
			PaneTarget string `json:"pane_target"`
			Relation   string `json:"relation"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&adoptRes); err != nil {
		t.Fatalf("decode adopt response failed: %v", err)
	}
	if !adoptRes.OK || adoptRes.Data.TaskID == "" {
		t.Fatalf("expected adopt response task_id, got %#v", adoptRes)
	}
	if adoptRes.Data.PaneTarget != "e2e:0.9" {
		t.Fatalf("expected pane_target e2e:0.9, got %q", adoptRes.Data.PaneTarget)
	}
	if adoptRes.Data.Relation != "child" {
		t.Fatalf("expected relation child, got %q", adoptRes.Data.Relation)
	}

	store := projectstate.NewStore(repo)
	panes, err := store.LoadPanes()
	if err != nil {
		t.Fatalf("load panes failed: %v", err)
	}
	if panes[adoptRes.Data.TaskID].PaneTarget != "e2e:0.9" {
		t.Fatalf("expected adopted pane binding target e2e:0.9, got %q", panes[adoptRes.Data.TaskID].PaneTarget)
	}

	adoptAgainBody := bytes.NewBufferString(`{"pane_target":"e2e:0.9","title":"dupe"}`)
	resp, err = http.Post(ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/adopt-pane", "application/json", adoptAgainBody)
	if err != nil {
		t.Fatalf("POST adopt-pane duplicate failed: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("POST adopt-pane duplicate expected 409, got %d", resp.StatusCode)
	}
	var conflictRes struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&conflictRes); err != nil {
		t.Fatalf("decode conflict response failed: %v", err)
	}
	if conflictRes.OK || conflictRes.Error.Code != "PANE_ALREADY_BOUND" {
		t.Fatalf("expected PANE_ALREADY_BOUND, got %#v", conflictRes)
	}

	resp, err = http.Post(ts.URL+"/api/v1/tasks/missing/adopt-pane", "application/json", bytes.NewBufferString(`{"pane_target":"e2e:1.1"}`))
	if err != nil {
		t.Fatalf("POST adopt-pane missing task failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST adopt-pane missing task expected 404, got %d", resp.StatusCode)
	}
	var missingRes struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&missingRes); err != nil {
		t.Fatalf("decode missing task response failed: %v", err)
	}
	if missingRes.OK || missingRes.Error.Code != "TASK_NOT_FOUND" {
		t.Fatalf("expected TASK_NOT_FOUND, got %#v", missingRes)
	}
}

func TestTaskCompletionActions_AutoProgressCreatesTimeline(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &fakeTaskMessageRunner{reply: "auto-progress ack"}
	srv := NewServer(Deps{
		ConfigStore:     &staticConfigStore{},
		ProjectsStore:   projects,
		AgentLoopRunner: runner,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"task auto progress"}`))
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
	taskID := createOut.Data.TaskID

	_, runErr := postRunReportResult(t, srv, ts, repo, taskID, "done", nil)
	if runErr != nil {
		t.Fatalf("POST report-result expected success, got %v", runErr)
	}

	deadline := time.Now().Add(3 * time.Second)
	userSeen := false
	assistantSeen := false
	for time.Now().Before(deadline) {
		msgResp, err := http.Get(ts.URL + "/api/v1/tasks/" + taskID + "/messages")
		if err != nil {
			t.Fatalf("GET messages failed: %v", err)
		}
		var msgOut struct {
			Data struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
					Status  string `json:"status"`
				} `json:"messages"`
			} `json:"data"`
		}
		if err := json.NewDecoder(msgResp.Body).Decode(&msgOut); err != nil {
			t.Fatalf("decode messages failed: %v", err)
		}
		_ = msgResp.Body.Close()
		for _, msg := range msgOut.Data.Messages {
			if msg.Role == "user" {
				var parsed struct {
					Text string `json:"text"`
					Meta struct {
						DisplayType string `json:"display_type"`
						Source      string `json:"source"`
						Event       string `json:"event"`
					} `json:"meta"`
				}
				if err := json.Unmarshal([]byte(msg.Content), &parsed); err == nil &&
					strings.Contains(parsed.Text, "done") &&
					parsed.Meta.DisplayType == "runtime" &&
					parsed.Meta.Source == "tty_output" &&
					parsed.Meta.Event == "tty_output" {
					userSeen = true
				}
			}
			if msg.Role == "assistant" && msg.Status == "completed" && strings.Contains(msg.Content, "auto-progress ack") {
				assistantSeen = true
			}
		}
		if userSeen && assistantSeen {
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	if !userSeen || !assistantSeen {
		t.Fatalf("expected tty_output user+assistant timeline entries, userSeen=%v assistantSeen=%v", userSeen, assistantSeen)
	}
	if len(runner.calls) == 0 {
		t.Fatal("expected task agent loop runner call")
	}
	if !strings.Contains(runner.calls[0], "TTY_OUTPUT_EVENT") || !strings.Contains(runner.calls[0], "summary: done") {
		t.Fatalf("unexpected tty_output prompt: %q", runner.calls[0])
	}
	requiredAutoProgressContext := []string{
		"\"task_context\"",
		"\"current_command\"",
		"\"output_tail\"",
		"\"cwd\"",
		"\"parent_task\"",
		"\"child_tasks\"",
	}
	for _, item := range requiredAutoProgressContext {
		if !strings.Contains(runner.calls[0], item) {
			t.Fatalf("expected tty_output prompt contains %q, got: %q", item, runner.calls[0])
		}
	}
}

func TestTaskCompletionActions_AutoProgressDoesNotRunNotifyCommand(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	tmpFile := filepath.Join(t.TempDir(), "completion_trigger_should_not_run.txt")
	cfgStore := &mutableConfigStore{
		cfg: global.GlobalConfig{
			LocalPort: 4621,
			TaskCompletion: global.TaskCompletionConfig{
				NotifyEnabled:      true,
				NotifyCommand:      "echo completed > " + tmpFile,
				NotifyIdleDuration: 0,
			},
		},
	}
	runner := &fakeTaskMessageRunner{reply: "ok"}
	srv := NewServer(Deps{
		ConfigStore:     cfgStore,
		ProjectsStore:   projects,
		AgentLoopRunner: runner,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/tasks", "application/json", bytes.NewBufferString(`{"project_id":"p1","title":"task command guard"}`))
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
	taskID := createOut.Data.TaskID

	_, runErr := postRunReportResult(t, srv, ts, repo, taskID, "done", nil)
	if runErr != nil {
		t.Fatalf("POST report-result expected success, got %v", runErr)
	}

	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(tmpFile); err == nil {
		t.Fatal("expected notify command not executed for pane-idle tty_output")
	}
	if len(runner.calls) == 0 {
		t.Fatal("expected agent loop runner called for tty_output")
	}
}

func TestTaskReportResult_LegacyEndpointRejected(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/tasks/t1/report-result", "application/json", bytes.NewBufferString(`{"summary":"done"}`))
	if err != nil {
		t.Fatalf("POST report-result failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestTaskCompletionLogs_RecordTriggerSkippedWhenAgentLoopUnavailable(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{
		ConfigStore:   &staticConfigStore{},
		ProjectsStore: projects,
	})
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
	taskID := createOut.Data.TaskID

	_, runErr := postRunReportResult(t, srv, ts, repo, taskID, "done", nil)
	if runErr != nil {
		t.Fatalf("POST report-result expected success, got %v", runErr)
	}

	logPath := path.Join(os.Getenv("SHELLMAN_CONFIG_DIR"), "logs", "task-completion-automation.log")
	deadline := time.Now().Add(2 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(logPath)
		if err == nil {
			text := string(b)
			if strings.Contains(text, "\"stage\":\"trigger.received\"") &&
				strings.Contains(text, "\"stage\":\"trigger.skipped\"") &&
				strings.Contains(text, "\"task_id\":\""+taskID+"\"") &&
				strings.Contains(text, "\"reason\":\"agent-loop-enqueue-failed\"") {
				found = true
				break
			}
		}
		time.Sleep(80 * time.Millisecond)
	}
	if !found {
		t.Fatalf("expected trigger skipped logs at %s", logPath)
	}
}

func TestTaskCompletionLogs_IncludeRequestMetaOnAutoProgress(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{
		ConfigStore:   &staticConfigStore{},
		ProjectsStore: projects,
	})
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
	taskID := createOut.Data.TaskID

	_, runErr := postRunReportResult(t, srv, ts, repo, taskID, "done", map[string]string{
		"User-Agent":                "shellman-e2e-diagnostic/1.0",
		"X-Shellman-Turn-UUID":      "turn-test-123",
		"X-Shellman-Gateway-Source": "unit-test-gateway",
	})
	if runErr != nil {
		t.Fatalf("POST report-result expected success, got %v", runErr)
	}

	logPath := path.Join(os.Getenv("SHELLMAN_CONFIG_DIR"), "logs", "task-completion-automation.log")
	deadline := time.Now().Add(3 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(logPath)
		if err == nil {
			text := string(b)
			if strings.Contains(text, "\"stage\":\"trigger.skipped\"") &&
				strings.Contains(text, "\"task_id\":\""+taskID+"\"") &&
				strings.Contains(text, "\"caller_method\":\"INTERNAL\"") &&
				strings.Contains(text, "\"caller_path\":\"internal:auto-progress\"") &&
				strings.Contains(text, "\"caller_user_agent\":\"shellman-e2e-diagnostic/1.0\"") &&
				strings.Contains(text, "\"caller_turn_uuid\":\"turn-test-123\"") &&
				strings.Contains(text, "\"caller_gateway_source\":\"unit-test-gateway\"") {
				found = true
				break
			}
		}
		time.Sleep(80 * time.Millisecond)
	}
	if !found {
		t.Fatalf("expected trigger skip log with request meta at %s", logPath)
	}
}
