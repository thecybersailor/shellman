package main

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func containsAll(s string, needles []string) bool {
	for _, n := range needles {
		if !strings.Contains(s, n) {
			return false
		}
	}
	return true
}

func TestRun_PrintsScenarioSummary(t *testing.T) {
	var out bytes.Buffer
	err := run(&out, "fullscreen_redraw")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	s := out.String()
	if s == "" || !containsAll(s, []string{"scenario=fullscreen_redraw", "frames=", "resets="}) {
		t.Fatalf("unexpected output: %q", s)
	}
}

func TestRun_PrintsNonZeroRepaintCount(t *testing.T) {
	var out bytes.Buffer
	err := run(&out, "fullscreen_redraw")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	s := out.String()
	reResets := regexp.MustCompile(`resets=(\d+)`)
	reRepaints := regexp.MustCompile(`repaints=(\d+)`)
	mResets := reResets.FindStringSubmatch(s)
	mRepaints := reRepaints.FindStringSubmatch(s)
	if len(mResets) != 2 || len(mRepaints) != 2 {
		t.Fatalf("failed to parse counters from output: %q", s)
	}
	resets, _ := strconv.Atoi(mResets[1])
	repaints, _ := strconv.Atoi(mRepaints[1])
	if resets != 0 {
		t.Fatalf("expected resets=0, got %d output=%q", resets, s)
	}
	if repaints <= 0 {
		t.Fatalf("expected repaints>0, got %d output=%q", repaints, s)
	}
}
