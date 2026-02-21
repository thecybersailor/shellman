package agentloop

import (
	"context"
	"encoding/json"
	"testing"
)

func TestTaskChildSpawnTool_RequiresPromptAndPassesToExec(t *testing.T) {
	var gotTaskID, gotCommand, gotTitle, gotDesc, gotPrompt string
	tool := &TaskChildSpawnTool{
		Exec: func(ctx context.Context, taskID, command, title, description, prompt string) (string, *ToolError) {
			gotTaskID, gotCommand, gotTitle, gotDesc, gotPrompt = taskID, command, title, description, prompt
			return `{"ok":true}`, nil
		},
	}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t_parent"})
	out, err := tool.Execute(ctx, json.RawMessage(`{"command":"echo hi","title":"child","description":"d","prompt":"p"}`), "call_1")
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected output: %s", out)
	}
	if gotTaskID != "t_parent" || gotCommand != "echo hi" || gotTitle != "child" || gotDesc != "d" || gotPrompt != "p" {
		t.Fatalf("unexpected args: %q %q %q %q %q", gotTaskID, gotCommand, gotTitle, gotDesc, gotPrompt)
	}
}

func TestTaskChildSpawnTool_InvalidSpawnInputWhenPromptMissing(t *testing.T) {
	tool := &TaskChildSpawnTool{Exec: func(context.Context, string, string, string, string, string) (string, *ToolError) {
		return `{"ok":true}`, nil
	}}
	ctx := WithTaskScope(context.Background(), TaskScope{TaskID: "t_parent"})
	_, err := tool.Execute(ctx, json.RawMessage(`{"command":"echo hi","title":"child","description":"d"}`), "call_1")
	if err == nil || err.Error() != "INVALID_SPAWN_INPUT" {
		t.Fatalf("expected INVALID_SPAWN_INPUT, got %v", err)
	}
}
