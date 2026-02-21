//go:build e2e_tty

package localapi

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"shellman/cli/internal/projectstate"
)

func TestTaskAgentModeRealtime_RealTmux_NoShellFallbackForUnknownCommand(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}

	socket := fmt.Sprintf("tt_e2e_tty_%d", time.Now().UnixNano())
	session := fmt.Sprintf("e2e_tty_%d", time.Now().UnixNano())
	paneTarget := session + ":0.0"
	t.Setenv("SHELLMAN_TMUX_SOCKET", socket)

	runTmux(t, socket, "-f", "/dev/null", "new-session", "-d", "-s", session, "bash --noprofile --norc")
	t.Cleanup(func() {
		_ = runTmuxNoFail(socket, "kill-server")
	})

	waitForTmuxCommand(t, socket, paneTarget, "bash", 4*time.Second)

	store := projectstate.NewStore(t.TempDir())
	mode := projectstate.SidecarModeAutopilot
	initialCmd := "bash"
	taskID := fmt.Sprintf("t_e2e_tty_%d", time.Now().UnixNano())
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:         taskID,
		ProjectID:      "p1",
		SidecarMode:    &mode,
		CurrentCommand: &initialCmd,
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}
	if err := store.SavePanes(projectstate.PanesIndex{
		taskID: {
			TaskID:     taskID,
			PaneID:     paneTarget,
			PaneTarget: paneTarget,
		},
	}); err != nil {
		t.Fatalf("save panes failed: %v", err)
	}

	srv := NewServer(Deps{})
	gotMode, gotCommand, gotTools := srv.resolveTaskAgentToolModeAndNamesRealtime(store, "p1", taskID, "user_input")
	if gotMode != string(taskAgentToolModeShell) {
		t.Fatalf("shell mode expected at prompt, got mode=%q command=%q tools=%v", gotMode, gotCommand, gotTools)
	}
	if !containsTool(gotTools, "exec_command") {
		t.Fatalf("shell mode should include exec_command, got tools=%v", gotTools)
	}

	runTmux(t, socket, "send-keys", "-l", "-t", paneTarget, "sleep 5")
	runTmux(t, socket, "send-keys", "-t", paneTarget, "C-m")
	waitForTmuxCommand(t, socket, paneTarget, "sleep", 3*time.Second)

	gotMode, gotCommand, gotTools = srv.resolveTaskAgentToolModeAndNamesRealtime(store, "p1", taskID, "user_input")
	if gotMode != string(taskAgentToolModeDefault) {
		t.Fatalf("default mode expected for unknown command, got mode=%q command=%q tools=%v", gotMode, gotCommand, gotTools)
	}
	if containsTool(gotTools, "exec_command") {
		t.Fatalf("default mode must not include exec_command, got tools=%v", gotTools)
	}
	if !containsTool(gotTools, "write_stdin") {
		t.Fatalf("default mode should keep write_stdin, got tools=%v", gotTools)
	}

	runTmux(t, socket, "send-keys", "-t", paneTarget, "C-c")
}

func TestTaskAgentModeRealtime_RealTmux_RealCodexUsesAIAgentTools(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex not installed")
	}

	socket := fmt.Sprintf("tt_e2e_tty_%d", time.Now().UnixNano())
	session := fmt.Sprintf("e2e_tty_%d", time.Now().UnixNano())
	paneTarget := session + ":0.0"
	t.Setenv("SHELLMAN_TMUX_SOCKET", socket)

	runTmux(t, socket, "-f", "/dev/null", "new-session", "-d", "-s", session, "bash --noprofile --norc")
	t.Cleanup(func() {
		_ = runTmuxNoFail(socket, "kill-server")
	})

	waitForTmuxCommand(t, socket, paneTarget, "bash", 4*time.Second)

	store := projectstate.NewStore(t.TempDir())
	mode := projectstate.SidecarModeAutopilot
	initialCmd := "bash"
	taskID := fmt.Sprintf("t_e2e_tty_codex_%d", time.Now().UnixNano())
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:         taskID,
		ProjectID:      "p1",
		SidecarMode:    &mode,
		CurrentCommand: &initialCmd,
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}
	if err := store.SavePanes(projectstate.PanesIndex{
		taskID: {
			TaskID:     taskID,
			PaneID:     paneTarget,
			PaneTarget: paneTarget,
		},
	}); err != nil {
		t.Fatalf("save panes failed: %v", err)
	}

	srv := NewServer(Deps{})
	gotMode, gotCommand, gotTools := srv.resolveTaskAgentToolModeAndNamesRealtime(store, "p1", taskID, "user_input")
	if gotMode != string(taskAgentToolModeShell) {
		t.Fatalf("shell mode expected before launching codex, got mode=%q command=%q tools=%v", gotMode, gotCommand, gotTools)
	}
	if !containsTool(gotTools, "exec_command") {
		t.Fatalf("shell mode should include exec_command, got tools=%v", gotTools)
	}

	runTmux(t, socket, "send-keys", "-l", "-t", paneTarget, "codex")
	runTmux(t, socket, "send-keys", "-t", paneTarget, "C-m")
	waitForTmuxCommandWithPaneDump(t, socket, paneTarget, "codex", 12*time.Second)
	time.Sleep(300 * time.Millisecond)

	gotMode, gotCommand, gotTools = srv.resolveTaskAgentToolModeAndNamesRealtime(store, "p1", taskID, "user_input")
	if gotMode != string(taskAgentToolModeAIAgent) {
		t.Fatalf("ai-agent mode expected for codex, got mode=%q command=%q tools=%v", gotMode, gotCommand, gotTools)
	}
	if !containsTool(gotTools, "task.input_prompt") {
		t.Fatalf("codex mode should include task.input_prompt, got tools=%v", gotTools)
	}
	if containsTool(gotTools, "exec_command") {
		t.Fatalf("codex mode must not include exec_command, got tools=%v", gotTools)
	}

	runTmux(t, socket, "send-keys", "-t", paneTarget, "C-c")
}

func runTmux(t *testing.T, socket string, args ...string) {
	t.Helper()
	fullArgs := append([]string{"-L", socket}, args...)
	cmd := exec.Command("tmux", fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("tmux %v failed: %v output=%s", args, err, strings.TrimSpace(string(out)))
	}
}

func runTmuxNoFail(socket string, args ...string) error {
	fullArgs := append([]string{"-L", socket}, args...)
	cmd := exec.Command("tmux", fullArgs...)
	_, err := cmd.CombinedOutput()
	return err
}

func tmuxCurrentCommand(t *testing.T, socket, paneTarget string) string {
	t.Helper()
	cmd := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", paneTarget, "#{pane_current_command}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("read pane_current_command failed: %v output=%s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out))
}

func waitForTmuxCommand(t *testing.T, socket, paneTarget, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.EqualFold(tmuxCurrentCommand(t, socket, paneTarget), expected) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("wait command timeout: expected=%q got=%q", expected, tmuxCurrentCommand(t, socket, paneTarget))
}

func waitForTmuxCommandWithPaneDump(t *testing.T, socket, paneTarget, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.EqualFold(tmuxCurrentCommand(t, socket, paneTarget), expected) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	got := tmuxCurrentCommand(t, socket, paneTarget)
	cmd := exec.Command("tmux", "-L", socket, "capture-pane", "-p", "-S", "-120", "-t", paneTarget)
	out, _ := cmd.CombinedOutput()
	t.Fatalf("wait command timeout: expected=%q got=%q pane_tail=%q", expected, got, strings.TrimSpace(string(out)))
}

func containsTool(tools []string, name string) bool {
	for _, item := range tools {
		if strings.TrimSpace(item) == strings.TrimSpace(name) {
			return true
		}
	}
	return false
}
