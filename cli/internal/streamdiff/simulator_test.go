package streamdiff

import "testing"

func TestBuildFullscreenRedrawScenario(t *testing.T) {
	frames := BuildScenario(ScenarioFullscreenRedraw)
	if len(frames) < 6 {
		t.Fatalf("expected >= 6 frames, got %d", len(frames))
	}
	if frames[0].Target == "" {
		t.Fatal("target should not be empty")
	}

	changedMiddle := false
	for i := 1; i < len(frames); i++ {
		if len(frames[i].Text) == len(frames[i-1].Text) && frames[i].Text != frames[i-1].Text {
			changedMiddle = true
			break
		}
	}
	if !changedMiddle {
		t.Fatal("scenario should contain same-length but content-changed frames")
	}
}
