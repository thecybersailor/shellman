package systempicker

import "testing"

func TestBuildPickCommand_Darwin(t *testing.T) {
	cmd, args := buildPickCommand("darwin")
	if cmd != "osascript" {
		t.Fatalf("unexpected cmd: %s", cmd)
	}
	if len(args) == 0 {
		t.Fatal("expected osascript args")
	}
}

func TestBuildPickCommand_Linux(t *testing.T) {
	cmd, _ := buildPickCommand("linux")
	if cmd != "zenity" {
		t.Fatalf("unexpected cmd: %s", cmd)
	}
}
