package protocol

import (
	"encoding/json"
	"testing"
)

func TestMessage_RoundTrip(t *testing.T) {
	raw := []byte(`{"id":"req_1","type":"req","op":"tmux.list","payload":{"scope":"all"}}`)
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if msg.Op != "tmux.list" || msg.Type != "req" {
		t.Fatalf("unexpected message: %+v", msg)
	}
}
