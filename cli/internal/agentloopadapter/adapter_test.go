package agentloopadapter

import (
	"context"
	"reflect"
	"testing"

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
