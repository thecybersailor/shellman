package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"termteam/cli/internal/global"
	"termteam/cli/internal/helperconfig"
)

func mustRunGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v, out=%s", args, err, string(out))
	}
}

func TestAddonRoutes_DiffFilesAndContent(t *testing.T) {
	repo := t.TempDir()
	mustRunGit(t, repo, "init")
	mustRunGit(t, repo, "config", "user.email", "muxt@example.com")
	mustRunGit(t, repo, "config", "user.name", "Muxt Test")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write a.txt failed: %v", err)
	}
	mustRunGit(t, repo, "add", "a.txt")
	mustRunGit(t, repo, "commit", "-m", "init")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("modify a.txt failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("new file\n"), 0o644); err != nil {
		t.Fatalf("write b.txt failed: %v", err)
	}

	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, PaneService: &fakePaneService{}})
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
	if createRes.Data.TaskID == "" {
		t.Fatal("expected task id")
	}

	diffResp, err := http.Get(ts.URL + "/api/v1/tasks/" + createRes.Data.TaskID + "/diff")
	if err != nil {
		t.Fatalf("GET diff failed: %v", err)
	}
	if diffResp.StatusCode != http.StatusOK {
		t.Fatalf("GET diff expected 200, got %d", diffResp.StatusCode)
	}
	var diffRes struct {
		Data struct {
			Diff string `json:"diff"`
		} `json:"data"`
	}
	if err := json.NewDecoder(diffResp.Body).Decode(&diffRes); err != nil {
		t.Fatalf("decode diff failed: %v", err)
	}
	if !strings.Contains(diffRes.Data.Diff, "a.txt") {
		t.Fatalf("expected diff contains a.txt, got: %s", diffRes.Data.Diff)
	}

	filesResp, err := http.Get(ts.URL + "/api/v1/tasks/" + createRes.Data.TaskID + "/files")
	if err != nil {
		t.Fatalf("GET files failed: %v", err)
	}
	if filesResp.StatusCode != http.StatusOK {
		t.Fatalf("GET files expected 200, got %d", filesResp.StatusCode)
	}
	var filesRes struct {
		Data struct {
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.NewDecoder(filesResp.Body).Decode(&filesRes); err != nil {
		t.Fatalf("decode files failed: %v", err)
	}
	paths := map[string]bool{}
	for _, f := range filesRes.Data.Files {
		paths[f.Path] = true
	}
	if !paths["a.txt"] || !paths["b.txt"] {
		t.Fatalf("expected files include a.txt and b.txt, got %#v", filesRes.Data.Files)
	}

	contentURL := ts.URL + "/api/v1/tasks/" + createRes.Data.TaskID + "/files/content?path=" + url.QueryEscape("a.txt")
	contentResp, err := http.Get(contentURL)
	if err != nil {
		t.Fatalf("GET file content failed: %v", err)
	}
	if contentResp.StatusCode != http.StatusOK {
		t.Fatalf("GET file content expected 200, got %d", contentResp.StatusCode)
	}
	var contentRes struct {
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.NewDecoder(contentResp.Body).Decode(&contentRes); err != nil {
		t.Fatalf("decode content failed: %v", err)
	}
	if !strings.Contains(contentRes.Data.Content, "hello world") {
		t.Fatalf("unexpected file content: %s", contentRes.Data.Content)
	}

	commitBody := bytes.NewBufferString(`{"message":"chore: save local changes"}`)
	commitResp, err := http.Post(ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/commit", "application/json", commitBody)
	if err != nil {
		t.Fatalf("POST commit failed: %v", err)
	}
	if commitResp.StatusCode != http.StatusOK {
		t.Fatalf("POST commit expected 200, got %d", commitResp.StatusCode)
	}
	var commitRes struct {
		OK   bool `json:"ok"`
		Data struct {
			CommitHash string `json:"commit_hash"`
		} `json:"data"`
	}
	if err := json.NewDecoder(commitResp.Body).Decode(&commitRes); err != nil {
		t.Fatalf("decode commit failed: %v", err)
	}
	if !commitRes.OK || strings.TrimSpace(commitRes.Data.CommitHash) == "" {
		t.Fatalf("expected commit hash, got %+v", commitRes)
	}

	filesAfterResp, err := http.Get(ts.URL + "/api/v1/tasks/" + createRes.Data.TaskID + "/files")
	if err != nil {
		t.Fatalf("GET files after commit failed: %v", err)
	}
	if filesAfterResp.StatusCode != http.StatusOK {
		t.Fatalf("GET files after commit expected 200, got %d", filesAfterResp.StatusCode)
	}
	var filesAfterRes struct {
		Data struct {
			Files []struct {
				Path string `json:"path"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.NewDecoder(filesAfterResp.Body).Decode(&filesAfterRes); err != nil {
		t.Fatalf("decode files after commit failed: %v", err)
	}
	if len(filesAfterRes.Data.Files) != 0 {
		t.Fatalf("expected no files after commit, got %#v", filesAfterRes.Data.Files)
	}
}

func TestAddonRoutes_CommitMessageGenerate_UsesDefaultHelperProgram(t *testing.T) {
	repo := t.TempDir()
	mustRunGit(t, repo, "init")
	mustRunGit(t, repo, "config", "user.email", "muxt@example.com")
	mustRunGit(t, repo, "config", "user.name", "Muxt Test")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write a.txt failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("new file\n"), 0o644); err != nil {
		t.Fatalf("write b.txt failed: %v", err)
	}

	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	called := false
	mockRunner := func(_ context.Context, cmd string, args ...string) ([]byte, error) {
		called = true
		if cmd != "mock-helper" {
			return nil, errors.New("unexpected helper command")
		}
		if len(args) == 0 {
			return nil, errors.New("expected helper args")
		}
		return []byte("feat: use helper generator"), nil
	}

	cfgStore := &mutableConfigStore{cfg: global.GlobalConfig{
		LocalPort: 4621,
		Defaults: global.GlobalDefaults{
			HelperProgram: "codex",
		},
	}}
	appProgramsStore := &fakeAppProgramsStore{
		cfg: global.AppProgramsConfig{
			Version: 1,
			Providers: []global.AppProgramProvider{
				{ID: "codex", DisplayName: "codex", Command: "mock-helper"},
				{ID: "claude", DisplayName: "Claude", Command: "claude"},
			},
		},
	}

	srv := NewServer(Deps{
		ConfigStore:      cfgStore,
		AppProgramsStore: appProgramsStore,
		ProjectsStore:    projects,
		ExecuteCommand:   mockRunner,
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
		t.Fatalf("decode create response failed: %v", err)
	}
	if createRes.Data.TaskID == "" {
		t.Fatal("expected task id")
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/commit-message/generate", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST commit-message/generate failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST commit-message/generate expected 200, got %d", resp.StatusCode)
	}
	var genRes struct {
		Data struct {
			Message string `json:"message"`
			TaskID  string `json:"task_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&genRes); err != nil {
		t.Fatalf("decode commit-message response failed: %v", err)
	}
	if !called {
		t.Fatal("expected helper command invoked")
	}
	if genRes.Data.TaskID != createRes.Data.TaskID {
		t.Fatalf("expected task_id=%q, got %q", createRes.Data.TaskID, genRes.Data.TaskID)
	}
	if genRes.Data.Message != "feat: use helper generator" {
		t.Fatalf("expected helper message, got %q", genRes.Data.Message)
	}
}

func TestAddonRoutes_CommitMessageGenerate_UsesConfiguredHelperCommandArgs(t *testing.T) {
	repo := t.TempDir()
	mustRunGit(t, repo, "init")
	mustRunGit(t, repo, "config", "user.email", "muxt@example.com")
	mustRunGit(t, repo, "config", "user.name", "Muxt Test")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write a.txt failed: %v", err)
	}

	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	var calledCmd string
	var calledArgs []string
	mockRunner := func(_ context.Context, cmd string, args ...string) ([]byte, error) {
		calledCmd = cmd
		calledArgs = append([]string{}, args...)
		return []byte("feat: run configured cli"), nil
	}

	cfgStore := &mutableConfigStore{cfg: global.GlobalConfig{
		LocalPort: 4621,
		Defaults: global.GlobalDefaults{
			HelperProgram: "claude",
		},
	}}
	appProgramsStore := &fakeAppProgramsStore{
		cfg: global.AppProgramsConfig{
			Version: 1,
			Providers: []global.AppProgramProvider{
				{ID: "codex", DisplayName: "codex", Command: "mock-codex"},
				{ID: "claude", DisplayName: "Claude", Command: "mock-helper --model sonnet"},
			},
		},
	}

	srv := NewServer(Deps{
		ConfigStore:      cfgStore,
		AppProgramsStore: appProgramsStore,
		ProjectsStore:    projects,
		ExecuteCommand:   mockRunner,
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
		t.Fatalf("decode create response failed: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/commit-message/generate", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST commit-message/generate failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST commit-message/generate expected 200, got %d", resp.StatusCode)
	}
	if calledCmd != "mock-helper" {
		t.Fatalf("expected helper cmd mock-helper, got %q", calledCmd)
	}
	if len(calledArgs) < 3 {
		t.Fatalf("expected helper args include fixed args and prompt, got %#v", calledArgs)
	}
	if calledArgs[0] != "--model" || calledArgs[1] != "sonnet" {
		t.Fatalf("expected fixed helper args preserved, got %#v", calledArgs)
	}
}

func TestAddonRoutes_CommitMessageGenerate_PrefersCommitMessageCommand(t *testing.T) {
	repo := t.TempDir()
	mustRunGit(t, repo, "init")
	mustRunGit(t, repo, "config", "user.email", "muxt@example.com")
	mustRunGit(t, repo, "config", "user.name", "Muxt Test")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write a.txt failed: %v", err)
	}

	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	var calledCmd string
	var calledArgs []string
	mockRunner := func(_ context.Context, cmd string, args ...string) ([]byte, error) {
		calledCmd = cmd
		calledArgs = append([]string{}, args...)
		return []byte("feat: use commit-message command"), nil
	}

	cfgStore := &mutableConfigStore{cfg: global.GlobalConfig{
		LocalPort: 4621,
		Defaults: global.GlobalDefaults{
			HelperProgram: "claude",
		},
	}}
	appProgramsStore := &fakeAppProgramsStore{
		cfg: global.AppProgramsConfig{
			Version: 1,
			Providers: []global.AppProgramProvider{
				{
					ID:                   "claude",
					DisplayName:          "Claude",
					Command:              "mock-helper --model default",
					CommitMessageCommand: "mock-helper --model sonnet-4",
				},
			},
		},
	}

	srv := NewServer(Deps{
		ConfigStore:      cfgStore,
		AppProgramsStore: appProgramsStore,
		ProjectsStore:    projects,
		ExecuteCommand:   mockRunner,
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
		t.Fatalf("decode create response failed: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/commit-message/generate", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST commit-message/generate failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST commit-message/generate expected 200, got %d", resp.StatusCode)
	}
	if calledCmd != "mock-helper" {
		t.Fatalf("expected helper cmd mock-helper, got %q", calledCmd)
	}
	if len(calledArgs) < 3 {
		t.Fatalf("expected helper args include fixed args and prompt, got %#v", calledArgs)
	}
	if calledArgs[0] != "--model" || calledArgs[1] != "sonnet-4" {
		t.Fatalf("expected commit-message command args, got %#v", calledArgs)
	}
}

func TestAddonRoutes_CommitMessageGenerate_ReturnsErrorWhenHelperFails(t *testing.T) {
	repo := t.TempDir()
	mustRunGit(t, repo, "init")
	mustRunGit(t, repo, "config", "user.email", "muxt@example.com")
	mustRunGit(t, repo, "config", "user.name", "Muxt Test")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write a.txt failed: %v", err)
	}
	mustRunGit(t, repo, "add", "a.txt")
	mustRunGit(t, repo, "commit", "-m", "init")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("modify a.txt failed: %v", err)
	}

	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	mockRunner := func(_ context.Context, cmd string, args ...string) ([]byte, error) {
		return []byte("model not found"), errors.New("exit status 1")
	}

	cfgStore := &mutableConfigStore{cfg: global.GlobalConfig{
		LocalPort: 4621,
		Defaults: global.GlobalDefaults{
			HelperProgram: "claude",
		},
	}}
	appProgramsStore := &fakeAppProgramsStore{
		cfg: global.AppProgramsConfig{
			Version: 1,
			Providers: []global.AppProgramProvider{
				{ID: "codex", DisplayName: "codex", Command: "mock-helper"},
				{ID: "claude", DisplayName: "Claude", Command: "mock-helper"},
			},
		},
	}

	srv := NewServer(Deps{
		ConfigStore:      cfgStore,
		AppProgramsStore: appProgramsStore,
		ProjectsStore:    projects,
		ExecuteCommand:   mockRunner,
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
		t.Fatalf("decode create response failed: %v", err)
	}
	if createRes.Data.TaskID == "" {
		t.Fatal("expected task id")
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/commit-message/generate", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST commit-message/generate failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("POST commit-message/generate expected 500, got %d", resp.StatusCode)
	}
	var genRes struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&genRes); err != nil {
		t.Fatalf("decode commit-message response failed: %v", err)
	}
	if genRes.OK {
		t.Fatal("expected request failure when helper fails")
	}
	if genRes.Error.Code != "COMMIT_MESSAGE_GENERATE_FAILED" {
		t.Fatalf("expected COMMIT_MESSAGE_GENERATE_FAILED, got %q", genRes.Error.Code)
	}
	if !strings.Contains(genRes.Error.Message, "model not found") {
		t.Fatalf("expected helper stderr in error message, got %q", genRes.Error.Message)
	}
}

func TestAddonRoutes_CommitMessageGenerate_UsesOpenAIConfig(t *testing.T) {
	repo := t.TempDir()
	mustRunGit(t, repo, "init")
	mustRunGit(t, repo, "config", "user.email", "muxt@example.com")
	mustRunGit(t, repo, "config", "user.name", "Muxt Test")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write a.txt failed: %v", err)
	}

	openAICalled := false
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openAICalled = true
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("expected /chat/completions, got %s", r.URL.Path)
		}
		if got := strings.TrimSpace(r.Header.Get("Authorization")); got != "Bearer sk-openai-123" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		var body struct {
			Model       string  `json:"model"`
			Temperature float64 `json:"temperature"`
			Messages    []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body failed: %v", err)
		}
		if body.Model != "gpt-5" {
			t.Fatalf("expected model gpt-5, got %q", body.Model)
		}
		if len(body.Messages) == 0 || strings.TrimSpace(body.Messages[0].Content) == "" {
			t.Fatalf("expected non-empty prompt in messages")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{
						"content": "feat: from openai api",
					},
				},
			},
		})
	}))
	defer openAIServer.Close()

	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	cliCalled := false
	mockRunner := func(_ context.Context, cmd string, args ...string) ([]byte, error) {
		cliCalled = true
		return []byte("feat: from cli helper"), nil
	}

	cfgStore := &mutableConfigStore{cfg: global.GlobalConfig{
		LocalPort: 4621,
		Defaults: global.GlobalDefaults{
			HelperProgram: "codex",
		},
	}}
	appProgramsStore := &fakeAppProgramsStore{
		cfg: global.AppProgramsConfig{
			Version: 1,
			Providers: []global.AppProgramProvider{
				{ID: "codex", DisplayName: "codex", Command: "mock-helper"},
			},
		},
	}
	helperStore := &fakeHelperConfigStore{
		cfg: helperconfig.OpenAIConfig{
			Endpoint:  openAIServer.URL,
			Model:     "gpt-5",
			APIKey:    "sk-openai-123",
			APIKeySet: true,
		},
	}

	srv := NewServer(Deps{
		ConfigStore:       cfgStore,
		AppProgramsStore:  appProgramsStore,
		HelperConfigStore: helperStore,
		ProjectsStore:     projects,
		ExecuteCommand:    mockRunner,
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
		t.Fatalf("decode create response failed: %v", err)
	}
	if createRes.Data.TaskID == "" {
		t.Fatal("expected task id")
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/commit-message/generate", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST commit-message/generate failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST commit-message/generate expected 200, got %d", resp.StatusCode)
	}
	var genRes struct {
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&genRes); err != nil {
		t.Fatalf("decode commit-message response failed: %v", err)
	}
	if genRes.Data.Message != "feat: from openai api" {
		t.Fatalf("expected openai message, got %q", genRes.Data.Message)
	}
	if cliCalled {
		t.Fatal("expected cli helper runner NOT called when openai config is complete")
	}
	if !openAICalled {
		t.Fatal("expected openai endpoint called")
	}
}

func TestAddonRoutes_CommitMessageGenerate_FallsBackToCLIWhenOpenAIConfigMissing(t *testing.T) {
	repo := t.TempDir()
	mustRunGit(t, repo, "init")
	mustRunGit(t, repo, "config", "user.email", "muxt@example.com")
	mustRunGit(t, repo, "config", "user.name", "Muxt Test")

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write a.txt failed: %v", err)
	}

	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	cliCalled := false
	mockRunner := func(_ context.Context, cmd string, args ...string) ([]byte, error) {
		cliCalled = true
		return []byte("feat: fallback to cli helper"), nil
	}

	cfgStore := &mutableConfigStore{cfg: global.GlobalConfig{
		LocalPort: 4621,
		Defaults: global.GlobalDefaults{
			HelperProgram: "codex",
		},
	}}
	appProgramsStore := &fakeAppProgramsStore{
		cfg: global.AppProgramsConfig{
			Version: 1,
			Providers: []global.AppProgramProvider{
				{ID: "codex", DisplayName: "codex", Command: "mock-helper"},
			},
		},
	}
	helperStore := &fakeHelperConfigStore{
		cfg: helperconfig.OpenAIConfig{},
	}

	srv := NewServer(Deps{
		ConfigStore:       cfgStore,
		AppProgramsStore:  appProgramsStore,
		HelperConfigStore: helperStore,
		ProjectsStore:     projects,
		ExecuteCommand:    mockRunner,
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
		t.Fatalf("decode create response failed: %v", err)
	}
	if createRes.Data.TaskID == "" {
		t.Fatal("expected task id")
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/tasks/"+createRes.Data.TaskID+"/commit-message/generate", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST commit-message/generate failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST commit-message/generate expected 200, got %d", resp.StatusCode)
	}
	var genRes struct {
		Data struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&genRes); err != nil {
		t.Fatalf("decode commit-message response failed: %v", err)
	}
	if genRes.Data.Message != "feat: fallback to cli helper" {
		t.Fatalf("expected fallback cli message, got %q", genRes.Data.Message)
	}
	if !cliCalled {
		t.Fatal("expected cli helper called when openai config is incomplete")
	}
}
