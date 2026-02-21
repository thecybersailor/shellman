package localapi

import (
	"errors"
	"strings"

	"shellman/cli/internal/projectstate"
)

var errInvalidSidecarMode = errors.New("sidecar_mode must be one of advisor|observer|autopilot")

func normalizeSidecarMode(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case projectstate.SidecarModeAdvisor, "":
		return projectstate.SidecarModeAdvisor
	case projectstate.SidecarModeObserver:
		return projectstate.SidecarModeObserver
	case projectstate.SidecarModeAutopilot:
		return projectstate.SidecarModeAutopilot
	default:
		return ""
	}
}

func validSidecarMode(mode string) bool {
	return normalizeSidecarMode(mode) != ""
}
