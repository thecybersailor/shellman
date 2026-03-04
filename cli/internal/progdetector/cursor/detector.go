package cursor

import (
	"context"
	"errors"
	"strings"

	"shellman/cli/internal/programadapter"
	"shellman/cli/internal/progdetector"
)

const (
	programID       = "cursor"
	enterTimeoutMs  = 15000
	submitTimeoutMs = 1000
	submitInput     = "\n"
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
	return !d.MatchCurrentCommand(state.CurrentCommand), nil
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
