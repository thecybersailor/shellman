package localapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildTaskAgentAutoProgressDisplayContent_ReturnsRuntimeTypedJSON(t *testing.T) {
	raw := buildTaskAgentAutoProgressDisplayContent("t_1", "done", "r_1")
	var parsed struct {
		Text string `json:"text"`
		Meta struct {
			DisplayType string `json:"display_type"`
			Source      string `json:"source"`
			Event       string `json:"event"`
			TaskID      string `json:"task_id"`
			RunID       string `json:"run_id"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("expected json content, got err=%v raw=%q", err, raw)
	}
	if parsed.Text != "done" {
		t.Fatalf("expected text=done, got %q", parsed.Text)
	}
	if parsed.Meta.DisplayType != "runtime" || parsed.Meta.Source != "tty_output" || parsed.Meta.Event != "tty_output" {
		t.Fatalf("unexpected runtime meta: %#v", parsed.Meta)
	}
	if parsed.Meta.TaskID != "t_1" || parsed.Meta.RunID != "r_1" {
		t.Fatalf("unexpected task/run ids: %#v", parsed.Meta)
	}
}

func TestBuildTaskAgentAutoProgressPrompt_AlwaysInjectsRequiredContext(t *testing.T) {
	prompt := buildTaskAgentAutoProgressPrompt(TaskAgentAutoProgressPromptInput{
		TaskID:            "t_1",
		RunID:             "r_1",
		Name:              "task name",
		Description:       "task desc",
		Summary:           "done",
		HistoryBlock:      "[assistant#2] done",
		PrevFlag:          "notify",
		PrevStatusMessage: "need check",
		TTY: TaskAgentTTYContext{
			CurrentCommand: "bash",
			OutputTail:     "$ echo ok",
			Cwd:            "/tmp/repo",
		},
		ParentTask: &TaskAgentParentContext{
			Name:          "parent",
			Description:   "parent-desc",
			Flag:          "success",
			StatusMessage: "done",
		},
		ChildTasks: []TaskAgentChildContext{{
			TaskID:        "t_child",
			Name:          "child",
			Description:   "child-desc",
			Flag:          "notify",
			StatusMessage: "waiting",
			ReportMessage: "report",
		}},
	})
	required := []string{
		"TTY_OUTPUT_EVENT",
		"conversation_history:",
		"[assistant#2] done",
		"terminal_screen_state_json:",
		"\"terminal_screen_state\"",
		"\"prev_flag\":\"notify\"",
		"\"prev_status_message\":\"need check\"",
		"\"current_command\":\"bash\"",
		"\"viewport_text\":\"$ echo ok\"",
		"\"cwd\":\"/tmp/repo\"",
		"\"cursor\"",
		"\"cursor_hint\"",
		"\"cursor_semantic\"",
		"\"parent_task\"",
		"\"child_tasks\"",
		"\"report_message\":\"report\"",
		"Rules:",
		"respond with short action-oriented summary after tool calls.",
	}
	for _, s := range required {
		if !strings.Contains(prompt, s) {
			t.Fatalf("expected prompt contains %q, got %q", s, prompt)
		}
	}
}

func TestBuildTaskAgentUserPrompt_IncludesConversationHistoryBlock(t *testing.T) {
	prompt := buildTaskAgentUserPrompt(
		"do X",
		"",
		"",
		TaskAgentTTYContext{},
		nil,
		nil,
		"[user#1] hi",
	)
	required := []string{
		"USER_INPUT_EVENT",
		"conversation_history:",
		"[user#1] hi",
		"terminal_screen_state_json:",
	}
	for _, s := range required {
		if !strings.Contains(prompt, s) {
			t.Fatalf("expected prompt contains %q, got %q", s, prompt)
		}
	}
}

func TestBuildTaskAgentAutoProgressPrompt_IncludesConversationHistoryBlock(t *testing.T) {
	prompt := buildTaskAgentAutoProgressPrompt(TaskAgentAutoProgressPromptInput{
		TaskID:       "t_1",
		Summary:      "idle",
		HistoryBlock: "[assistant#2] done",
	})
	if !strings.Contains(prompt, "conversation_history:") {
		t.Fatalf("expected conversation_history section, got %q", prompt)
	}
	if !strings.Contains(prompt, "[assistant#2] done") {
		t.Fatalf("expected history content in prompt, got %q", prompt)
	}
}

func TestBuildTaskAgentPrompt_CursorSemanticReadyAtShellPrompt(t *testing.T) {
	prompt := buildTaskAgentUserPrompt(
		"ls",
		"",
		"",
		TaskAgentTTYContext{
			CurrentCommand: "bash",
			OutputTail:     "$ ",
			HasCursor:      true,
			CursorX:        2,
			CursorY:        20,
		},
		nil,
		nil,
		"",
	)
	if !strings.Contains(prompt, "\"cursor_semantic\":\"shell_prompt_ready\"") {
		t.Fatalf("expected shell_prompt_ready semantic, got %q", prompt)
	}
}

func TestBuildTaskAgentPrompt_CursorSemanticUnavailable(t *testing.T) {
	prompt := buildTaskAgentUserPrompt(
		"ls",
		"",
		"",
		TaskAgentTTYContext{
			CurrentCommand: "bash",
			OutputTail:     "running task",
			HasCursor:      false,
		},
		nil,
		nil,
		"",
	)
	if !strings.Contains(prompt, "\"cursor_semantic\":\"cursor_unavailable\"") {
		t.Fatalf("expected cursor_unavailable semantic, got %q", prompt)
	}
}
