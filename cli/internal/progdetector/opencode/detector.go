package opencode

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"shellman/cli/internal/progdetector"
	"shellman/cli/internal/programadapter"
)

const (
	programID       = "opencode"
	enterTimeoutMs  = 15000
	submitTimeoutMs = 1000
	submitInput     = "\r"
)

type Detector struct{}

func New() Detector {
	return Detector{}
}

func (Detector) ProgramID() string {
	return programID
}

func (Detector) IsAvailable(ctx context.Context) (bool, error) {
	return programadapter.CommandExists(ctx, programID)
}

func (Detector) MatchCurrentCommand(currentCommand string) bool {
	return progdetector.MatchProgramInCommand(currentCommand, programID)
}

func (d Detector) MatchRuntimeState(state progdetector.RuntimeState) bool {
	if d.MatchCurrentCommand(state.CurrentCommand) {
		return true
	}
	binary := normalizeProgramToken(state.CurrentBinary)
	if binary == programID {
		return true
	}
	for _, arg := range state.CurrentArgs {
		if normalizeProgramToken(arg) == programID {
			return true
		}
	}
	return false
}

func (d Detector) HasExitedMode(_ context.Context, state progdetector.RuntimeState) (bool, error) {
	if hasRuntimeSignature(state) {
		return !d.MatchRuntimeState(state), nil
	}
	if d.MatchCurrentCommand(state.CurrentCommand) {
		return false, nil
	}
	return !isNodeCommand(state.CurrentCommand), nil
}

func hasRuntimeSignature(state progdetector.RuntimeState) bool {
	binary := normalizeProgramToken(state.CurrentBinary)
	if binary == "" {
		return false
	}
	if binary == "node" && len(state.CurrentArgs) == 0 {
		return false
	}
	return true
}

func isNodeCommand(command string) bool {
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(parts[0]), "node")
}

func normalizeProgramToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	base := filepath.Base(raw)
	base = strings.TrimSpace(base)
	if base == "" {
		base = raw
	}
	return strings.ToLower(base)
}

func (Detector) BuildInputPromptSteps(prompt string) ([]progdetector.PromptStep, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}
	return []progdetector.PromptStep{
		{Input: prompt, TimeoutMs: enterTimeoutMs},
		{Input: submitInput, TimeoutMs: submitTimeoutMs},
	}, nil
}

func init() {
	progdetector.ProgramDetectorRegistry.MustRegister(New())
}
