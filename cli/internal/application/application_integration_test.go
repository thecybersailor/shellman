package application

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStartApplication_LocalMode_HealthAndProjectsFlow(t *testing.T) {
	repo := initGitRepoForTest(t)
	port := pickFreePort(t)
	cfgDir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app, err := StartApplication(ctx, StartOptions{
		Mode:      "local",
		ConfigDir: cfgDir,
		DBDSN:     fmt.Sprintf("file:startapp_it_%d?mode=memory&cache=shared", time.Now().UnixNano()),
		LocalHost: "127.0.0.1",
		LocalPort: port,
		WebUI: WebUIOptions{
			Mode:        "dev",
			DevProxyURL: "http://127.0.0.1:15173",
		},
	})
	if err != nil {
		t.Fatalf("StartApplication failed: %v", err)
	}

	runDone := make(chan error, 1)
	go func() {
		runDone <- app.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		_ = app.Shutdown(context.Background())
		select {
		case <-runDone:
		case <-time.After(4 * time.Second):
			t.Fatal("app run goroutine did not exit")
		}
	})

	baseURL := app.LocalAPIBaseURL()
	waitHTTPReady(t, baseURL+"/healthz", 5*time.Second)

	postBody, _ := json.Marshal(map[string]any{
		"project_id": "p_startapp",
		"repo_root":  repo,
	})
	postResp, err := http.Post(baseURL+"/api/v1/projects/active", "application/json", bytes.NewReader(postBody))
	if err != nil {
		t.Fatalf("POST active project failed: %v", err)
	}
	defer func() { _ = postResp.Body.Close() }()
	if postResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on add project, got %d", postResp.StatusCode)
	}

	listResp, err := http.Get(baseURL + "/api/v1/projects/active")
	if err != nil {
		t.Fatalf("GET active projects failed: %v", err)
	}
	defer func() { _ = listResp.Body.Close() }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on list projects, got %d", listResp.StatusCode)
	}
	var body struct {
		OK   bool `json:"ok"`
		Data []struct {
			ProjectID string `json:"project_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode list projects failed: %v", err)
	}
	if !body.OK || len(body.Data) == 0 || body.Data[0].ProjectID != "p_startapp" {
		t.Fatalf("unexpected projects payload: %+v", body)
	}
}

func waitHTTPReady(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("http endpoint not ready: %s", url)
}

func initGitRepoForTest(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo failed: %v", err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v output=%s", args, err, string(out))
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("start app test\n"), 0o644); err != nil {
		t.Fatalf("write readme failed: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")
	return repo
}
