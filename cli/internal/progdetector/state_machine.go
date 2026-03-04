package progdetector

import (
	"context"
	"strings"
)

// ResolveActiveAdapter applies detector enter/exit rules for one command sample.
// If current active adapter has not exited, keep it; otherwise clear and try enter by command.
func ResolveActiveAdapter(activeAdapterID, currentCommand string) string {
	return ResolveActiveAdapterByState(activeAdapterID, RuntimeState{
		CurrentCommand: currentCommand,
	})
}

// ResolveActiveAdapterByState applies detector enter/exit rules for one runtime state sample.
// If current active adapter has not exited, keep it; otherwise clear and try enter by runtime state.
func ResolveActiveAdapterByState(activeAdapterID string, state RuntimeState) string {
	activeAdapterID = strings.TrimSpace(activeAdapterID)
	state = normalizeRuntimeState(state)

	if activeAdapterID != "" {
		if detector, ok := ProgramDetectorRegistry.Get(activeAdapterID); ok && detector != nil {
			exited, err := detector.HasExitedMode(context.Background(), state)
			if err == nil && !exited {
				return activeAdapterID
			}
		}
	}

	if detector, ok := ProgramDetectorRegistry.DetectByState(state); ok && detector != nil {
		return strings.TrimSpace(detector.ProgramID())
	}
	return ""
}

func normalizeRuntimeState(state RuntimeState) RuntimeState {
	state.CurrentCommand = strings.TrimSpace(state.CurrentCommand)
	state.CurrentBinary = strings.TrimSpace(state.CurrentBinary)
	if len(state.CurrentArgs) == 0 {
		return state
	}
	next := make([]string, 0, len(state.CurrentArgs))
	for _, arg := range state.CurrentArgs {
		item := strings.TrimSpace(arg)
		if item == "" {
			continue
		}
		next = append(next, item)
	}
	state.CurrentArgs = next
	return state
}
