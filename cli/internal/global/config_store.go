package global

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const (
	configTOMLFileName = "config.toml"
)

type GlobalDefaults struct {
	SessionProgram string `json:"session_program" toml:"session_program"`
	HelperProgram  string `json:"helper_program" toml:"helper_program"`
}

type GlobalConfig struct {
	LocalPort            int                  `json:"local_port" toml:"local_port"`
	Defaults             GlobalDefaults       `json:"defaults" toml:"defaults"`
	DefaultLaunchProgram string               `json:"default_launch_program,omitempty" toml:"default_launch_program,omitempty"`
	TaskCompletion       TaskCompletionConfig `json:"task_completion" toml:"task_completion"`
}

type TaskCompletionConfig struct {
	NotifyEnabled      bool   `json:"notify_enabled" toml:"notify_enabled"`
	NotifyCommand      string `json:"notify_command" toml:"notify_command"`
	NotifyIdleDuration int    `json:"notify_idle_duration_seconds" toml:"notify_idle_duration_seconds"`
}

type ConfigStore struct {
	dir string
}

func NewConfigStore(dir string) *ConfigStore {
	return &ConfigStore{dir: dir}
}

func (s *ConfigStore) LoadOrInit() (GlobalConfig, error) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return GlobalConfig{}, err
	}

	path := filepath.Join(s.dir, configTOMLFileName)
	if b, err := os.ReadFile(path); err == nil {
		var cfg GlobalConfig
		if err := toml.Unmarshal(b, &cfg); err != nil {
			return GlobalConfig{}, err
		}
		return normalizeConfig(cfg), nil
	} else if !os.IsNotExist(err) {
		return GlobalConfig{}, err
	}

	cfg := normalizeConfig(GlobalConfig{})
	if err := writeTOMLAtomically(path, cfg); err != nil {
		return GlobalConfig{}, err
	}
	return cfg, nil
}

func (s *ConfigStore) Save(cfg GlobalConfig) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	return writeTOMLAtomically(filepath.Join(s.dir, configTOMLFileName), normalizeConfig(cfg))
}

func normalizeConfig(cfg GlobalConfig) GlobalConfig {
	if cfg.LocalPort <= 0 {
		cfg.LocalPort = 4621
	}
	cfg.Defaults = normalizeDefaults(cfg.Defaults, cfg.DefaultLaunchProgram)
	cfg.DefaultLaunchProgram = ""
	cfg.TaskCompletion.NotifyCommand = strings.TrimSpace(cfg.TaskCompletion.NotifyCommand)
	if cfg.TaskCompletion.NotifyIdleDuration < 0 {
		cfg.TaskCompletion.NotifyIdleDuration = 0
	}
	if cfg.TaskCompletion.NotifyCommand == "" {
		cfg.TaskCompletion.NotifyEnabled = false
	}
	return cfg
}

func normalizeDefaults(defaults GlobalDefaults, legacyDefaultLaunchProgram string) GlobalDefaults {
	sessionProgram := strings.ToLower(strings.TrimSpace(defaults.SessionProgram))
	helperProgram := strings.ToLower(strings.TrimSpace(defaults.HelperProgram))
	legacyProgram := strings.ToLower(strings.TrimSpace(legacyDefaultLaunchProgram))

	switch sessionProgram {
	case "shell", "codex", "claude", "cursor":
	default:
		switch legacyProgram {
		case "codex", "claude", "cursor":
			sessionProgram = legacyProgram
		default:
			sessionProgram = "shell"
		}
	}

	switch helperProgram {
	case "codex", "claude", "cursor":
	default:
		helperProgram = "codex"
	}

	return GlobalDefaults{
		SessionProgram: sessionProgram,
		HelperProgram:  helperProgram,
	}
}

func writeTOMLAtomically(path string, v any) error {
	b, err := toml.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeJSONAtomically(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
