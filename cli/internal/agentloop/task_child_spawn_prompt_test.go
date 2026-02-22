package agentloop

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTaskChildSpawnTool_RequiresPromptAndPassesToExec(t *testing.T) {
	var gotTaskID, gotCommand, gotTitle, gotDesc, gotPrompt, gotTaskRole string
	tool := &TaskChildSpawnTool{
		Exec: func(ctx context.Context, taskID, command, title, description, prompt, taskRole string) (string, *ToolError) {
			gotTaskID, gotCommand, gotTitle, gotDesc, gotPrompt, gotTaskRole = taskID, command, title, description, prompt, taskRole
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t_parent"})
	out, err := tool.Execute(ctx, json.RawMessage(`{"command":"echo hi","title":"child","description":"d","prompt":"p","task_role":"executor"}`), "call_1")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected output: %s", out)
	}
	if gotTaskID != "t_parent" || gotCommand != "echo hi" || gotTitle != "child" || gotDesc != "d" || gotPrompt != "p" || gotTaskRole != "executor" {
		t.Fatalf("unexpected args: %q %q %q %q %q %q", gotTaskID, gotCommand, gotTitle, gotDesc, gotPrompt, gotTaskRole)
	}
}

func TestTaskChildSpawnTool_InvalidSpawnInputWhenPromptMissing(t *testing.T) {
	tool := &TaskChildSpawnTool{Exec: func(context.Context, string, string, string, string, string, string) (string, *ToolError) {
		return `{"ok":true}`, nil
	}}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t_parent"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"command":"echo hi","title":"child","description":"d","task_role":"executor"}`), "call_1")
	if err == nil || err.Error() != "INVALID_SPAWN_INPUT" {
		t.Fatalf("expected INVALID_SPAWN_INPUT, got %v", err)
	}
}

func TestTaskChildSpawnTool_InvalidSpawnInputWhenTaskRoleMissing(t *testing.T) {
	tool := &TaskChildSpawnTool{Exec: func(context.Context, string, string, string, string, string, string) (string, *ToolError) {
		return `{"ok":true}`, nil
	}}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t_parent"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"command":"echo hi","title":"child","description":"d","prompt":"p"}`), "call_1")
	if err == nil || err.Error() != "INVALID_SPAWN_INPUT" {
		t.Fatalf("expected INVALID_SPAWN_INPUT, got %v", err)
	}
}

func TestTaskChildSpawnTool_InvalidSpawnInputWhenTaskRoleInvalid(t *testing.T) {
	tool := &TaskChildSpawnTool{Exec: func(context.Context, string, string, string, string, string, string) (string, *ToolError) {
		return `{"ok":true}`, nil
	}}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t_parent"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"command":"echo hi","title":"child","description":"d","prompt":"p","task_role":"full"}`), "call_1")
	if err == nil || err.Error() != "INVALID_SPAWN_INPUT" {
		t.Fatalf("expected INVALID_SPAWN_INPUT, got %v", err)
	}
}
