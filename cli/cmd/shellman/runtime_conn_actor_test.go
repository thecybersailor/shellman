package main

import "testing"

func TestConnActor_SelectAndWatch_KeepRecentLimit(t *testing.T) {
	conn := NewConnActor("conn_1")
	if conn == nil {
		t.Fatal("expected conn actor")
	}

	targets := []string{"e2e:0.1", "e2e:0.2", "e2e:0.3", "e2e:0.4", "e2e:0.5", "e2e:0.6"}
	evicted := ""
	for _, target := range targets {
		evicted = conn.SelectAndWatch(target, 5)
	}

	if evicted != "e2e:0.1" {
		t.Fatalf("expected evicted e2e:0.1, got %q", evicted)
	}
	if got := conn.Selected(); got != "e2e:0.6" {
		t.Fatalf("expected selected e2e:0.6, got %q", got)
	}
	got := conn.WatchedTargets()
	want := []string{"e2e:0.2", "e2e:0.3", "e2e:0.4", "e2e:0.5", "e2e:0.6"}
	if len(got) != len(want) {
		t.Fatalf("unexpected watched length, want=%d got=%d watched=%v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected watched[%d], want=%q got=%q all=%v", i, want[i], got[i], got)
		}
	}
}

