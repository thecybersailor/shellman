package agentloopadapter

import (
	"testing"

	"github.com/flaboy/agentloop"
)

func TestToLegacyToolEvent_ToolInput(t *testing.T) {
	in := agentloop.ToolInputEvent{
		CallID:       " call-1 ",
		ResponseID:   " resp-1 ",
		ToolName:     " exec_command ",
		Input:        " {\"cmd\":\"ls\"} ",
		InputPreview: " ls ",
		InputRawLen:  11,
	}
	out, ok := ToLegacyToolEvent(in)
	if !ok {
		t.Fatal("expected conversion success")
	}
	if out["type"] != "tool_input" {
		t.Fatalf("unexpected type: %#v", out["type"])
	}
	if out["call_id"] != "call-1" {
		t.Fatalf("unexpected call_id: %#v", out["call_id"])
	}
	if out["tool_name"] != "exec_command" {
		t.Fatalf("unexpected tool_name: %#v", out["tool_name"])
	}
	if out["state"] != "input-available" {
		t.Fatalf("unexpected state: %#v", out["state"])
	}
}

func TestToLegacyToolEvent_ToolOutput(t *testing.T) {
	in := agentloop.ToolOutputEvent{
		CallID:      " call-1 ",
		ResponseID:  " resp-1 ",
		ToolName:    " exec_command ",
		State:       " output-error ",
		Output:      " failed ",
		OutputLen:   6,
		ErrorString: " ERR ",
	}
	out, ok := ToLegacyToolEvent(in)
	if !ok {
		t.Fatal("expected conversion success")
	}
	if out["type"] != "tool_output" {
		t.Fatalf("unexpected type: %#v", out["type"])
	}
	if out["state"] != "output-error" {
		t.Fatalf("unexpected state: %#v", out["state"])
	}
	if out["error_text"] != "ERR" {
		t.Fatalf("unexpected error_text: %#v", out["error_text"])
	}
}

func TestToLegacyToolEvent_Unsupported(t *testing.T) {
	out, ok := ToLegacyToolEvent(agentloop.ModelRequestEvent{})
	if ok {
		t.Fatal("expected conversion to fail for unsupported event")
	}
	if out != nil {
		t.Fatalf("expected nil output, got %#v", out)
	}
}
