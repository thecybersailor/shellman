package turn

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterClient_Register(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/register" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"turn_uuid":"u1","visit_url":"https://x/t/u1","agent_ws_url":"wss://x/ws/agent/u1"}`))
	}))
	defer srv.Close()

	c := NewRegisterClient(srv.URL)
	got, err := c.Register()
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if got.TurnUUID != "u1" {
		t.Fatalf("unexpected uuid: %s", got.TurnUUID)
	}
}
