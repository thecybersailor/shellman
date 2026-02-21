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
