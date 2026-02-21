package agentloop

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTaskCurrentSetFlagTool_ValidateAndExecute(t *testing.T) {
	called := false
	tool := &TaskCurrentSetFlagTool{
		Exec: func(ctx context.Context, taskID, flag, statusMessage string) (string, *ToolError) {
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
	if err == nil || err.Message != "INVALID_FLAG_KEY" {
		t.Fatalf("expected INVALID_FLAG_KEY, got %v", err)
	}
	if strings.TrimSpace(err.Suggest) == "" {
		t.Fatalf("expected suggest, got %#v", err)
	}
}

func TestWriteStdinTool_SendsRawInputWithoutNewlineAppend(t *testing.T) {
	var gotInput string
	var gotTimeout int
	tool := &WriteStdinTool{
		Exec: func(ctx context.Context, taskID, input string, timeoutMs int) (string, *ToolError) {
			if taskID != "t1" {
				t.Fatalf("expected task_id=t1, got %q", taskID)
			}
			gotInput = input
			gotTimeout = timeoutMs
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
	if gotTimeout != 1800 {
		t.Fatalf("expected default timeout_ms=1800, got %d", gotTimeout)
	}
}

func TestWriteStdinTool_RejectsInvalidTimeout(t *testing.T) {
	tool := &WriteStdinTool{
		Exec: func(ctx context.Context, taskID, input string, timeoutMs int) (string, *ToolError) {
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"input":"echo hi","timeout_ms":99}`), "call_1")
	if err == nil || err.Message != "INVALID_TIMEOUT_MS" {
		t.Fatalf("expected INVALID_TIMEOUT_MS, got %v", err)
	}
	if strings.TrimSpace(err.Suggest) == "" {
		t.Fatalf("expected suggest, got %#v", err)
	}
}

func TestWriteStdinTool_ShellModeRejectsCommandWithoutSubmit(t *testing.T) {
	called := false
	tool := &WriteStdinTool{
		Exec: func(ctx context.Context, taskID, input string, timeoutMs int) (string, *ToolError) {
			called = true
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1"})
	ctx = WithAllowedToolNames(ctx, []string{"exec_command", "write_stdin"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"input":"echo hi"}`), "call_1")
	if err == nil || !strings.Contains(err.Message, "SHELL_WRITE_STDIN_COMMAND_MISSING_SUBMIT") {
		t.Fatalf("expected missing submit error, got %v", err)
	}
	if !strings.Contains(err.Suggest, "exec_command") {
		t.Fatalf("expected suggest to mention exec_command, got %#v", err)
	}
	if called {
		t.Fatal("executor should not be called when shell command misses submit")
	}
}

func TestWriteStdinTool_ShellModeAllowsCommandWithCR(t *testing.T) {
	called := false
	tool := &WriteStdinTool{
		Exec: func(ctx context.Context, taskID, input string, timeoutMs int) (string, *ToolError) {
			called = true
			if input != "echo hi\r" {
				t.Fatalf("expected raw input with CR, got %q", input)
			}
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1"})
	ctx = WithAllowedToolNames(ctx, []string{"exec_command", "write_stdin"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"input":"echo hi\r"}`), "call_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("executor should be called when CR is present")
	}
}

func TestWriteStdinTool_AIAgentModeDoesNotApplyShellSubmitGuard(t *testing.T) {
	called := false
	tool := &WriteStdinTool{
		Exec: func(ctx context.Context, taskID, input string, timeoutMs int) (string, *ToolError) {
			called = true
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1"})
	ctx = WithAllowedToolNames(ctx, []string{"task.input_prompt", "write_stdin"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"input":"Please summarize this file"}`), "call_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("executor should be called in ai_agent mode")
	}
}

func TestWriteStdinTool_ShellModeAllowsIncompleteTypingState(t *testing.T) {
	called := false
	tool := &WriteStdinTool{
		Exec: func(ctx context.Context, taskID, input string, timeoutMs int) (string, *ToolError) {
			called = true
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1"})
	ctx = WithAllowedToolNames(ctx, []string{"exec_command", "write_stdin"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"input":"echo '"}`), "call_1")
	if err != nil {
		t.Fatalf("unexpected error for incomplete typing state: %v", err)
	}
	if !called {
		t.Fatal("executor should be called for incomplete typing state")
	}
}

func TestTaskInputPromptTool_AppendsEnter(t *testing.T) {
	var gotPrompt string
	tool := &TaskInputPromptTool{
		Exec: func(ctx context.Context, taskID, prompt string) (string, *ToolError) {
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
		Exec: func(ctx context.Context, taskID, prompt string) (string, *ToolError) {
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

func TestReadFileTool_ValidateAndExecute(t *testing.T) {
	var gotPath string
	var gotMaxChars int
	tool := &ReadFileTool{
		Exec: func(ctx context.Context, taskID, path string, maxChars int) (string, *ToolError) {
			if taskID != "t1" {
				t.Fatalf("expected task_id=t1, got %q", taskID)
			}
			gotPath = path
			gotMaxChars = maxChars
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1"})
	out, err := tool.Execute(ctx, json.RawMessage(`{"path":"README.md"}`), "call_1")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected output: %s", out)
	}
	if gotPath != "README.md" {
		t.Fatalf("unexpected path: %q", gotPath)
	}
	if gotMaxChars != 24000 {
		t.Fatalf("expected default max_chars=24000, got %d", gotMaxChars)
	}
}

func TestReadFileTool_RejectsInvalidMaxChars(t *testing.T) {
	tool := &ReadFileTool{
		Exec: func(ctx context.Context, taskID, path string, maxChars int) (string, *ToolError) {
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t1"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"path":"README.md","max_chars":64}`), "call_1")
	if err == nil || err.Message != "INVALID_MAX_CHARS" {
		t.Fatalf("expected INVALID_MAX_CHARS, got %v", err)
	}
	if strings.TrimSpace(err.Suggest) == "" {
		t.Fatalf("expected suggest, got %#v", err)
	}
}

func TestTaskChildSendMessageTool_ValidateInput(t *testing.T) {
	tool := &TaskChildSendMessageTool{Exec: func(context.Context, string, string, string) (string, *ToolError) {
		return `{"ok":true}`, nil
	}}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t_parent"})

	_, err := tool.Execute(ctx, json.RawMessage(`{"task_id":"","message":"hello"}`), "call_1")
	if err == nil || err.Message != "INVALID_TASK_ID" {
		t.Fatalf("expected INVALID_TASK_ID, got %v", err)
	}
	if strings.TrimSpace(err.Suggest) == "" {
		t.Fatalf("expected suggest, got %#v", err)
	}

	_, err = tool.Execute(ctx, json.RawMessage(`{"task_id":"t_child","message":""}`), "call_2")
	if err == nil || err.Message != "INVALID_MESSAGE" {
		t.Fatalf("expected INVALID_MESSAGE, got %v", err)
	}
	if strings.TrimSpace(err.Suggest) == "" {
		t.Fatalf("expected suggest, got %#v", err)
	}
}

func TestTaskChildGetTTYOutputTool_ValidateInput(t *testing.T) {
	tool := &TaskChildGetTTYOutputTool{Exec: func(context.Context, string, string, int) (string, *ToolError) {
		return `{"ok":true}`, nil
	}}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t_parent"})

	_, err := tool.Execute(ctx, json.RawMessage(`{"task_id":"","offset":0}`), "call_1")
	if err == nil || err.Message != "INVALID_TASK_ID" {
		t.Fatalf("expected INVALID_TASK_ID, got %v", err)
	}
	if strings.TrimSpace(err.Suggest) == "" {
		t.Fatalf("expected suggest, got %#v", err)
	}
}

func TestTaskCurrentTools_RequireTaskContext(t *testing.T) {
	setTool := &TaskCurrentSetFlagTool{Exec: func(ctx context.Context, taskID, flag, statusMessage string) (string, *ToolError) {
		return "", nil
	}}
	_, err := setTool.Execute(context.Background(), json.RawMessage(`{"flag":"success","status_message":"done"}`), "call_1")
	if err == nil || err.Message != "TASK_CONTEXT_MISSING" {
		t.Fatalf("expected TASK_CONTEXT_MISSING, got %v", err)
	}
	if strings.TrimSpace(err.Suggest) == "" {
		t.Fatalf("expected suggest, got %#v", err)
	}

	inputTool := &WriteStdinTool{Exec: func(ctx context.Context, taskID, input string, timeoutMs int) (string, *ToolError) {
		return "", nil
	}}
	_, err = inputTool.Execute(context.Background(), json.RawMessage(`{"input":"echo hi"}`), "call_2")
	if err == nil || err.Message != "TASK_CONTEXT_MISSING" {
		t.Fatalf("expected TASK_CONTEXT_MISSING, got %v", err)
	}
	if strings.TrimSpace(err.Suggest) == "" {
		t.Fatalf("expected suggest, got %#v", err)
	}
}

func TestTaskTools_ToolContract_OnlyActionTools(t *testing.T) {
	expected := []string{
		"task.current.set_flag",
		"write_stdin",
		"exec_command",
		"readfile",
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
