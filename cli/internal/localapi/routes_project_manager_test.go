package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"shellman/cli/internal/global"
	"shellman/cli/internal/projectstate"
)

type pmPromptCaptureRunner struct {
	mu      sync.Mutex
	calls   []string
	replies []string
}

func (r *pmPromptCaptureRunner) Run(_ context.Context, userPrompt string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, userPrompt)
	idx := len(r.calls) - 1
	if idx >= 0 && idx < len(r.replies) {
		return r.replies[idx], nil
	}
	return "ok", nil
}

func (r *pmPromptCaptureRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func (r *pmPromptCaptureRunner) promptAt(index int) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if index < 0 || index >= len(r.calls) {
		return ""
	}
	return r.calls[index]
}

func TestProjectManagerRoutes_CreateAndListSessions(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions", "application/json", bytes.NewBufferString(`{"title":"PM Session 1"}`))
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("create session expected 200, got %d", createResp.StatusCode)
	}
	var created struct {
		OK   bool `json:"ok"`
		Data struct {
			SessionID string `json:"session_id"`
			ProjectID string `json:"project_id"`
			Title     string `json:"title"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}
	if created.Data.SessionID == "" {
		t.Fatal("expected non-empty session_id")
	}
	if created.Data.ProjectID != "p1" {
		t.Fatalf("expected project_id p1, got %q", created.Data.ProjectID)
	}

	listResp, err := http.Get(ts.URL + "/api/v1/projects/p1/pm/sessions")
	if err != nil {
		t.Fatalf("list sessions failed: %v", err)
	}
	defer func() { _ = listResp.Body.Close() }()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list sessions expected 200, got %d", listResp.StatusCode)
	}
	var listed struct {
		OK   bool `json:"ok"`
		Data struct {
			Items []struct {
				SessionID string `json:"session_id"`
				ProjectID string `json:"project_id"`
				Title     string `json:"title"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list response failed: %v", err)
	}
	if len(listed.Data.Items) != 1 {
		t.Fatalf("expected 1 session, got %d", len(listed.Data.Items))
	}
	if listed.Data.Items[0].SessionID != created.Data.SessionID {
		t.Fatalf("expected listed session %q, got %q", created.Data.SessionID, listed.Data.Items[0].SessionID)
	}
}

func TestProjectManagerRoutes_SendMessageQueued(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &fakeTaskMessageRunner{reply: "ok"}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions", "application/json", bytes.NewBufferString(`{"title":"PM Session 1"}`))
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("create session expected 200, got %d", createResp.StatusCode)
	}
	var created struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}

	reqBody := bytes.NewBufferString(`{"content":"hello pm","source":"user_input"}`)
	msgResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions/"+created.Data.SessionID+"/messages", "application/json", reqBody)
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	defer func() { _ = msgResp.Body.Close() }()
	if msgResp.StatusCode != http.StatusOK {
		t.Fatalf("send message expected 200, got %d", msgResp.StatusCode)
	}
	var out struct {
		OK   bool `json:"ok"`
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(msgResp.Body).Decode(&out); err != nil {
		t.Fatalf("decode send message response failed: %v", err)
	}
	if out.Data.Status != "queued" {
		t.Fatalf("expected status queued, got %q", out.Data.Status)
	}
}

func TestProjectManagerRoutes_ListMessages(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &fakeTaskMessageRunner{reply: "assistant-ok"}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions", "application/json", bytes.NewBufferString(`{"title":"PM Session 1"}`))
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	var created struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}

	reqBody := bytes.NewBufferString(`{"content":"hello pm","source":"user_input"}`)
	msgResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions/"+created.Data.SessionID+"/messages", "application/json", reqBody)
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	_ = msgResp.Body.Close()

	deadline := time.Now().Add(2 * time.Second)
	for {
		listResp, err := http.Get(ts.URL + "/api/v1/projects/p1/pm/sessions/" + created.Data.SessionID + "/messages")
		if err != nil {
			t.Fatalf("list messages failed: %v", err)
		}
		var out struct {
			OK   bool `json:"ok"`
			Data struct {
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			} `json:"data"`
		}
		if err := json.NewDecoder(listResp.Body).Decode(&out); err != nil {
			_ = listResp.Body.Close()
			t.Fatalf("decode list messages response failed: %v", err)
		}
		_ = listResp.Body.Close()
		if len(out.Data.Messages) >= 2 {
			last := out.Data.Messages[len(out.Data.Messages)-1]
			if last.Role != "assistant" {
				t.Fatalf("expected last role assistant, got %q", last.Role)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting pm messages, got %d", len(out.Data.Messages))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestProjectManagerRoutes_InvalidProjectID(t *testing.T) {
	projects := &memProjectsStore{projects: []global.ActiveProject{}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/projects/p_not_found/pm/sessions")
	if err != nil {
		t.Fatalf("list sessions failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestProjectManagerRoutes_InvalidSessionID(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &fakeTaskMessageRunner{reply: "ok"}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := bytes.NewBufferString(`{"content":"hello pm","source":"user_input"}`)
	msgResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions/not-exists/messages", "application/json", reqBody)
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	defer func() { _ = msgResp.Body.Close() }()
	if msgResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", msgResp.StatusCode)
	}
}

func TestProjectManagerRoutes_AgentLoopUnavailable(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions", "application/json", bytes.NewBufferString(`{"title":"PM Session 1"}`))
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("create session expected 200, got %d", createResp.StatusCode)
	}
	var created struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}

	reqBody := bytes.NewBufferString(`{"content":"hello pm","source":"user_input"}`)
	msgResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions/"+created.Data.SessionID+"/messages", "application/json", reqBody)
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}
	defer func() { _ = msgResp.Body.Close() }()
	if msgResp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", msgResp.StatusCode)
	}
	var out struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(msgResp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if out.Error.Code != "AGENT_LOOP_UNAVAILABLE" {
		t.Fatalf("expected AGENT_LOOP_UNAVAILABLE, got %q", out.Error.Code)
	}
}

func TestProjectManagerRoutes_SecondMessagePromptIncludesHistory(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &pmPromptCaptureRunner{replies: []string{"ack-first", "ack-second"}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	createResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions", "application/json", bytes.NewBufferString(`{"title":"PM Session 1"}`))
	if err != nil {
		t.Fatalf("create session failed: %v", err)
	}
	defer func() { _ = createResp.Body.Close() }()
	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("create session expected 200, got %d", createResp.StatusCode)
	}
	var created struct {
		Data struct {
			SessionID string `json:"session_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response failed: %v", err)
	}

	firstReqBody := bytes.NewBufferString(`{"content":"first sentence","source":"user_input"}`)
	firstResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions/"+created.Data.SessionID+"/messages", "application/json", firstReqBody)
	if err != nil {
		t.Fatalf("send first message failed: %v", err)
	}
	defer func() { _ = firstResp.Body.Close() }()
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("first message expected 200, got %d", firstResp.StatusCode)
	}

	waitUntil(t, 3*time.Second, func() bool {
		if runner.callCount() < 1 {
			return false
		}
		store := projectstate.NewStore(repo)
		items, listErr := store.ListPMMessages(created.Data.SessionID, 10)
		if listErr != nil || len(items) < 2 {
			return false
		}
		last := items[len(items)-1]
		return last.Role == "assistant" && last.Status == projectstate.StatusCompleted
	})

	secondReqBody := bytes.NewBufferString(`{"content":"second sentence","source":"user_input"}`)
	secondResp, err := http.Post(ts.URL+"/api/v1/projects/p1/pm/sessions/"+created.Data.SessionID+"/messages", "application/json", secondReqBody)
	if err != nil {
		t.Fatalf("send second message failed: %v", err)
	}
	defer func() { _ = secondResp.Body.Close() }()
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("second message expected 200, got %d", secondResp.StatusCode)
	}

	waitUntil(t, 3*time.Second, func() bool {
		return runner.callCount() >= 2
	})
	secondPrompt := runner.promptAt(1)
	if !strings.Contains(secondPrompt, "conversation_history:") {
		t.Fatalf("expected second prompt contains conversation_history, got %q", secondPrompt)
	}
	if !strings.Contains(secondPrompt, "first sentence") {
		t.Fatalf("expected second prompt contains first user content, got %q", secondPrompt)
	}
	if !strings.Contains(secondPrompt, "ack-first") {
		t.Fatalf("expected second prompt contains first assistant content, got %q", secondPrompt)
	}
}
