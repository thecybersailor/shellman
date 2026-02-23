package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"shellman/cli/internal/global"
	"shellman/cli/internal/helperconfig"
	"shellman/cli/internal/projectstate"
)

type fakeConfigStore struct {
	cfg global.GlobalConfig
}

func (f *fakeConfigStore) LoadOrInit() (global.GlobalConfig, error) { return f.cfg, nil }
func (f *fakeConfigStore) Save(cfg global.GlobalConfig) error {
	f.cfg = cfg
	return nil
}

type fakeProjectsStore struct {
	projects []global.ActiveProject
}

func (f *fakeProjectsStore) ListProjects() ([]global.ActiveProject, error) {
	return append([]global.ActiveProject{}, f.projects...), nil
}
func (f *fakeProjectsStore) AddProject(p global.ActiveProject) error {
	f.projects = append(f.projects, p)
	return nil
}
func (f *fakeProjectsStore) RemoveProject(projectID string) error {
	out := make([]global.ActiveProject, 0, len(f.projects))
	for _, p := range f.projects {
		if p.ProjectID != projectID {
			out = append(out, p)
		}
	}
	f.projects = out
	return nil
}

type fakeAppProgramsStore struct {
	cfg global.AppProgramsConfig
}

func (f *fakeAppProgramsStore) LoadOrInit() (global.AppProgramsConfig, error) { return f.cfg, nil }
func (f *fakeAppProgramsStore) Save(cfg global.AppProgramsConfig) error {
	f.cfg = cfg
	return nil
}

type fakeHelperConfigStore struct {
	cfg helperconfig.OpenAIConfig
}

func (f *fakeHelperConfigStore) LoadOpenAI() (helperconfig.OpenAIConfig, error) { return f.cfg, nil }
func (f *fakeHelperConfigStore) SaveOpenAI(cfg helperconfig.OpenAIConfig) error {
	f.cfg = cfg
	return nil
}

type fakeAgentLoopRunner struct{}

func (fakeAgentLoopRunner) Run(_ context.Context, userPrompt string) (string, error) {
	return "echo:" + userPrompt, nil
}

// uniqueTaskID returns a task id unique to this test so shared global DB does not conflict.
func uniqueTaskID(t *testing.T, base string) string {
	t.Helper()
	s := strings.ReplaceAll(t.Name(), "/", "_")
	return base + "_" + s
}

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "shellman_localapi_test_")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	if err := os.Setenv("SHELLMAN_CONFIG_DIR", dir); err != nil {
		panic(err)
	}
	if err := projectstate.InitGlobalDB(filepath.Join(dir, "shellman.db")); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestServer_ConfigAndProjectsEndpoints(t *testing.T) {
	cfgStore := &fakeConfigStore{cfg: global.GlobalConfig{
		LocalPort: 4621,
		Defaults: global.GlobalDefaults{
			SessionProgram: "shell",
			HelperProgram:  "codex",
		},
	}}
	appProgramsStore := &fakeAppProgramsStore{cfg: global.AppProgramsConfig{
		Version: 1,
		Providers: []global.AppProgramProvider{
			{ID: "codex", DisplayName: "codex", Command: "codex"},
			{ID: "claude", DisplayName: "Claude", Command: "claude"},
			{ID: "cursor", DisplayName: "Cursor", Command: "cursor"},
		},
	}}
	projStore := &fakeProjectsStore{}
	srv := NewServer(Deps{ConfigStore: cfgStore, AppProgramsStore: appProgramsStore, ProjectsStore: projStore})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/config")
	if err != nil {
		t.Fatalf("GET config failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	patchBody := bytes.NewBufferString(`{"local_port":4820}`)
	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/config", patchBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH config failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if cfgStore.cfg.LocalPort != 4820 {
		t.Fatalf("expected config updated to 4820, got %d", cfgStore.cfg.LocalPort)
	}
	patchBody = bytes.NewBufferString(`{"defaults":{"session_program":"codex","helper_program":"claude"}}`)
	req, _ = http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/config", patchBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH config failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if cfgStore.cfg.LocalPort != 4820 {
		t.Fatalf("expected local port preserved, got %d", cfgStore.cfg.LocalPort)
	}
	if cfgStore.cfg.Defaults.SessionProgram != "codex" {
		t.Fatalf("expected defaults.session_program=codex, got %q", cfgStore.cfg.Defaults.SessionProgram)
	}
	if cfgStore.cfg.Defaults.HelperProgram != "claude" {
		t.Fatalf("expected defaults.helper_program=claude, got %q", cfgStore.cfg.Defaults.HelperProgram)
	}
	patchBody = bytes.NewBufferString(`{"defaults":{"sidecar_mode":"autopilot"}}`)
	req, _ = http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/config", patchBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH config failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if cfgStore.cfg.Defaults.SidecarMode != "autopilot" {
		t.Fatalf("expected defaults.sidecar_mode=autopilot, got %q", cfgStore.cfg.Defaults.SidecarMode)
	}

	resp, err = http.Get(ts.URL + "/api/v1/projects/active")
	if err != nil {
		t.Fatalf("GET projects failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	repo := initGitRepo(t)
	createReqBody, _ := json.Marshal(map[string]string{"project_id": "p1", "repo_root": repo})
	resp, err = http.Post(ts.URL+"/api/v1/projects/active", "application/json", bytes.NewReader(createReqBody))
	if err != nil {
		t.Fatalf("POST projects failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/projects/active/p1", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE projects failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServer_Config_GetAndPatch_HelperOpenAI(t *testing.T) {
	cfgStore := &fakeConfigStore{cfg: global.GlobalConfig{
		LocalPort: 4621,
		Defaults: global.GlobalDefaults{
			SessionProgram: "shell",
			HelperProgram:  "codex",
		},
	}}
	helperStore := &fakeHelperConfigStore{}
	srv := NewServer(Deps{
		ConfigStore:       cfgStore,
		AppProgramsStore:  &fakeAppProgramsStore{},
		HelperConfigStore: helperStore,
		ProjectsStore:     &fakeProjectsStore{},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	patchBody := bytes.NewBufferString(`{
		"helper_openai": {
			"endpoint": "https://api.openai.com",
			"model": "gpt-5",
			"api_key": "sk-test-123"
		}
	}`)
	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/config", patchBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH config failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/api/v1/config")
	if err != nil {
		t.Fatalf("GET config failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			HelperOpenAI map[string]any `json:"helper_openai"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode config failed: %v", err)
	}
	if !body.OK {
		t.Fatalf("expected ok=true")
	}
	if body.Data.HelperOpenAI["endpoint"] != "https://api.openai.com" {
		t.Fatalf("unexpected endpoint: %#v", body.Data.HelperOpenAI["endpoint"])
	}
	if body.Data.HelperOpenAI["model"] != "gpt-5" {
		t.Fatalf("unexpected model: %#v", body.Data.HelperOpenAI["model"])
	}
	if body.Data.HelperOpenAI["api_key_set"] != true {
		t.Fatalf("expected api_key_set=true, got %#v", body.Data.HelperOpenAI["api_key_set"])
	}
	if _, exists := body.Data.HelperOpenAI["api_key"]; exists {
		t.Fatalf("api_key should not be exposed in GET response")
	}
}

func TestServer_Config_Get_AgentOpenAI(t *testing.T) {
	cfgStore := &fakeConfigStore{cfg: global.GlobalConfig{
		LocalPort: 4621,
		Defaults: global.GlobalDefaults{
			SessionProgram: "shell",
			HelperProgram:  "codex",
		},
	}}
	srv := NewServer(Deps{
		ConfigStore:         cfgStore,
		AppProgramsStore:    &fakeAppProgramsStore{},
		ProjectsStore:       &fakeProjectsStore{},
		AgentLoopRunner:     fakeAgentLoopRunner{},
		AgentOpenAIEndpoint: "https://api.openai.com/v1",
		AgentOpenAIModel:    "gpt-5-mini",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/config")
	if err != nil {
		t.Fatalf("GET config failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			AgentOpenAI struct {
				Endpoint string `json:"endpoint"`
				Model    string `json:"model"`
				Enabled  bool   `json:"enabled"`
			} `json:"agent_openai"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode config failed: %v", err)
	}
	if !body.OK {
		t.Fatalf("expected ok=true")
	}
	if body.Data.AgentOpenAI.Endpoint != "https://api.openai.com/v1" {
		t.Fatalf("unexpected endpoint: %q", body.Data.AgentOpenAI.Endpoint)
	}
	if body.Data.AgentOpenAI.Model != "gpt-5-mini" {
		t.Fatalf("unexpected model: %q", body.Data.AgentOpenAI.Model)
	}
	if !body.Data.AgentOpenAI.Enabled {
		t.Fatalf("expected enabled=true")
	}
}

func TestServer_AddActiveProject_RejectsMissingRepoRoot(t *testing.T) {
	cfgStore := &fakeConfigStore{cfg: global.GlobalConfig{LocalPort: 4621}}
	projStore := &fakeProjectsStore{}
	srv := NewServer(Deps{ConfigStore: cfgStore, ProjectsStore: projStore})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := bytes.NewBufferString(`{"project_id":"p_missing","repo_root":"/tmp/not-exists-shellman"}`)
	resp, err := http.Post(ts.URL+"/api/v1/projects/active", "application/json", body)
	if err != nil {
		t.Fatalf("POST projects failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestServer_AddActiveProject_AcceptsRealGitRepo(t *testing.T) {
	repo := initGitRepo(t)

	cfgStore := &fakeConfigStore{cfg: global.GlobalConfig{LocalPort: 4621}}
	projStore := &fakeProjectsStore{}
	srv := NewServer(Deps{ConfigStore: cfgStore, ProjectsStore: projStore})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := bytes.NewBufferString(fmt.Sprintf(`{"project_id":"p1","repo_root":"%s"}`, repo))
	resp, err := http.Post(ts.URL+"/api/v1/projects/active", "application/json", body)
	if err != nil {
		t.Fatalf("POST projects failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestServer_AddActiveProject_AcceptsNonGitDirectory(t *testing.T) {
	repo := t.TempDir()

	cfgStore := &fakeConfigStore{cfg: global.GlobalConfig{LocalPort: 4621}}
	projStore := &fakeProjectsStore{}
	srv := NewServer(Deps{ConfigStore: cfgStore, ProjectsStore: projStore})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	body := bytes.NewBufferString(fmt.Sprintf(`{"project_id":"p_plain","repo_root":"%s"}`, repo))
	resp, err := http.Post(ts.URL+"/api/v1/projects/active", "application/json", body)
	if err != nil {
		t.Fatalf("POST projects failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var createBody struct {
		OK   bool `json:"ok"`
		Data struct {
			ProjectID string `json:"project_id"`
			IsGitRepo bool   `json:"is_git_repo"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createBody); err != nil {
		t.Fatalf("decode add project response failed: %v", err)
	}
	if !createBody.OK {
		t.Fatalf("expected ok=true")
	}
	if createBody.Data.ProjectID != "p_plain" {
		t.Fatalf("unexpected project_id: %q", createBody.Data.ProjectID)
	}
	if createBody.Data.IsGitRepo {
		t.Fatalf("expected non-git directory to return is_git_repo=false")
	}

	listResp, err := http.Get(ts.URL + "/api/v1/projects/active")
	if err != nil {
		t.Fatalf("GET projects failed: %v", err)
	}
	defer func() { _ = listResp.Body.Close() }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listBody struct {
		OK   bool `json:"ok"`
		Data []struct {
			ProjectID string `json:"project_id"`
			RepoRoot  string `json:"repo_root"`
			IsGitRepo bool   `json:"is_git_repo"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode list projects response failed: %v", err)
	}
	if !listBody.OK || len(listBody.Data) != 1 {
		t.Fatalf("unexpected list payload: ok=%v len=%d", listBody.OK, len(listBody.Data))
	}
	if listBody.Data[0].ProjectID != "p_plain" || listBody.Data[0].RepoRoot != repo {
		t.Fatalf("unexpected project payload: %#v", listBody.Data[0])
	}
	if listBody.Data[0].IsGitRepo {
		t.Fatalf("expected listed project is_git_repo=false")
	}
}

func TestServer_SystemCapabilities_DefaultFalse(t *testing.T) {
	srv := NewServer(Deps{ConfigStore: &fakeConfigStore{}, ProjectsStore: &fakeProjectsStore{}, HelperConfigStore: &fakeHelperConfigStore{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/system/capabilities")
	if err != nil {
		t.Fatalf("GET capabilities failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			DirectoryPicker bool `json:"directory_picker"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !body.OK {
		t.Fatalf("expected ok=true")
	}
	if body.Data.DirectoryPicker {
		t.Fatalf("expected directory_picker=false")
	}
}

func TestServer_AppProgramsEndpoint(t *testing.T) {
	srv := NewServer(Deps{
		ConfigStore: &fakeConfigStore{},
		AppProgramsStore: &fakeAppProgramsStore{
			cfg: global.AppProgramsConfig{
				Version: 1,
				Providers: []global.AppProgramProvider{
					{ID: "codex", DisplayName: "codex", Command: "codex"},
					{ID: "claude", DisplayName: "Claude", Command: "claude"},
					{ID: "cursor", DisplayName: "Cursor", Command: "cursor"},
				},
			},
		},
		ProjectsStore: &fakeProjectsStore{},
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/system/app-programs")
	if err != nil {
		t.Fatalf("GET app-programs failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Version   int `json:"version"`
			Providers []struct {
				ID string `json:"id"`
			} `json:"providers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !body.OK {
		t.Fatalf("expected ok=true")
	}
	if body.Data.Version != 1 {
		t.Fatalf("expected version=1, got %d", body.Data.Version)
	}
	if len(body.Data.Providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(body.Data.Providers))
	}
}

func TestServer_SelectDirectory_Unavailable(t *testing.T) {
	srv := NewServer(Deps{ConfigStore: &fakeConfigStore{}, ProjectsStore: &fakeProjectsStore{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/system/select-directory", "application/json", bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("POST select-directory failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", resp.StatusCode)
	}
}

func TestServer_SelectDirectory_Success(t *testing.T) {
	srv := NewServer(Deps{
		ConfigStore:   &fakeConfigStore{},
		ProjectsStore: &fakeProjectsStore{},
		PickDirectory: func() (string, error) { return "/tmp/repo", nil },
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/api/v1/system/select-directory", "application/json", bytes.NewBuffer(nil))
	if err != nil {
		t.Fatalf("POST select-directory failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			RepoRoot string `json:"repo_root"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !body.OK || body.Data.RepoRoot != "/tmp/repo" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestServer_UploadImage_Success(t *testing.T) {
	srv := NewServer(Deps{ConfigStore: &fakeConfigStore{}, ProjectsStore: &fakeProjectsStore{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="image.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader([]byte{1, 2, 3, 4})); err != nil {
		t.Fatalf("write multipart failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/system/uploads/image", &body)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post upload failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var out struct {
		OK   bool `json:"ok"`
		Data struct {
			Path string `json:"path"`
			Mime string `json:"mime"`
			Size int64  `json:"size"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected ok=true")
	}
	if out.Data.Path == "" {
		t.Fatalf("expected path")
	}
	if !strings.HasPrefix(out.Data.Path, os.TempDir()) {
		t.Fatalf("unexpected path=%q", out.Data.Path)
	}
	if out.Data.Mime != "image/png" {
		t.Fatalf("unexpected mime=%q", out.Data.Mime)
	}
	if out.Data.Size <= 0 {
		t.Fatalf("expected positive size, got %d", out.Data.Size)
	}
}

func TestServer_UploadImage_Rejects_MissingFile(t *testing.T) {
	srv := NewServer(Deps{ConfigStore: &fakeConfigStore{}, ProjectsStore: &fakeProjectsStore{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("note", "hello"); err != nil {
		t.Fatalf("write field failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/system/uploads/image", &body)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post upload failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var out struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if out.OK {
		t.Fatalf("expected ok=false")
	}
	if out.Error.Code != "INVALID_UPLOAD" {
		t.Fatalf("unexpected error code %q", out.Error.Code)
	}
}

func TestServer_UploadImage_Rejects_NonImage(t *testing.T) {
	srv := NewServer(Deps{ConfigStore: &fakeConfigStore{}, ProjectsStore: &fakeProjectsStore{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="note.txt"`)
	header.Set("Content-Type", "text/plain")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create part failed: %v", err)
	}
	if _, err := io.WriteString(part, "hello"); err != nil {
		t.Fatalf("write part failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/system/uploads/image", &body)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post upload failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var out struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if out.OK {
		t.Fatalf("expected ok=false")
	}
	if out.Error.Code != "INVALID_UPLOAD" {
		t.Fatalf("unexpected error code %q", out.Error.Code)
	}
}

func TestServer_UploadImage_Rejects_EmptyFile(t *testing.T) {
	srv := NewServer(Deps{ConfigStore: &fakeConfigStore{}, ProjectsStore: &fakeProjectsStore{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="empty.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(nil)); err != nil {
		t.Fatalf("write multipart failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/system/uploads/image", &body)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post upload failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var out struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if out.OK {
		t.Fatalf("expected ok=false")
	}
	if out.Error.Code != "INVALID_UPLOAD" {
		t.Fatalf("unexpected error code %q", out.Error.Code)
	}
}

func TestServer_UploadImage_Rejects_TooLarge(t *testing.T) {
	srv := NewServer(Deps{ConfigStore: &fakeConfigStore{}, ProjectsStore: &fakeProjectsStore{}})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	payload := bytes.Repeat([]byte{0x12}, maxUploadImageSize+1)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="huge.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(payload)); err != nil {
		t.Fatalf("write multipart failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/system/uploads/image", &body)
	if err != nil {
		t.Fatalf("new request failed: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post upload failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", resp.StatusCode)
	}

	var out struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if out.OK {
		t.Fatalf("expected ok=false")
	}
	if out.Error.Code != "FILE_TOO_LARGE" {
		t.Fatalf("unexpected error code %q", out.Error.Code)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	cmd := exec.Command("git", "-C", repo, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v, out=%s", err, string(out))
	}
	return repo
}
