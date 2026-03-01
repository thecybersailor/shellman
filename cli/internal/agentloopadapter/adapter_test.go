package agentloopadapter

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/flaboy/agentloop"
	core "github.com/flaboy/agentloop/core"
)

func TestPolicyResolver_TaskModeAllowedTools(t *testing.T) {
	resolver := NewPolicyResolver([]string{"readfile", "exec_command"}, []string{"update_plan"})
	policy, err := resolver.Resolve(context.Background(), core.PolicyRequest[State]{
		State: State{Mode: ModeTask},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !reflect.DeepEqual(policy.AllowedToolNames, []string{"readfile", "exec_command"}) {
		t.Fatalf("unexpected allowed tools: %#v", policy.AllowedToolNames)
	}
}

func TestPolicyResolver_PMModeAllowedTools(t *testing.T) {
	resolver := NewPolicyResolver([]string{"readfile", "exec_command"}, []string{"update_plan", "apply_patch"})
	policy, err := resolver.Resolve(context.Background(), core.PolicyRequest[State]{
		State: State{Mode: ModePM},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if !reflect.DeepEqual(policy.AllowedToolNames, []string{"update_plan", "apply_patch"}) {
		t.Fatalf("unexpected allowed tools: %#v", policy.AllowedToolNames)
	}
}

func TestToLegacyToolEventBridgeContract(t *testing.T) {
	out, ok := ToLegacyToolEvent(agentloop.ToolInputEvent{
		Iteration:    1,
		Timestamp:    time.Now(),
		CallID:       " call-1 ",
		ResponseID:   " resp-1 ",
		ToolName:     " exec_command ",
		Input:        `{"cmd":"ls"}`,
		InputRawLen:  12,
		InputPreview: "ls",
	})
	if !ok {
		t.Fatal("expected legacy bridge conversion success")
	}
	if out["type"] != "tool_input" {
		t.Fatalf("unexpected type: %#v", out["type"])
	}
	if out["call_id"] != "call-1" || out["response_id"] != "resp-1" {
		t.Fatalf("unexpected ids: %#v", out)
	}
	if out["tool_name"] != "exec_command" {
		t.Fatalf("unexpected tool_name: %#v", out["tool_name"])
	}
	if out["state"] != "input-available" {
		t.Fatalf("unexpected state: %#v", out["state"])
	}
}
