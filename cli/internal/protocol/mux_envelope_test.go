package protocol

import "testing"

func TestMuxEnvelope_RoundTrip(t *testing.T) {
	raw := []byte(`{"id":"req_1","type":"req","op":"tmux.list","payload":{"scope":"all"}}`)
	out, err := WrapMuxEnvelope("conn_a", raw)
	if err != nil {
		t.Fatalf("wrap failed: %v", err)
	}

	connID, inner, err := UnwrapMuxEnvelope(out)
	if err != nil {
		t.Fatalf("unwrap failed: %v", err)
	}
	if connID != "conn_a" {
		t.Fatalf("unexpected conn id: %s", connID)
	}
	if string(inner) != string(raw) {
		t.Fatalf("unexpected payload: %s", string(inner))
	}
}
