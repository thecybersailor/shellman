package global

import (
	"strings"
)

type AppProgramProvider struct {
	ID                   string `json:"id"`
	DisplayName          string `json:"display_name"`
	Command              string `json:"command"`
	CommitMessageCommand string `json:"commit_message_command,omitempty"`
}

type AppProgramsConfig struct {
	Version   int                  `json:"version"`
	Providers []AppProgramProvider `json:"providers"`
}

type AppProgramsStore struct {
	dir string
}

func NewAppProgramsStore(dir string) *AppProgramsStore {
	return &AppProgramsStore{dir: dir}
}

func (s *AppProgramsStore) LoadOrInit() (AppProgramsConfig, error) {
	return normalizeAppPrograms(defaultAppProgramsConfig()), nil
}

func (s *AppProgramsStore) Save(cfg AppProgramsConfig) error {
	return nil
}

func normalizeAppPrograms(cfg AppProgramsConfig) AppProgramsConfig {
	if cfg.Version <= 0 {
		cfg.Version = 1
	}
	out := AppProgramsConfig{Version: cfg.Version, Providers: make([]AppProgramProvider, 0, len(cfg.Providers))}
	seen := map[string]bool{}
	for _, item := range cfg.Providers {
		id, ok := normalizeProviderID(item.ID)
		if !ok {
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		displayName, command := providerDefaults(id)
		nextDisplayName := strings.TrimSpace(item.DisplayName)
		nextCommand := strings.TrimSpace(item.Command)
		nextCommitMessageCommand := strings.TrimSpace(item.CommitMessageCommand)
		if nextDisplayName == "" {
			nextDisplayName = displayName
		}
		if nextCommand == "" {
			nextCommand = command
		}
		out.Providers = append(out.Providers, AppProgramProvider{
			ID:                   id,
			DisplayName:          nextDisplayName,
			Command:              nextCommand,
			CommitMessageCommand: nextCommitMessageCommand,
		})
	}
	if len(out.Providers) == 0 {
		return defaultAppProgramsConfig()
	}
	return out
}

func defaultAppProgramsConfig() AppProgramsConfig {
	return AppProgramsConfig{
		Version: 1,
		Providers: []AppProgramProvider{
			{ID: "codex", DisplayName: "codex", Command: "codex"},
			{ID: "claude", DisplayName: "Claude", Command: "claude"},
			{ID: "cursor", DisplayName: "Cursor", Command: "cursor"},
		},
	}
}

func normalizeProviderID(raw string) (string, bool) {
	id := strings.ToLower(strings.TrimSpace(raw))
	switch id {
	case "codex", "claude", "cursor":
		return id, true
	default:
		return "", false
	}
}

func providerDefaults(id string) (displayName, command string) {
	switch id {
	case "codex":
		return "codex", "codex"
	case "claude":
		return "Claude", "claude"
	case "cursor":
		return "Cursor", "cursor"
	default:
		return id, id
	}
}
