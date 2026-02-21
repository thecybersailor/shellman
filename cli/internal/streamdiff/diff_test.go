package streamdiff

import "testing"

func TestDecideDelta_AppendWhenPrefixMatches(t *testing.T) {
	d := DecideDelta("abc", "abcdef", true)
	if d.Mode != "append" || d.Data != "def" {
		t.Fatalf("unexpected delta: %+v", d)
	}
}

func TestDecideDelta_RepaintWhenPrefixMiss(t *testing.T) {
	d := DecideDelta("abc", "axc", true)
	if d.Mode != "append" {
		t.Fatalf("expected append, got %+v", d)
	}
	if d.Reason != "ansi_repaint" {
		t.Fatalf("expected ansi_repaint reason, got %+v", d)
	}
	if d.Data != "\x1b[0m\x1b[H\x1b[2Jaxc" {
		t.Fatalf("unexpected repaint payload: %+v", d)
	}
}

func TestDecideDelta_RepaintInsteadOfReset_OnFullscreenRedraw(t *testing.T) {
	frames := BuildScenario(ScenarioFullscreenRedraw)
	resets := 0
	for i := 1; i < len(frames); i++ {
		d := DecideDelta(frames[i-1].Text, frames[i].Text, true)
		if d.Mode == "reset" {
			resets++
		}
	}
	if resets != 0 {
		t.Fatalf("expected 0 resets for fullscreen redraw scenario, got %d", resets)
	}
}
