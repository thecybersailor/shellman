package codex

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

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

func (Detector) IsAvailable(context.Context) (bool, error) {
	if _, err := exec.LookPath(programID); err != nil {
		return false, nil
	}
	return true, nil
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
		{Input: submitInputReturn, Delay: submitDelay, TimeoutMs: submitTimeoutMs},
	}, nil
}

func init() {
	progdetector.ProgramDetectorRegistry.MustRegister(New())
}
