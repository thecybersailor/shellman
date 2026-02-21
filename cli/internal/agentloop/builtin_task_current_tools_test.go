package agentloop

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestTaskCurrentSetFlagTool_ValidateAndExecute(t *testing.T) {
	called := false
	tool := &TaskCurrentSetFlagTool{
		Exec: func(ctx context.Context, taskID, flag, statusMessage string) (string, error) {
			called = true
			if taskID != "t1" {
				t.Fatalf("expected task_id=t1, got %q", taskID)
			}
			if flag != "success" {
				t.Fatalf("expected flag=success, got %q", flag)
			}
			if statusMessage != "done" {
				t.Fatalf("expected status_message=done, got %q", statusMessage)
			}
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1", ProjectID: "p1", Source: "tty_output"})
	out, err := tool.Execute(ctx, json.RawMessage(`{"flag":"success","status_message":"done"}`), "call_1")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if !called {
		t.Fatal("expected executor called")
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected output: %s", out)
	}

	_, err = tool.Execute(ctx, json.RawMessage(`{"flag":"invalid","status_message":"done"}`), "call_2")
	if err == nil || err.Error() != "INVALID_FLAG_KEY" {
		t.Fatalf("expected INVALID_FLAG_KEY, got %v", err)
	}
}

func TestWriteStdinTool_SendsRawInputWithoutNewlineAppend(t *testing.T) {
	var gotInput string
	tool := &WriteStdinTool{
		Exec: func(ctx context.Context, taskID, input string) (string, error) {
			if taskID != "t1" {
				t.Fatalf("expected task_id=t1, got %q", taskID)
			}
			gotInput = input
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1"})
	out, err := tool.Execute(ctx, json.RawMessage(`{"input":"echo hi"}`), "call_1")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected output: %s", out)
	}
	if gotInput != "echo hi" {
		t.Fatalf("expected raw input without newline append, got %q", gotInput)
	}
}

func TestTaskInputPromptTool_AppendsEnter(t *testing.T) {
	var gotPrompt string
	tool := &TaskInputPromptTool{
		Exec: func(ctx context.Context, taskID, prompt string) (string, error) {
			if taskID != "t1" {
				t.Fatalf("expected task_id=t1, got %q", taskID)
			}
			gotPrompt = prompt
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1"})
	out, err := tool.Execute(ctx, json.RawMessage(`{"prompt":"hello world"}`), "call_1")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected output: %s", out)
	}
	if gotPrompt != "hello world\r" {
		t.Fatalf("expected prompt to append enter, got %q", gotPrompt)
	}
}

func TestTaskInputPromptTool_DoesNotDuplicateEnter(t *testing.T) {
	var gotPrompt string
	tool := &TaskInputPromptTool{
		Exec: func(ctx context.Context, taskID, prompt string) (string, error) {
			if taskID != "t1" {
				t.Fatalf("expected task_id=t1, got %q", taskID)
			}
			gotPrompt = prompt
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1"})
	out, err := tool.Execute(ctx, json.RawMessage("{\"prompt\":\"hello world\\r\"}"), "call_1")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected output: %s", out)
	}
	if gotPrompt != "hello world\r" {
		t.Fatalf("expected prompt keep single enter, got %q", gotPrompt)
	}
}

func TestTaskChildSendMessageTool_ValidateInput(t *testing.T) {
	tool := &TaskChildSendMessageTool{Exec: func(context.Context, string, string, string) (string, error) {
		return `{"ok":true}`, nil
	}}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t_parent"})

	_, err := tool.Execute(ctx, json.RawMessage(`{"task_id":"","message":"hello"}`), "call_1")
	if err == nil || err.Error() != "INVALID_TASK_ID" {
		t.Fatalf("expected INVALID_TASK_ID, got %v", err)
	}

	_, err = tool.Execute(ctx, json.RawMessage(`{"task_id":"t_child","message":""}`), "call_2")
	if err == nil || err.Error() != "INVALID_MESSAGE" {
		t.Fatalf("expected INVALID_MESSAGE, got %v", err)
	}
}

func TestTaskChildGetTTYOutputTool_ValidateInput(t *testing.T) {
	tool := &TaskChildGetTTYOutputTool{Exec: func(context.Context, string, string, int) (string, error) {
		return `{"ok":true}`, nil
	}}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t_parent"})

	_, err := tool.Execute(ctx, json.RawMessage(`{"task_id":"","offset":0}`), "call_1")
	if err == nil || err.Error() != "INVALID_TASK_ID" {
		t.Fatalf("expected INVALID_TASK_ID, got %v", err)
	}
}

func TestTaskCurrentTools_RequireTaskContext(t *testing.T) {
	setTool := &TaskCurrentSetFlagTool{Exec: func(ctx context.Context, taskID, flag, statusMessage string) (string, error) {
		return "", nil
	}}
	_, err := setTool.Execute(context.Background(), json.RawMessage(`{"flag":"success","status_message":"done"}`), "call_1")
	if err == nil || err.Error() != "TASK_CONTEXT_MISSING" {
		t.Fatalf("expected TASK_CONTEXT_MISSING, got %v", err)
	}

	inputTool := &WriteStdinTool{Exec: func(ctx context.Context, taskID, input string) (string, error) {
		return "", nil
	}}
	_, err = inputTool.Execute(context.Background(), json.RawMessage(`{"input":"echo hi"}`), "call_2")
	if err == nil || err.Error() != "TASK_CONTEXT_MISSING" {
		t.Fatalf("expected TASK_CONTEXT_MISSING, got %v", err)
	}
}

func TestTaskTools_ToolContract_OnlyActionTools(t *testing.T) {
	expected := []string{
		"task.current.set_flag",
		"write_stdin",
		"exec_command",
		"task.input_prompt",
		"task.child.get_context",
		"task.child.get_tty_output",
		"task.child.spawn",
		"task.child.send_message",
		"task.parent.report",
	}
	if !reflect.DeepEqual(TaskActionToolContractNames(), expected) {
		t.Fatalf("unexpected contract tools: %#v", TaskActionToolContractNames())
	}
	legacy := []string{
		"gateway_http",
		"task.current.input",
		"task.tty.current_command",
		"task.tty.state",
		"task.tty.output_tail",
		"task.tty.cwd",
	}
	got := map[string]struct{}{}
	for _, name := range TaskActionToolContractNames() {
		got[name] = struct{}{}
	}
	for _, old := range legacy {
		if _, ok := got[old]; ok {
			t.Fatalf("legacy tool should not be in contract: %s", old)
		}
	}
}
