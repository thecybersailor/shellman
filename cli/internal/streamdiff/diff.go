package streamdiff

import "strings"

type Delta struct {
	Mode   string
	Data   string
	Reason string
}

func DecideDelta(prev, curr string, snapshotChanged bool) Delta {
	if !snapshotChanged {
		return Delta{Mode: "append", Data: "", Reason: "cursor_only"}
	}
	if strings.HasPrefix(curr, prev) {
		return Delta{Mode: "append", Data: curr[len(prev):], Reason: "prefix_append"}
	}
	return Delta{Mode: "append", Data: "\x1b[0m\x1b[H\x1b[2J" + curr, Reason: "ansi_repaint"}
}
