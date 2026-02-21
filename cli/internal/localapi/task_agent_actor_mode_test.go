package localapi

import (
	"reflect"
	"testing"

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
		{command: "claude", want: taskAgentToolModeAIAgent},
		{command: "gemini -p hi", want: taskAgentToolModeAIAgent},
		{command: "cursor agent", want: taskAgentToolModeAIAgent},
		{command: "bash", want: taskAgentToolModeShell},
		{command: "zsh", want: taskAgentToolModeShell},
		{command: "npm test", want: taskAgentToolModeShell},
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
		"write_stdin",
	}
	if !reflect.DeepEqual(gotTools, wantTools) {
		t.Fatalf("unexpected tools: got=%#v want=%#v", gotTools, wantTools)
	}
}
