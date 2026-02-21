//go:build e2e_tty

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"shellman/cli/internal/protocol"
)

type e2eTaskPane struct {
	PaneTarget     string
	CurrentCommand string
	Output         string
}

func requireCommand(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not installed", name)
	}
}

func pickFreePortE2E(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen random port failed: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("unexpected addr type")
	}
	return addr.Port
}

func runTmuxE2E(t *testing.T, socket string, args ...string) {
	t.Helper()
	fullArgs := append([]string{"-L", socket}, args...)
	cmd := exec.Command("tmux", fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v failed: %v output=%s", args, err, strings.TrimSpace(string(out)))
	}
}

func runTmuxE2ENoFail(socket string, args ...string) {
	fullArgs := append([]string{"-L", socket}, args...)
	cmd := exec.Command("tmux", fullArgs...)
	_, _ = cmd.CombinedOutput()
}

func tmuxPaneCurrentCommand(t *testing.T, socket, paneTarget string) string {
	t.Helper()
	cmd := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", paneTarget, "#{pane_current_command}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("read pane_current_command failed: %v output=%s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out))
}

func tmuxPaneTail(t *testing.T, socket, paneTarget string, lines int) string {
	t.Helper()
	if lines <= 0 {
		lines = 120
	}
	cmd := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-S", fmt.Sprintf("-%d", lines), "-t", paneTarget)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("capture-pane failed: %v output=%s", err, strings.TrimSpace(string(out)))
	}
	return string(out)
}

func waitHTTPReadyE2E(t *testing.T, url string, timeout time.Duration) {
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
		time.Sleep(120 * time.Millisecond)
	}
	t.Fatalf("http endpoint not ready: %s", url)
}

func postJSONE2E(t *testing.T, url string, payload any) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s failed: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

func getTaskPaneE2E(t *testing.T, baseURL, taskID string) e2eTaskPane {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/tasks/%s/pane", baseURL, taskID))
	if err != nil {
		t.Fatalf("GET task pane failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET task pane status=%d body=%s", resp.StatusCode, string(body))
	}
	var parsed struct {
		OK   bool `json:"ok"`
		Data struct {
			PaneTarget     string `json:"pane_target"`
			CurrentCommand string `json:"current_command"`
			Snapshot       struct {
				Output string `json:"output"`
			} `json:"snapshot"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode task pane failed: %v body=%s", err, string(body))
	}
	return e2eTaskPane{
		PaneTarget:     strings.TrimSpace(parsed.Data.PaneTarget),
		CurrentCommand: strings.TrimSpace(parsed.Data.CurrentCommand),
		Output:         parsed.Data.Snapshot.Output,
	}
}

func waitPaneCommandContainsE2E(t *testing.T, socket, paneTarget, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	want = strings.ToLower(strings.TrimSpace(want))
	for time.Now().Before(deadline) {
		got := strings.ToLower(strings.TrimSpace(tmuxPaneCurrentCommand(t, socket, paneTarget)))
		if strings.Contains(got, want) {
			return
		}
		time.Sleep(120 * time.Millisecond)
	}
	t.Fatalf("wait pane command timeout: want contains=%q got=%q tail=%s", want, tmuxPaneCurrentCommand(t, socket, paneTarget), tmuxPaneTail(t, socket, paneTarget, 120))
}

func waitCodexDetectedE2E(t *testing.T, baseURL, taskID, socket, paneTarget string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		tmuxCmd := strings.ToLower(strings.TrimSpace(tmuxPaneCurrentCommand(t, socket, paneTarget)))
		pane := getTaskPaneE2E(t, baseURL, taskID)
		apiCmd := strings.ToLower(strings.TrimSpace(pane.CurrentCommand))
		if strings.Contains(tmuxCmd, "codex") || strings.Contains(apiCmd, "codex") {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	pane := getTaskPaneE2E(t, baseURL, taskID)
	t.Fatalf(
		"wait codex detect timeout: tmux_cmd=%q api_cmd=%q tail=%s",
		tmuxPaneCurrentCommand(t, socket, paneTarget),
		pane.CurrentCommand,
		tmuxPaneTail(t, socket, paneTarget, 120),
	)
}

func waitTaskPaneOutputContainsE2E(t *testing.T, baseURL, taskID, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pane := getTaskPaneE2E(t, baseURL, taskID)
		if strings.Contains(pane.Output, want) {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	pane := getTaskPaneE2E(t, baseURL, taskID)
	t.Fatalf("wait pane output timeout: want=%q command=%q tail=%q", want, pane.CurrentCommand, tailStringE2E(pane.Output, 1200))
}

func assertTaskPaneOutputNotContainsWithinE2E(t *testing.T, baseURL, taskID, forbidden string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pane := getTaskPaneE2E(t, baseURL, taskID)
		if strings.Contains(pane.Output, forbidden) {
			t.Fatalf("unexpected output appeared: %q output_tail=%q", forbidden, tailStringE2E(pane.Output, 1200))
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func waitTaskPaneOutputContainsWithinE2E(baseURL, taskID, want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pane := getTaskPaneE2EForBool(baseURL, taskID)
		if strings.Contains(pane.Output, want) {
			return true
		}
		time.Sleep(220 * time.Millisecond)
	}
	return false
}

func getTaskPaneE2EForBool(baseURL, taskID string) e2eTaskPane {
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/tasks/%s/pane", baseURL, taskID))
	if err != nil {
		return e2eTaskPane{}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return e2eTaskPane{}
	}
	var parsed struct {
		Data struct {
			PaneTarget     string `json:"pane_target"`
			CurrentCommand string `json:"current_command"`
			Snapshot       struct {
				Output string `json:"output"`
			} `json:"snapshot"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return e2eTaskPane{}
	}
	return e2eTaskPane{
		PaneTarget:     strings.TrimSpace(parsed.Data.PaneTarget),
		CurrentCommand: strings.TrimSpace(parsed.Data.CurrentCommand),
		Output:         parsed.Data.Snapshot.Output,
	}
}

func wsClientConnectE2E(t *testing.T, baseURL string) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/ws/client/local"
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws client failed: %v", err)
	}
	return conn
}

func wsSendMessageE2E(t *testing.T, conn *websocket.Conn, msg protocol.Message) {
	t.Helper()
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal ws msg failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatalf("ws write failed: %v", err)
	}
}

func wsWaitTermOutputResetE2E(t *testing.T, conn *websocket.Conn, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("ws read failed: %v", err)
		}
		var msg protocol.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.Op != "term.output" {
			continue
		}
		var payload struct {
			Mode string `json:"mode"`
		}
		_ = json.Unmarshal(msg.Payload, &payload)
		if strings.TrimSpace(payload.Mode) == "reset" {
			return
		}
	}
}

func initGitRepoE2E(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo failed: %v", err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v output=%s", args, err, strings.TrimSpace(string(out)))
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("e2e\n"), 0o644); err != nil {
		t.Fatalf("write readme failed: %v", err)
	}
	run("add", "README.md")
	run("commit", "-m", "init")
	return repo
}

func tailStringE2E(text string, n int) string {
	if n <= 0 || len(text) <= n {
		return text
	}
	return text[len(text)-n:]
}
