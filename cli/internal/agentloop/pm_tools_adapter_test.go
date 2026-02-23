package agentloop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPMToolsAdapter_ExecWritePatchDispatch(t *testing.T) {
	adapter := NewPMToolsAdapter(PMToolsAdapterDeps{
		ExecCommand: func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError) {
			if strings.TrimSpace(callID) != "c1" {
				t.Fatalf("unexpected callID: %q", callID)
			}
			if !strings.Contains(string(input), `"cmd":"pwd"`) {
				t.Fatalf("unexpected exec input: %s", string(input))
			}
			return `{"ok":true,"tool":"exec_command"}`, nil
		},
		WriteStdin: func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError) {
			if !strings.Contains(string(input), `"chars":"ls"`) {
				t.Fatalf("unexpected write_stdin input: %s", string(input))
			}
			return `{"ok":true,"tool":"write_stdin"}`, nil
		},
		ApplyPatch: func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError) {
			if !strings.Contains(string(input), "*** Begin Patch") {
				t.Fatalf("unexpected apply_patch input: %s", string(input))
			}
			return `{"ok":true,"tool":"apply_patch"}`, nil
		},
	})

	out, err := adapter.Execute(context.Background(), "exec_command", json.RawMessage(`{"cmd":"pwd"}`), "c1")
	if err != nil {
		t.Fatalf("exec_command failed: %v", err)
	}
	if !strings.Contains(out, `"exec_command"`) {
		t.Fatalf("unexpected exec out: %s", out)
	}

	out, err = adapter.Execute(context.Background(), "write_stdin", json.RawMessage(`{"chars":"ls"}`), "c2")
	if err != nil {
		t.Fatalf("write_stdin failed: %v", err)
	}
	if !strings.Contains(out, `"write_stdin"`) {
		t.Fatalf("unexpected write out: %s", out)
	}

	out, err = adapter.Execute(context.Background(), "apply_patch", json.RawMessage(`"*** Begin Patch\n*** End Patch\n"`), "c3")
	if err != nil {
		t.Fatalf("apply_patch failed: %v", err)
	}
	if !strings.Contains(out, `"apply_patch"`) {
		t.Fatalf("unexpected patch out: %s", out)
	}
}

func TestPMToolsAdapter_WebToolsDispatch(t *testing.T) {
	adapter := NewPMToolsAdapter(PMToolsAdapterDeps{
		WebTool: func(ctx context.Context, toolName string, input json.RawMessage, callID string) (string, *ToolError) {
			if toolName != "web.search_query" {
				t.Fatalf("unexpected web tool: %s", toolName)
			}
			if !strings.Contains(string(input), `"q":"hello"`) {
				t.Fatalf("unexpected web input: %s", string(input))
			}
			return `{"ok":true,"tool":"web.search_query"}`, nil
		},
	})

	out, err := adapter.Execute(context.Background(), "web.search_query", json.RawMessage(`{"search_query":[{"q":"hello"}]}`), "w1")
	if err != nil {
		t.Fatalf("web.search_query failed: %v", err)
	}
	if !strings.Contains(out, `"web.search_query"`) {
		t.Fatalf("unexpected web out: %s", out)
	}
}

func TestPMToolsAdapter_RequestUserInputGuard(t *testing.T) {
	adapter := NewPMToolsAdapter(PMToolsAdapterDeps{
		RequestUserInput: func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError) {
			return `{"ok":true}`, nil
		},
	})

	_, err := adapter.Execute(context.Background(), "request_user_input", json.RawMessage(`{"questions":[]}`), "r1")
	if err == nil || err.Message != "REQUEST_USER_INPUT_PLAN_MODE_ONLY" {
		t.Fatalf("expected REQUEST_USER_INPUT_PLAN_MODE_ONLY, got %#v", err)
	}

	ctx := WithPMConversationMode(context.Background(), PMConversationModePlan)
	out, err := adapter.Execute(ctx, "request_user_input", json.RawMessage(`{"questions":[]}`), "r2")
	if err != nil {
		t.Fatalf("request_user_input in plan mode failed: %v", err)
	}
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("unexpected request_user_input out: %s", out)
	}
}
