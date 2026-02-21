package main

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeTermSnapshot_PreservesTrailingBlankLinesForCursorAlignment(t *testing.T) {
	in := "bash-5.3$ \n\n\n"
	got := normalizeTermSnapshot(in)
	want := in
	if got != want {
		t.Fatalf("unexpected normalized text: %q", got)
	}
}

func TestNormalizeTermSnapshot_PreservesInnerNewlines(t *testing.T) {
	in := "line1\nline2\nline3\n"
	got := normalizeTermSnapshot(in)
	want := in
	if got != want {
		t.Fatalf("unexpected normalized text: %q", got)
	}
}

func TestNormalizeTermSnapshot_PreservesTrailingCRLF(t *testing.T) {
	in := "line1\r\nline2\r\n"
	got := normalizeTermSnapshot(in)
	want := in
	if got != want {
		t.Fatalf("unexpected normalized text: %q", got)
	}
}

func TestChunkTermData_SplitsOversizedAndKeepsBytes(t *testing.T) {
	oversized := ""
	for i := 0; i < maxTermFrameDataBytes*2+17; i++ {
		oversized += "x"
	}
	frames := chunkTermData("reset", oversized)
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	if frames[0].Mode != "reset" {
		t.Fatalf("first frame should keep reset mode, got %q", frames[0].Mode)
	}
	if frames[1].Mode != "append" || frames[2].Mode != "append" {
		t.Fatalf("remaining frames should use append mode: %#v", frames)
	}
	joined := frames[0].Data + frames[1].Data + frames[2].Data
	if joined != oversized {
		t.Fatal("chunked output must keep full payload without truncation")
	}
	for i, frame := range frames {
		if len(frame.Data) > maxTermFrameDataBytes {
			t.Fatalf("frame[%d] exceeds max size: %d", i, len(frame.Data))
		}
	}
}

func TestChunkTermData_DoesNotSplitUTF8Rune(t *testing.T) {
	prefix := strings.Repeat("x", maxTermFrameDataBytes-1)
	payload := prefix + "你"
	frames := chunkTermData("reset", payload)
	if len(frames) < 2 {
		t.Fatalf("expected split frames, got %d", len(frames))
	}
	if !utf8.ValidString(frames[0].Data) {
		t.Fatalf("frame0 contains broken utf8: %q", frames[0].Data[len(frames[0].Data)-8:])
	}
	if !utf8.ValidString(frames[1].Data) {
		t.Fatalf("frame1 contains broken utf8: %q", frames[1].Data)
	}
	if frames[0].Data+frames[1].Data != payload {
		t.Fatal("payload changed after chunking")
	}
}
