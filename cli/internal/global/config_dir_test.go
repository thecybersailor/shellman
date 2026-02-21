package global

import "testing"

func TestDefaultConfigDir_UsesOverride(t *testing.T) {
	t.Setenv("SHELLMAN_CONFIG_DIR", "/tmp/shellman-e2e-config-test")
	got, err := DefaultConfigDir()
	if err != nil {
		t.Fatalf("DefaultConfigDir returned error: %v", err)
	}
	if got != "/tmp/shellman-e2e-config-test" {
		t.Fatalf("expected override path, got %q", got)
	}
}
