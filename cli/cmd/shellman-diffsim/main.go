package main

import (
	"fmt"
	"io"
	"os"

	"shellman/cli/internal/streamdiff"
)

func run(out io.Writer, scenario string) error {
	frames := streamdiff.BuildScenario(streamdiff.Scenario(scenario))
	resets := 0
	appends := 0
	repaints := 0
	for i := 1; i < len(frames); i++ {
		delta := streamdiff.DecideDelta(frames[i-1].Text, frames[i].Text, true)
		switch delta.Mode {
		case "reset":
			resets++
		case "append":
			appends++
			if delta.Reason == "ansi_repaint" {
				repaints++
			}
		}
	}
	_, _ = fmt.Fprintf(out, "scenario=%s frames=%d resets=%d appends=%d repaints=%d\n",
		scenario, len(frames), resets, appends, repaints)
	return nil
}

func main() {
	scenario := "fullscreen_redraw"
	if len(os.Args) > 1 {
		scenario = os.Args[1]
	}
	if err := run(os.Stdout, scenario); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
