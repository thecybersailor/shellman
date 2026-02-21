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
		"context_json:",
		"\"task_context\"",
		"\"prev_flag\":\"notify\"",
		"\"prev_status_message\":\"need check\"",
		"\"current_command\":\"bash\"",
		"\"output_tail\":\"$ echo ok\"",
		"\"cwd\":\"/tmp/repo\"",
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
