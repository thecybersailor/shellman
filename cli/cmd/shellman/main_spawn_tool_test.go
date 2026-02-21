package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

type spawnCall struct {
	Method string
	Path   string
	Body   string
}

func TestEnsureCommandEndsWithEnter(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"echo hi", "echo hi\r"},
		{"echo hi\n", "echo hi\n"},
		{"echo hi\r", "echo hi\r"},
		{"echo hi<ENTER>", "echo hi\r"},
	}
	for _, tc := range tests {
		if got := ensureCommandEndsWithEnter(tc.in); got != tc.want {
			t.Fatalf("in=%q got=%q want=%q", tc.in, got, tc.want)
		}
	}
}

func TestExecuteTaskChildSpawnAction_AutoEnterSidecarModeAndPrompt(t *testing.T) {
	calls := make([]spawnCall, 0, 8)
	callTaskTool := func(method, path string, payload any) (string, error) {
		body := ""
		if payload != nil {
			raw, _ := json.Marshal(payload)
			body = string(raw)
		}
		calls = append(calls, spawnCall{Method: method, Path: path, Body: body})
		switch {
		case method == http.MethodPost && path == "/api/v1/tasks/t_parent/panes/child":
			return `{"ok":true,"data":{"task_id":"t_child","run_id":"r1","pane_target":"e2e:0.2"}}`, nil
		case method == http.MethodPatch && path == "/api/v1/tasks/t_child/description":
			return `{"ok":true}`, nil
		case method == http.MethodGet && path == "/api/v1/tasks/t_parent/sidecar-mode":
			return `{"ok":true,"data":{"sidecar_mode":"autopilot"}}`, nil
		case method == http.MethodPatch && path == "/api/v1/tasks/t_child/sidecar-mode":
			return `{"ok":true}`, nil
		case method == http.MethodPost && path == "/api/v1/tasks/t_child/messages":
			return `{"ok":true}`, nil
		default:
			return `{"ok":true}`, nil
		}
	}

	out, err := executeTaskChildSpawnAction(callTaskTool, "t_parent", "echo hi", "child", "desc", "fix this")
	if err != nil {
		t.Fatalf("execute spawn action failed: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}

	var sawCommandWithEnter bool
	var sawSidecarModeCopy bool
	var sawPromptMessage bool
	for _, c := range calls {
		if c.Method == http.MethodPost && c.Path == "/api/v1/tasks/t_child/messages" && c.Body != "" {
			var payload map[string]any
			_ = json.Unmarshal([]byte(c.Body), &payload)
			if payload["source"] == "tty_write_stdin" && payload["input"] == "echo hi\r" {
				sawCommandWithEnter = true
			}
			if payload["source"] == "parent_message" && payload["content"] == "fix this" {
				sawPromptMessage = true
			}
		}
		if c.Method == http.MethodPatch && c.Path == "/api/v1/tasks/t_child/sidecar-mode" && c.Body == `{"sidecar_mode":"autopilot"}` {
			sawSidecarModeCopy = true
		}
	}
	if !sawCommandWithEnter {
		t.Fatal("expected command auto-enter send")
	}
	if !sawSidecarModeCopy {
		t.Fatal("expected parent sidecar_mode copied to child")
	}
	if !sawPromptMessage {
		t.Fatal("expected prompt sent to child")
	}
}

func TestBuildInputPromptStepsForCommand_Codex(t *testing.T) {
	steps, err := buildInputPromptStepsForCommand("codex (/Users/wanglei/.)", "请继续执行")
	if err != nil {
		t.Fatalf("build steps failed: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("unexpected steps count: %d", len(steps))
	}
	if steps[0].Input != "请继续执行" {
		t.Fatalf("unexpected first step input: %q", steps[0].Input)
	}
	if steps[1].Input != "\r" {
		t.Fatalf("unexpected second step input: %q", steps[1].Input)
	}
	if steps[1].Delay != 50*time.Millisecond {
		t.Fatalf("unexpected second step delay: %v", steps[1].Delay)
	}
}

func TestBuildInputPromptStepsForCommand_Default(t *testing.T) {
	steps, err := buildInputPromptStepsForCommand("zsh", "echo hi")
	if err != nil {
		t.Fatalf("build steps failed: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("unexpected steps count: %d", len(steps))
	}
	if steps[0].Input != "echo hi" {
		t.Fatalf("unexpected first step input: %q", steps[0].Input)
	}
	if steps[1].Input != "\r" {
		t.Fatalf("unexpected second step input: %q", steps[1].Input)
	}
}
