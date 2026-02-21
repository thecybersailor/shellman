package global

import (
	"os"
	"path/filepath"
	"strings"
)

// DefaultConfigDir returns ~/.config/muxt.
func DefaultConfigDir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("TERMTEAM_CONFIG_DIR")); override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "muxt"), nil
}
