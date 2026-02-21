package main

import "testing"

func TestActiveTarget_VersionSemantics(t *testing.T) {
	target := newActiveTarget("")

	if got, v := target.GetWithVersion(); got != "" || v != 0 {
		t.Fatalf("unexpected initial snapshot: target=%q version=%d", got, v)
	}

	target.Set("botworks:2.0")
	if got, v := target.GetWithVersion(); got != "botworks:2.0" || v != 1 {
		t.Fatalf("unexpected after Set new target: target=%q version=%d", got, v)
	}

	target.Set("botworks:2.0")
	if got, v := target.GetWithVersion(); got != "botworks:2.0" || v != 1 {
		t.Fatalf("Set same target should not bump version: target=%q version=%d", got, v)
	}

	target.Select("botworks:2.0")
	if got, v := target.GetWithVersion(); got != "botworks:2.0" || v != 2 {
		t.Fatalf("Select same target should bump version: target=%q version=%d", got, v)
	}

	target.Select("botworks:3.0")
	if got, v := target.GetWithVersion(); got != "botworks:3.0" || v != 3 {
		t.Fatalf("Select new target should bump version: target=%q version=%d", got, v)
	}

	target.ClearIf("botworks:2.0")
	if got, v := target.GetWithVersion(); got != "botworks:3.0" || v != 3 {
		t.Fatalf("ClearIf mismatch target should not change state: target=%q version=%d", got, v)
	}

	target.ClearIf("botworks:3.0")
	if got, v := target.GetWithVersion(); got != "" || v != 4 {
		t.Fatalf("ClearIf exact target should clear and bump version: target=%q version=%d", got, v)
	}
}
