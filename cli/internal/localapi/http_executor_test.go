package localapi

import (
	"strings"
	"testing"
)

func TestHTTPExecutor_ExecutesLocalAPI(t *testing.T) {
	srv := NewServer(Deps{ConfigStore: &fakeConfigStore{}, ProjectsStore: &fakeProjectsStore{}})
	exec := NewHTTPExecutor(srv.Handler())

	status, _, body, err := exec.Execute("GET", "/api/v1/system/capabilities", nil, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected 200, got %d", status)
	}
	if !strings.Contains(body, "directory_picker") {
		t.Fatalf("unexpected body: %s", body)
	}
}
