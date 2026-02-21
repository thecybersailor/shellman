package agentloop

import (
	"context"
	"reflect"
	"testing"
)

func TestWithAllowedToolNames(t *testing.T) {
	ctx := WithAllowedToolNames(context.Background(), []string{" write_stdin ", "", "write_stdin", "exec_command"})
	got, ok := AllowedToolNamesFromContext(ctx)
	if !ok {
		t.Fatal("expected allowlist in context")
	}
	want := []string{"write_stdin", "exec_command"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected allowlist: got=%#v want=%#v", got, want)
	}
}

func TestAllowedToolNamesFromContext_UsesResolver(t *testing.T) {
	calls := 0
	ctx := WithAllowedToolNames(context.Background(), []string{"write_stdin"})
	ctx = WithAllowedToolNamesResolver(ctx, func() []string {
		calls++
		return []string{" task.input_prompt ", "task.input_prompt", "", "write_stdin"}
	})
	got, ok := AllowedToolNamesFromContext(ctx)
	if !ok {
		t.Fatal("expected allowlist from resolver")
	}
	want := []string{"task.input_prompt", "write_stdin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected allowlist from resolver: got=%#v want=%#v", got, want)
	}
	if calls != 1 {
		t.Fatalf("resolver should be called once, got %d", calls)
	}
}
