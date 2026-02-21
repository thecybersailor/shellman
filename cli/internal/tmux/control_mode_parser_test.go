package tmux

import (
	"testing"
	"unsafe"
)

func TestParseControlOutputLine_DecodesEscapedBytes(t *testing.T) {
	line := `%output %1 hello\040world\012`
	ev, ok := ParseControlOutputLine(line)
	if !ok {
		t.Fatal("expected parse ok")
	}
	if ev.PaneID != "%1" {
		t.Fatalf("unexpected pane id: %s", ev.PaneID)
	}
	if ev.Data != "hello world\n" {
		t.Fatalf("unexpected data: %q", ev.Data)
	}
}

func TestParseControlOutputLine_IgnoresBeginBlocks(t *testing.T) {
	if _, ok := ParseControlOutputLine("%begin 123 1 0"); ok {
		t.Fatal("begin block should not parse as output event")
	}
}

func TestDecodeControlEscaped_NoEscapeReusesInputString(t *testing.T) {
	in := "plain-ascii-123"
	got := decodeControlEscaped(in)
	if got != in {
		t.Fatalf("unexpected decode: %q", got)
	}
	if unsafe.StringData(got) != unsafe.StringData(in) {
		t.Fatal("expected fast path to reuse input string storage")
	}
}

func TestDecodeControlEscaped_MixedOctalAndRaw(t *testing.T) {
	in := `A\040B\012C`
	got := decodeControlEscaped(in)
	if got != "A B\nC" {
		t.Fatalf("unexpected decode: %q", got)
	}
}
