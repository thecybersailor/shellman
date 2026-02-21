package localapi

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"shellman/cli/internal/projectstate"
)

func TestResolveTaskAgentToolModeFromCommand(t *testing.T) {
	cases := []struct {
		command string
		want    taskAgentToolMode
	}{
		{command: "", want: taskAgentToolModeDefault},
		{command: "   ", want: taskAgentToolModeDefault},
		{command: "codex --ask", want: taskAgentToolModeAIAgent},
		{command: "node (codex)", want: taskAgentToolModeAIAgent},
		{command: "claude", want: taskAgentToolModeAIAgent},
		{command: "gemini -p hi", want: taskAgentToolModeDefault},
		{command: "cursor agent", want: taskAgentToolModeAIAgent},
		{command: "bash", want: taskAgentToolModeShell},
		{command: "zsh", want: taskAgentToolModeShell},
		{command: "npm test", want: taskAgentToolModeDefault},
		{command: "node", want: taskAgentToolModeDefault},
	}
	for _, tc := range cases {
		if got := resolveTaskAgentToolModeFromCommand(tc.command); got != tc.want {
			t.Fatalf("command=%q got=%q want=%q", tc.command, got, tc.want)
		}
	}
}

func TestResolveTaskAgentToolModeAndNames_AutopilotAIAgentIncludesWriteStdin(t *testing.T) {
	store := projectstate.NewStore(t.TempDir())
	mode := projectstate.SidecarModeAutopilot
	cmd := "codex --ask"
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:         "t1",
		ProjectID:      "p1",
		SidecarMode:    &mode,
		CurrentCommand: &cmd,
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}

	gotMode, gotCommand, gotTools := resolveTaskAgentToolModeAndNames(store, "p1", "t1")
	if gotMode != string(taskAgentToolModeAIAgent) {
		t.Fatalf("unexpected tool mode: got=%q want=%q", gotMode, taskAgentToolModeAIAgent)
	}
	if gotCommand != cmd {
		t.Fatalf("unexpected current command: got=%q want=%q", gotCommand, cmd)
	}
	wantTools := []string{
		"task.current.set_flag",
		"task.child.get_context",
		"task.child.get_tty_output",
		"task.child.spawn",
		"task.child.send_message",
		"task.parent.report",
		"task.input_prompt",
		"readfile",
		"write_stdin",
	}
	if !reflect.DeepEqual(gotTools, wantTools) {
		t.Fatalf("unexpected tools: got=%#v want=%#v", gotTools, wantTools)
	}
}

func TestResolveTaskAgentToolModeAndNames_UserTurn_IgnoresSidecarMode(t *testing.T) {
	currentCommand := "codex --ask"
	wantTools := []string{
		"task.current.set_flag",
		"task.child.get_context",
		"task.child.get_tty_output",
		"task.child.spawn",
		"task.child.send_message",
		"task.parent.report",
		"task.input_prompt",
		"readfile",
		"write_stdin",
	}
	for _, mode := range []string{
		projectstate.SidecarModeAdvisor,
		projectstate.SidecarModeObserver,
		projectstate.SidecarModeAutopilot,
	} {
		gotMode, gotCommand, gotTools := resolveTaskAgentToolModeAndNamesFromInputs(currentCommand, mode)
		if gotMode != string(taskAgentToolModeAIAgent) {
			t.Fatalf("mode=%q unexpected tool mode: got=%q", mode, gotMode)
		}
		if gotCommand != currentCommand {
			t.Fatalf("mode=%q unexpected current command: got=%q", mode, gotCommand)
		}
		if !reflect.DeepEqual(gotTools, wantTools) {
			t.Fatalf("mode=%q unexpected tools: got=%#v want=%#v", mode, gotTools, wantTools)
		}
	}
}

func TestResolveTaskAgentToolModeAndNamesRealtime_UsesLivePaneCommandInAutopilot(t *testing.T) {
	store := projectstate.NewStore(t.TempDir())
	taskID := fmt.Sprintf("t_mode_realtime_%d", time.Now().UTC().UnixNano())
	mode := projectstate.SidecarModeAutopilot
	dbCommand := "zsh"
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:         taskID,
		ProjectID:      "p1",
		SidecarMode:    &mode,
		CurrentCommand: &dbCommand,
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}
	if err := store.SavePanes(projectstate.PanesIndex{
		taskID: {
			TaskID:     taskID,
			PaneID:     "1",
			PaneTarget: "sess:1.1",
		},
	}); err != nil {
		t.Fatalf("save panes failed: %v", err)
	}
	gotDBCommand, gotSidecarMode := resolveTaskAgentModeInputs(store, "p1", taskID)
	if gotDBCommand != "zsh" || gotSidecarMode != projectstate.SidecarModeAutopilot {
		t.Fatalf("unexpected db mode inputs: command=%q sidecar=%q", gotDBCommand, gotSidecarMode)
	}

	srv := NewServer(Deps{
		ExecuteCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if strings.TrimSpace(name) != "tmux" {
				t.Fatalf("unexpected command: %s", name)
			}
			return []byte("pane-title\tcodex\tabc\n"), nil
		},
	})
	gotMode, gotCommand, gotTools := srv.resolveTaskAgentToolModeAndNamesRealtime(store, "p1", taskID, "user_input")
	if gotMode != string(taskAgentToolModeAIAgent) {
		t.Fatalf("unexpected mode: got=%q", gotMode)
	}
	if gotCommand != "codex" {
		t.Fatalf("unexpected current command: got=%q", gotCommand)
	}
	if !reflect.DeepEqual(gotTools, []string{
		"task.current.set_flag",
		"task.child.get_context",
		"task.child.get_tty_output",
		"task.child.spawn",
		"task.child.send_message",
		"task.parent.report",
		"task.input_prompt",
		"readfile",
		"write_stdin",
	}) {
		t.Fatalf("unexpected tools: %#v", gotTools)
	}
}

func TestResolveTaskAgentToolModeAndNamesRealtime_DetectMissUsesDefaultMode(t *testing.T) {
	store := projectstate.NewStore(t.TempDir())
	taskID := fmt.Sprintf("t_mode_realtime_default_%d", time.Now().UTC().UnixNano())
	mode := projectstate.SidecarModeAutopilot
	dbCommand := "codex"
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:         taskID,
		ProjectID:      "p1",
		SidecarMode:    &mode,
		CurrentCommand: &dbCommand,
	}); err != nil {
		t.Fatalf("upsert task failed: %v", err)
	}
	if err := store.SavePanes(projectstate.PanesIndex{
		taskID: {
			TaskID:     taskID,
			PaneID:     "1",
			PaneTarget: "sess:1.1",
		},
	}); err != nil {
		t.Fatalf("save panes failed: %v", err)
	}

	srv := NewServer(Deps{
		ExecuteCommand: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if strings.TrimSpace(name) != "tmux" {
				t.Fatalf("unexpected command: %s", name)
			}
			return nil, fmt.Errorf("tmux not available")
		},
	})
	gotMode, gotCommand, gotTools := srv.resolveTaskAgentToolModeAndNamesRealtime(store, "p1", taskID, "user_input")
	if gotMode != string(taskAgentToolModeDefault) {
		t.Fatalf("unexpected mode: got=%q", gotMode)
	}
	if gotCommand != "" {
		t.Fatalf("unexpected current command: got=%q", gotCommand)
	}
	if !reflect.DeepEqual(gotTools, []string{
		"task.current.set_flag",
		"task.child.get_context",
		"task.child.get_tty_output",
		"task.child.spawn",
		"task.child.send_message",
		"task.parent.report",
		"readfile",
		"write_stdin",
	}) {
		t.Fatalf("unexpected tools: %#v", gotTools)
	}
}

func TestResolveTaskAgentToolModeAndNames_AutoTurnDiffersBySidecarMode(t *testing.T) {
	currentCommand := "codex --ask"
	wantFullTools := []string{
		"task.current.set_flag",
		"task.child.get_context",
		"task.child.get_tty_output",
		"task.child.spawn",
		"task.child.send_message",
		"task.parent.report",
		"task.input_prompt",
		"readfile",
		"write_stdin",
	}

	mode, gotCommand, tools := resolveTaskAgentToolModeAndNamesFromInputsForSource(currentCommand, projectstate.SidecarModeAdvisor, "tty_output")
	if mode != string(taskAgentToolModeAIAgent) {
		t.Fatalf("advisor unexpected tool mode: got=%q", mode)
	}
	if gotCommand != currentCommand {
		t.Fatalf("advisor unexpected current command: got=%q", gotCommand)
	}
	if len(tools) != 0 {
		t.Fatalf("advisor auto turn should disable tools, got=%#v", tools)
	}

	mode, gotCommand, tools = resolveTaskAgentToolModeAndNamesFromInputsForSource(currentCommand, projectstate.SidecarModeObserver, "tty_output")
	if mode != string(taskAgentToolModeAIAgent) {
		t.Fatalf("observer unexpected tool mode: got=%q", mode)
	}
	if gotCommand != currentCommand {
		t.Fatalf("observer unexpected current command: got=%q", gotCommand)
	}
	if !reflect.DeepEqual(tools, []string{"task.current.set_flag"}) {
		t.Fatalf("observer auto turn unexpected tools: got=%#v", tools)
	}

	mode, gotCommand, tools = resolveTaskAgentToolModeAndNamesFromInputsForSource(currentCommand, projectstate.SidecarModeAutopilot, "tty_output")
	if mode != string(taskAgentToolModeAIAgent) {
		t.Fatalf("autopilot unexpected tool mode: got=%q", mode)
	}
	if gotCommand != currentCommand {
		t.Fatalf("autopilot unexpected current command: got=%q", gotCommand)
	}
	if !reflect.DeepEqual(tools, wantFullTools) {
		t.Fatalf("autopilot auto turn unexpected tools: got=%#v want=%#v", tools, wantFullTools)
	}
}
