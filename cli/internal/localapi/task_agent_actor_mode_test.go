package localapi

import "testing"

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
