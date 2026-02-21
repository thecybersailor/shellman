package global

import "testing"

func TestDefaultConfigDir_UsesOverride(t *testing.T) {
	t.Setenv("TERMTEAM_CONFIG_DIR", "/tmp/muxt-e2e-config-test")
	got, err := DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir returned error: %v", err)
	}
	if got != "/tmp/muxt-e2e-config-test" {
		t.Fatalf("expected override path, got %q", got)
	}
}

