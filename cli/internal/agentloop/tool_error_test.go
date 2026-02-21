package agentloop

import "testing"

func TestToolError_JSONShapeAlwaysContainsErrorAndSuggest(t *testing.T) {
	te := NewToolError("INVALID_INPUT", "请检查输入参数并重试")
	raw := mustMarshalToolError(te)
	if raw != `{"error":"INVALID_INPUT","suggest":"请检查输入参数并重试"}` {
		t.Fatalf("unexpected json: %s", raw)
	}
}

func TestToolError_DefaultSuggestWhenEmpty(t *testing.T) {
	te := NewToolError("TASK_CONTEXT_MISSING", "")
	raw := mustMarshalToolError(te)
	if raw != `{"error":"TASK_CONTEXT_MISSING","suggest":"NO_SUGGESTION"}` {
		t.Fatalf("unexpected json: %s", raw)
	}
}
