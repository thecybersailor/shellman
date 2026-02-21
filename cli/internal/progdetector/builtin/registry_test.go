package builtin_test

import (
	"testing"

	"shellman/cli/internal/progdetector"
	_ "shellman/cli/internal/progdetector/builtin"
)

func TestBuiltinDetectorsRegistered(t *testing.T) {
	for _, id := range []string{"codex", "cursor", "claude", "antigravity"} {
		if _, ok := progdetector.ProgramDetectorRegistry.Get(id); !ok {
			t.Fatalf("expected builtin detector %q registered", id)
		}
	}
}
