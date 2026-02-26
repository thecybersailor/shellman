package agentloop

import (
	"strings"
	"testing"
)

func TestValidateResponseInputItems_RejectsMultipleSystemMessages(t *testing.T) {
	input := []map[string]any{
		{
			"type":    "message",
			"role":    "system",
			"content": []map[string]any{{"type": "input_text", "text": "s1"}},
		},
		{
			"type":    "message",
			"role":    "system",
			"content": []map[string]any{{"type": "input_text", "text": "s2"}},
		},
	}
	err := ValidateResponseInputInvariants(input)
	if err == nil || !strings.Contains(err.Error(), "at most one system message") {
		t.Fatalf("expected system-count invariant error, got %v", err)
	}
}

func TestValidateResponseInputItems_RejectsSystemNotFirst(t *testing.T) {
	input := []map[string]any{
		{
			"type":    "message",
			"role":    "user",
			"content": []map[string]any{{"type": "input_text", "text": "u"}},
		},
		{
			"type":    "message",
			"role":    "system",
			"content": []map[string]any{{"type": "input_text", "text": "s"}},
		},
	}
	err := ValidateResponseInputInvariants(input)
	if err == nil || !strings.Contains(err.Error(), "system message must be first") {
		t.Fatalf("expected system-order invariant error, got %v", err)
	}
}

func TestValidateResponseInputItems_RejectsFunctionCallOutputWithoutPriorCall(t *testing.T) {
	input := []map[string]any{
		{
			"type":    "message",
			"role":    "user",
			"content": []map[string]any{{"type": "input_text", "text": "u"}},
		},
		{
			"type":    "function_call_output",
			"call_id": "call_missing",
			"output":  "{}",
		},
	}
	err := ValidateResponseInputInvariants(input)
	if err == nil || !strings.Contains(err.Error(), "without prior function_call") {
		t.Fatalf("expected call ordering invariant error, got %v", err)
	}
}

func TestValidateResponseInputItems_RejectsFunctionCallOutputMissingCallID(t *testing.T) {
	input := []map[string]any{
		{
			"type":    "function_call",
			"id":      "fc_1",
			"call_id": "call_1",
			"name":    "task.current.set_flag",
		},
		{
			"type":   "function_call_output",
			"output": "{}",
		},
	}
	err := ValidateResponseInputInvariants(input)
	if err == nil || !strings.Contains(err.Error(), "missing call_id") {
		t.Fatalf("expected missing call_id invariant error, got %v", err)
	}
}

func TestValidateResponseInputItems_AllowsValidInput(t *testing.T) {
	input := []map[string]any{
		{
			"type":    "message",
			"role":    "system",
			"content": []map[string]any{{"type": "input_text", "text": "s"}},
		},
		{
			"type":    "message",
			"role":    "user",
			"content": []map[string]any{{"type": "input_text", "text": "u"}},
		},
		{
			"type":      "function_call",
			"id":        "fc_1",
			"call_id":   "call_1",
			"name":      "task.current.set_flag",
			"arguments": "{}",
		},
		{
			"type":    "function_call_output",
			"call_id": "call_1",
			"output":  "{\"ok\":true}",
		},
	}
	if err := ValidateResponseInputInvariants(input); err != nil {
		t.Fatalf("expected valid input, got %v", err)
	}
}
