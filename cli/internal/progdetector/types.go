package progdetector

import (
	"context"
	"time"
)

// RuntimeState is the minimal runtime snapshot for mode enter/exit detection.
type RuntimeState struct {
	CurrentCommand string
	ViewportText   string
	CursorVisible  bool
}

// PromptStep defines one raw input send step for program-specific input_prompt.
type PromptStep struct {
	Input     string
	Delay     time.Duration
	TimeoutMs int
}

// Detector is implemented by each app-specific detector package.
type Detector interface {
	ProgramID() string
	IsAvailable(ctx context.Context) (bool, error)
	MatchCurrentCommand(currentCommand string) bool
	HasExitedMode(ctx context.Context, state RuntimeState) (bool, error)
	BuildInputPromptSteps(prompt string) ([]PromptStep, error)
}
