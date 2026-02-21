package agentloop

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeTool struct {
	name string
}

func (f fakeTool) Name() string { return f.name }

func (f fakeTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type: "function",
		Name: f.name,
	}
}

func (f fakeTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError) {
	return "ok", nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewToolRegistry()
	if err := r.Register(fakeTool{name: "gateway_http"}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	tool, ok := r.Get("gateway_http")
	if !ok {
		t.Fatal("tool not found")
	}
	if tool.Name() != "gateway_http" {
		t.Fatalf("unexpected tool: %s", tool.Name())
	}
}

func TestRegistry_RejectDuplicateName(t *testing.T) {
	r := NewToolRegistry()
	if err := r.Register(fakeTool{name: "gateway_http"}); err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if err := r.Register(fakeTool{name: "gateway_http"}); err == nil {
		t.Fatal("expected duplicate register to fail")
	}
}

func TestRegistry_SpecsByNames(t *testing.T) {
	r := NewToolRegistry()
	if err := r.Register(fakeTool{name: "b_tool"}); err != nil {
		t.Fatalf("register b_tool failed: %v", err)
	}
	if err := r.Register(fakeTool{name: "a_tool"}); err != nil {
		t.Fatalf("register a_tool failed: %v", err)
	}
	specs := r.SpecsByNames([]string{"b_tool", "missing", "a_tool"})
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].Name != "a_tool" || specs[1].Name != "b_tool" {
		t.Fatalf("unexpected order/spec names: %#v", specs)
	}
}

func TestToolRegistry_ExecuteMissingToolReturnsToolError(t *testing.T) {
	r := NewToolRegistry()
	_, err := r.Execute(context.Background(), "missing", []byte(`{}`), "call_1")
	if err == nil {
		t.Fatal("expected tool error")
	}
	if err.Message != "TOOL_NOT_FOUND" {
		t.Fatalf("unexpected error: %#v", err)
	}
	if err.Suggest == "" {
		t.Fatalf("suggest must not be empty: %#v", err)
	}
}
