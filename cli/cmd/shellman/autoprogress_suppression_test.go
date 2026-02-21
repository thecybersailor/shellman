package main

import "testing"

func TestAutoProgressSuppression_RegisterAndConsumeOnce(t *testing.T) {
	resetAutoProgressSuppressionForTest()
	registerAutoProgressSuppression("e2e:0.0", "hash_1", true)
	if !consumeAutoProgressSuppression("e2e:0.0", "hash_1") {
		t.Fatal("expected first consume matched suppression entry")
	}
	if consumeAutoProgressSuppression("e2e:0.0", "hash_1") {
		t.Fatal("expected suppression entry consumed once")
	}
}

func TestAutoProgressSuppression_IgnoresUnchangedHash(t *testing.T) {
	resetAutoProgressSuppressionForTest()
	registerAutoProgressSuppression("e2e:0.0", "hash_1", false)
	if consumeAutoProgressSuppression("e2e:0.0", "hash_1") {
		t.Fatal("expected no suppression entry when hash_changed=false")
	}
}
