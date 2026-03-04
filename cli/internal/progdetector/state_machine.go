package progdetector

import (
	"context"
	"strings"
)

// ResolveActiveAdapter applies detector enter/exit rules for one command sample.
// If current active adapter has not exited, keep it; otherwise clear and try enter by command.
func ResolveActiveAdapter(activeAdapterID, currentCommand string) string {
	activeAdapterID = strings.TrimSpace(activeAdapterID)
	currentCommand = strings.TrimSpace(currentCommand)

	if activeAdapterID != "" {
		if detector, ok := ProgramDetectorRegistry.Get(activeAdapterID); ok && detector != nil {
			exited, err := detector.HasExitedMode(context.Background(), RuntimeState{
				CurrentCommand: currentCommand,
			})
			if err == nil && !exited {
				return activeAdapterID
			}
		}
		activeAdapterID = ""
	}

	if detector, ok := ProgramDetectorRegistry.DetectByCurrentCommand(currentCommand); ok && detector != nil {
		return strings.TrimSpace(detector.ProgramID())
	}
	return ""
}
