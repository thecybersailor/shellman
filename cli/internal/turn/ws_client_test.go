package turn

import "testing"

func TestWSClient_OnText_InvokesHandler(t *testing.T) {
	fake := NewFakeSocket()
	c := NewWSClient(fake)
	c.OnText(func(s string) {
		if s != "hello" {
			t.Fatalf("unexpected: %s", s)
		}
	})
	fake.EmitText("hello")
}
