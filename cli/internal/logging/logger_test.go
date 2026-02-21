package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewLogger_UsesJSONAndLevel(t *testing.T) {
	var buf bytes.Buffer
	lg := NewLogger(Options{Level: "debug", Writer: &buf, Component: "shellman"})
	lg.Debug("boot", "k", "v")

	out := strings.TrimSpace(buf.String())
	if !strings.Contains(out, `"level":"DEBUG"`) {
		t.Fatalf("expected DEBUG level, got %s", out)
	}
	if !strings.Contains(out, `"component":"shellman"`) {
		t.Fatalf("expected component field, got %s", out)
	}
}
