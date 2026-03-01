package agentloopadapter

import "testing"

func TestParseLegacyToolEvent_ToolInput(t *testing.T) {
	event, ok := ParseLegacyToolEvent(map[string]any{
		"type":          "tool_input",
		"call_id":       " c1 ",
		"response_id":   " r1 ",
		"tool_name":     " exec_command ",
		"state":         " input-available ",
		"input":         " {\"cmd\":\"ls\"} ",
		"input_preview": " ls ",
		"input_raw_len": 12,
	})
	if !ok {
		t.Fatal("expected parse success")
	}
	if event.Type != "tool_input" {
		t.Fatalf("unexpected type: %q", event.Type)
	}
	if event.CallID != "c1" {
		t.Fatalf("unexpected call_id: %q", event.CallID)
	}
	if !event.HasInput || event.Input != "{\"cmd\":\"ls\"}" {
		t.Fatalf("unexpected input: present=%v input=%q", event.HasInput, event.Input)
	}
	if event.InputLen != 12 {
		t.Fatalf("unexpected input len: %d", event.InputLen)
	}
}

func TestParseLegacyToolEvent_SkipDebugAndEmpty(t *testing.T) {
	if _, ok := ParseLegacyToolEvent(nil); ok {
		t.Fatal("expected nil map skipped")
	}
	if _, ok := ParseLegacyToolEvent(map[string]any{"type": "agent-debug"}); ok {
		t.Fatal("expected debug event skipped")
	}
	if _, ok := ParseLegacyToolEvent(map[string]any{"type": "  "}); ok {
		t.Fatal("expected empty type skipped")
	}
}

func TestMergeToolStatePatch_KeepPriorNonEmptyFields(t *testing.T) {
	inputEvent, ok := ParseLegacyToolEvent(map[string]any{
		"type":      "tool_input",
		"call_id":   "call_1",
		"tool_name": "exec_command",
		"state":     "input-available",
		"input":     "{\"cmd\":\"ls\"}",
	})
	if !ok {
		t.Fatal("expected input event parse success")
	}
	outputEvent, ok := ParseLegacyToolEvent(map[string]any{
		"type":       "tool_output",
		"call_id":    "call_1",
		"tool_name":  "exec_command",
		"state":      "output-available",
		"output":     "{\"ok\":true}",
		"output_len": 11,
	})
	if !ok {
		t.Fatal("expected output event parse success")
	}
	current := inputEvent.ToToolStatePatch()
	current = MergeToolStatePatch(current, outputEvent.ToToolStatePatch())
	if got := current["state"]; got != "output-available" {
		t.Fatalf("unexpected merged state: %#v", got)
	}
	if got := current["input"]; got != "{\"cmd\":\"ls\"}" {
		t.Fatalf("expected input kept, got %#v", got)
	}
	if got := current["output"]; got != "{\"ok\":true}" {
		t.Fatalf("expected output merged, got %#v", got)
	}
}
