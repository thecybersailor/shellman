package codex

import (
	"context"
	"errors"
	"strings"
	"time"

	"shellman/cli/internal/programadapter"
	"shellman/cli/internal/progdetector"
)

const (
	programID         = "codex"
	enterTimeoutMs    = 15000
	submitTimeoutMs   = 1000
	submitDelay       = 50 * time.Millisecond
	submitInputReturn = "\r"
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

func (d Detector) HasExitedMode(_ context.Context, state progdetector.RuntimeState) (bool, error) {
	return !isNodeCommand(state.CurrentCommand), nil
}

func isNodeCommand(command string) bool {
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(parts[0]), "node")
}

func (Detector) BuildInputPromptSteps(prompt string) ([]progdetector.PromptStep, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, errors.New("prompt is required")
	}
	return []progdetector.PromptStep{
		{Input: prompt, TimeoutMs: enterTimeoutMs},
		{Input: submitInputReturn, Delay: submitDelay, TimeoutMs: submitTimeoutMs},
	}, nil
}

func init() {
	progdetector.ProgramDetectorRegistry.MustRegister(New())
}
