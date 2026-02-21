package localapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"shellman/cli/internal/fsbrowser"
	"shellman/cli/internal/historydb"
	"shellman/cli/internal/projectstate"
)

func TestFSRoutes_ListAndResolve(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "child"), 0o755); err != nil {
		t.Fatalf("mkdir child failed: %v", err)
	}

	svc := fsbrowser.NewService()
	db, err := projectstate.GlobalDB()
	if err != nil {
		t.Fatalf("GlobalDB failed: %v", err)
	}
	hist, err := historydb.NewStore(db)
	if err != nil {
		t.Fatalf("new history store failed: %v", err)
	}
	defer func() { _ = hist.Close() }()

	srv := NewServer(Deps{ConfigStore: &fakeConfigStore{}, ProjectsStore: &fakeProjectsStore{}, FSBrowser: svc, DirHistory: hist})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	res1, err := http.Get(ts.URL + "/api/v1/fs/list?path=" + url.QueryEscape(root))
	if err != nil {
		t.Fatalf("list request failed: %v", err)
	}
	defer func() { _ = res1.Body.Close() }()
	if res1.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res1.StatusCode)
	}

	reqBody := bytes.NewBufferString(`{"path":"` + root + `"}`)
	res2, err := http.Post(ts.URL+"/api/v1/fs/resolve", "application/json", reqBody)
	if err != nil {
		t.Fatalf("resolve request failed: %v", err)
	}
	defer func() { _ = res2.Body.Close() }()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res2.StatusCode)
	}
}

func TestServer_FSHistory_CRUD(t *testing.T) {
	db, err := projectstate.GlobalDB()
	if err != nil {
		t.Fatalf("GlobalDB failed: %v", err)
	}
	hist, err := historydb.NewStore(db)
	if err != nil {
		t.Fatalf("new history store failed: %v", err)
	}
	defer func() { _ = hist.Close() }()

	srv := NewServer(Deps{
		ConfigStore:   &fakeConfigStore{},
		ProjectsStore: &fakeProjectsStore{},
		FSBrowser:     fsbrowser.NewService(),
		DirHistory:    hist,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	postRes, err := http.Post(ts.URL+"/api/v1/fs/history", "application/json", bytes.NewBufferString(`{"path":"/tmp/a"}`))
	if err != nil {
		t.Fatalf("post history failed: %v", err)
	}
	if postRes.StatusCode != http.StatusOK {
		t.Fatalf("expected post 200, got %d", postRes.StatusCode)
	}

	getRes, err := http.Get(ts.URL + "/api/v1/fs/history")
	if err != nil {
		t.Fatalf("get history failed: %v", err)
	}
	if getRes.StatusCode != http.StatusOK {
		t.Fatalf("expected get 200, got %d", getRes.StatusCode)
	}
	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Items []struct {
				Path string `json:"path"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getRes.Body).Decode(&body); err != nil {
		t.Fatalf("decode history failed: %v", err)
	}
	if len(body.Data.Items) == 0 || body.Data.Items[0].Path != "/tmp/a" {
		t.Fatalf("unexpected history items: %+v", body.Data.Items)
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/fs/history", nil)
	delRes, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("delete history failed: %v", err)
	}
	if delRes.StatusCode != http.StatusOK {
		t.Fatalf("expected delete 200, got %d", delRes.StatusCode)
	}

	getRes2, err := http.Get(ts.URL + "/api/v1/fs/history")
	if err != nil {
		t.Fatalf("get history after delete failed: %v", err)
	}
	var body2 struct {
		OK   bool `json:"ok"`
		Data struct {
			Items []any `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getRes2.Body).Decode(&body2); err != nil {
		t.Fatalf("decode history2 failed: %v", err)
	}
	if len(body2.Data.Items) != 0 {
		t.Fatalf("expected empty history, got %+v", body2.Data.Items)
	}
}
