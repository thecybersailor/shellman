//go:build e2e_tty

package main

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"shellman/cli/internal/config"
	"shellman/cli/internal/protocol"
)

func TestStartApplication_RealCodexLifecycle_NonUIRequestOnly(t *testing.T) {
	requireCommand(t, "tmux")
	requireCommand(t, "zsh")
	requireCommand(t, "codex")

	socket := fmt.Sprintf("tt_startapp_%d", time.Now().UnixNano())
	session := fmt.Sprintf("startapp_%d", time.Now().UnixNano())
	t.Setenv("SHELLMAN_TMUX_SOCKET", socket)
	t.Setenv("SHELLMAN_CONFIG_DIR", t.TempDir())

	runTmuxE2E(t, socket, "-f", "/dev/null", "new-session", "-d", "-s", session, "zsh -i")
	t.Cleanup(func() { runTmuxE2ENoFail(socket, "kill-server") })

	port := pickFreePortE2E(t)
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- runLocal(ctx, io.Discard, config.Config{
			Mode:             "local",
			LocalHost:        "127.0.0.1",
			LocalPort:        port,
			TmuxSocket:       socket,
			WebUIMode:        "dev",
			WebUIDevProxyURL: "http://127.0.0.1:15173",
			WebUIDistDir:     "../webui/dist",
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-runDone:
		case <-time.After(8 * time.Second):
			t.Fatal("runLocal goroutine did not stop")
		}
	})

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitHTTPReadyE2E(t, baseURL+"/healthz", 10*time.Second)

	repo := initGitRepoE2E(t)
	status, body := postJSONE2E(t, baseURL+"/api/v1/projects/active", map[string]any{
		"project_id": "p1",
		"repo_root":  repo,
	})
	if status != 200 {
		t.Fatalf("add active project failed status=%d body=%s", status, string(body))
	}

	status, body = postJSONE2E(t, baseURL+"/api/v1/projects/p1/panes/root", map[string]any{"title": "root"})
	if status != 200 {
		t.Fatalf("create root pane failed status=%d body=%s", status, string(body))
	}
	var createResp struct {
		OK   bool `json:"ok"`
		Data struct {
			TaskID     string `json:"task_id"`
			PaneTarget string `json:"pane_target"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &createResp); err != nil {
		t.Fatalf("decode create root pane response failed: %v body=%s", err, string(body))
	}
	taskID := strings.TrimSpace(createResp.Data.TaskID)
	paneTarget := strings.TrimSpace(createResp.Data.PaneTarget)
	if taskID == "" || paneTarget == "" {
		t.Fatalf("invalid create response: %s", string(body))
	}

	client := wsClientConnectE2E(t, baseURL)
	defer func() { _ = client.Close(1000, "done") }()

	wsSendMessageE2E(t, client, protocol.Message{
		ID:   "req_select_1",
		Type: "req",
		Op:   "tmux.select_pane",
		Payload: protocol.MustRaw(map[string]any{
			"target": paneTarget,
			"cols":   120,
			"rows":   40,
		}),
	})
	wsWaitTermOutputResetE2E(t, client, 8*time.Second)

	waitPaneCommandContainsE2E(t, socket, paneTarget, "zsh", 15*time.Second)
	time.Sleep(2 * time.Second)

	status, body = postJSONE2E(t, baseURL+"/api/v1/tasks/"+taskID+"/messages", map[string]any{
		"source": "tty_write_stdin",
		"input":  "codex",
	})
	if status != 200 {
		t.Fatalf("send codex text failed status=%d body=%s", status, string(body))
	}
	status, body = postJSONE2E(t, baseURL+"/api/v1/tasks/"+taskID+"/messages", map[string]any{
		"source": "tty_write_stdin",
		"input":  "\r",
	})
	if status != 200 {
		t.Fatalf("send codex enter failed status=%d body=%s", status, string(body))
	}

	waitCodexDetectedE2E(t, baseURL, taskID, socket, paneTarget, 30*time.Second)

	noDelayExpect := fmt.Sprintf("%x", md5.Sum([]byte("shellman-e2e-no-delay")))
	noDelayPrompt := "Reply with ONLY the lowercase md5 hex of: shellman-e2e-no-delay"
	status, body = postJSONE2E(t, baseURL+"/api/v1/tasks/"+taskID+"/messages", map[string]any{
		"source": "tty_write_stdin",
		"input":  noDelayPrompt + "\r",
	})
	if status != 200 {
		t.Fatalf("send no-delay prompt failed status=%d body=%s", status, string(body))
	}
	if waitTaskPaneOutputContainsWithinE2E(baseURL, taskID, noDelayExpect, 12*time.Second) {
		t.Fatalf("unexpected: no-delay submit succeeded, expected hash=%q tail=%q", noDelayExpect, tailStringE2E(getTaskPaneE2E(t, baseURL, taskID).Output, 1200))
	}

	delayedExpect := fmt.Sprintf("%x", md5.Sum([]byte("shellman-e2e-delayed")))
	delayedPrompt := "Reply with ONLY the lowercase md5 hex of: shellman-e2e-delayed"
	status, body = postJSONE2E(t, baseURL+"/api/v1/tasks/"+taskID+"/messages", map[string]any{
		"source": "tty_write_stdin",
		"input":  delayedPrompt,
	})
	if status != 200 {
		t.Fatalf("send delayed prompt text failed status=%d body=%s", status, string(body))
	}
	time.Sleep(50 * time.Millisecond)
	status, body = postJSONE2E(t, baseURL+"/api/v1/tasks/"+taskID+"/messages", map[string]any{
		"source": "tty_write_stdin",
		"input":  "\r",
	})
	if status != 200 {
		t.Fatalf("send delayed prompt enter failed status=%d body=%s", status, string(body))
	}

	waitTaskPaneOutputContainsE2E(t, baseURL, taskID, delayedExpect, 120*time.Second)

	status, body = postJSONE2E(t, baseURL+"/api/v1/tasks/"+taskID+"/messages", map[string]any{"source": "tty_write_stdin", "input": "/"})
	if status != 200 {
		t.Fatalf("send / failed status=%d body=%s", status, string(body))
	}
	time.Sleep(500 * time.Millisecond)
	status, body = postJSONE2E(t, baseURL+"/api/v1/tasks/"+taskID+"/messages", map[string]any{"source": "tty_write_stdin", "input": "ex"})
	if status != 200 {
		t.Fatalf("send ex failed status=%d body=%s", status, string(body))
	}
	time.Sleep(500 * time.Millisecond)
	status, body = postJSONE2E(t, baseURL+"/api/v1/tasks/"+taskID+"/messages", map[string]any{"source": "tty_write_stdin", "input": "\x1b[B"})
	if status != 200 {
		t.Fatalf("send down failed status=%d body=%s", status, string(body))
	}
	time.Sleep(500 * time.Millisecond)
	status, body = postJSONE2E(t, baseURL+"/api/v1/tasks/"+taskID+"/messages", map[string]any{"source": "tty_write_stdin", "input": "\r"})
	if status != 200 {
		t.Fatalf("send enter failed status=%d body=%s", status, string(body))
	}

	waitPaneCommandContainsE2E(t, socket, paneTarget, "zsh", 30*time.Second)
}
