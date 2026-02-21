package localapi

import (
	"context"
	"strings"
	"testing"
)

func TestDetectPaneCurrentCommand_UsesConfiguredTmuxSocket(t *testing.T) {
	t.Setenv("SHELLMAN_TMUX_SOCKET", "tt_e2e")
	var gotArgs []string
	srv := NewServer(Deps{
		ExecuteCommand: func(_ context.Context, _ string, args ...string) ([]byte, error) {
			gotArgs = append([]string{}, args...)
			return []byte("bash\n"), nil
		},
	})

	cmd := srv.detectPaneCurrentCommand("e2e:0.0")
	if cmd != "bash" {
		t.Fatalf("expected bash, got %q", cmd)
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "-L tt_e2e") {
		t.Fatalf("expected tmux socket flag in args, got %q", joined)
	}
}
