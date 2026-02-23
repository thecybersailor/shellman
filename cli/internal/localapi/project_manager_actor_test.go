package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"shellman/cli/internal/agentloop"
	"shellman/cli/internal/global"
	"shellman/cli/internal/projectstate"
)

type pmProbeRunner struct {
	calls      int32
	running    int32
	maxRunning int32
	sleep      time.Duration
	reply      string
}

func (r *pmProbeRunner) Run(_ context.Context, _ string) (string, error) {
	atomic.AddInt32(&r.calls, 1)
	cur := atomic.AddInt32(&r.running, 1)
	for {
		prev := atomic.LoadInt32(&r.maxRunning)
		if cur <= prev || atomic.CompareAndSwapInt32(&r.maxRunning, prev, cur) {
			break
		}
	}
	time.Sleep(r.sleep)
	atomic.AddInt32(&r.running, -1)
	return r.reply, nil
}

type pmToolAwareRunner struct {
	allowedTools []string
}

func (r *pmToolAwareRunner) Run(ctx context.Context, _ string) (string, error) {
	names, _ := agentloop.AllowedToolNamesFromContext(ctx)
	r.allowedTools = append([]string{}, names...)
	return "ok", nil
}

type pmStreamWithToolsRunner struct{}

func (r *pmStreamWithToolsRunner) Run(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (r *pmStreamWithToolsRunner) RunStreamWithTools(
	_ context.Context,
	_ string,
	onTextDelta func(string),
	onToolEvent func(map[string]any),
) (string, error) {
	if onTextDelta != nil {
		onTextDelta("stream reply")
	}
	if onToolEvent != nil {
		onToolEvent(map[string]any{
			"type":      "tool-call",
			"call_id":   "pm_call_1",
			"tool_name": "exec_command",
			"state":     "output-available",
			"output":    "{\"ok\":true}",
		})
	}
	return "stream reply", nil
}

func TestProjectManagerActor_SerialBySession(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &pmProbeRunner{sleep: 120 * time.Millisecond, reply: "ok"}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	store := projectstate.NewStore(repo)
	sessionID, err := store.CreatePMSession("p1", "session-a")
	if err != nil {
		t.Fatalf("CreatePMSession failed: %v", err)
	}

	if err := srv.sendProjectManagerLoop(context.Background(), PMAgentLoopEvent{
		SessionID:      sessionID,
		ProjectID:      "p1",
		Source:         "user_input",
		DisplayContent: "hello-1",
		AgentPrompt:    "hello-1",
	}); err != nil {
		t.Fatalf("enqueue first failed: %v", err)
	}
	if err := srv.sendProjectManagerLoop(context.Background(), PMAgentLoopEvent{
		SessionID:      sessionID,
		ProjectID:      "p1",
		Source:         "user_input",
		DisplayContent: "hello-2",
		AgentPrompt:    "hello-2",
	}); err != nil {
		t.Fatalf("enqueue second failed: %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool {
		return atomic.LoadInt32(&runner.calls) >= 2
	})
	if atomic.LoadInt32(&runner.maxRunning) > 1 {
		t.Fatalf("expected serialized run for same session, got maxRunning=%d", runner.maxRunning)
	}
}

func TestProjectManagerActor_RunAndPersistAssistantMessage(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &pmProbeRunner{sleep: 20 * time.Millisecond, reply: "assistant-reply"}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	store := projectstate.NewStore(repo)
	sessionID, err := store.CreatePMSession("p1", "session-a")
	if err != nil {
		t.Fatalf("CreatePMSession failed: %v", err)
	}

	if err := srv.sendProjectManagerLoop(context.Background(), PMAgentLoopEvent{
		SessionID:      sessionID,
		ProjectID:      "p1",
		Source:         "user_input",
		DisplayContent: "hello",
		AgentPrompt:    "hello",
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool {
		items, err := store.ListPMMessages(sessionID, 10)
		if err != nil {
			return false
		}
		if len(items) < 2 {
			return false
		}
		last := items[len(items)-1]
		return last.Role == "assistant" && last.Status == projectstate.StatusCompleted && last.Content == "assistant-reply"
	})
}

func TestProjectManagerActor_AgentLoopUnavailable(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects})
	store := projectstate.NewStore(repo)
	sessionID, err := store.CreatePMSession("p1", "session-a")
	if err != nil {
		t.Fatalf("CreatePMSession failed: %v", err)
	}

	err = srv.sendProjectManagerLoop(context.Background(), PMAgentLoopEvent{
		SessionID:      sessionID,
		ProjectID:      "p1",
		Source:         "user_input",
		DisplayContent: "hello",
		AgentPrompt:    "hello",
	})
	if !errors.Is(err, ErrProjectManagerLoopUnavailable) {
		t.Fatalf("expected ErrProjectManagerLoopUnavailable, got %v", err)
	}
}

func TestProjectManagerActor_InjectsCodexParityAllowedTools(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &pmToolAwareRunner{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	store := projectstate.NewStore(repo)
	sessionID, err := store.CreatePMSession("p1", "session-a")
	if err != nil {
		t.Fatalf("CreatePMSession failed: %v", err)
	}

	if err := srv.sendProjectManagerLoop(context.Background(), PMAgentLoopEvent{
		SessionID:      sessionID,
		ProjectID:      "p1",
		Source:         "user_input",
		DisplayContent: "hello",
		AgentPrompt:    "hello",
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool {
		return len(runner.allowedTools) > 0
	})
	if len(runner.allowedTools) == 0 {
		t.Fatal("expected allowed tools injected")
	}
}

func TestProjectManagerActor_PersistsToolEventsInAssistantMessage(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &pmStreamWithToolsRunner{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	store := projectstate.NewStore(repo)
	sessionID, err := store.CreatePMSession("p1", "session-a")
	if err != nil {
		t.Fatalf("CreatePMSession failed: %v", err)
	}

	if err := srv.sendProjectManagerLoop(context.Background(), PMAgentLoopEvent{
		SessionID:      sessionID,
		ProjectID:      "p1",
		Source:         "user_input",
		DisplayContent: "hello",
		AgentPrompt:    "hello",
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool {
		items, listErr := store.ListPMMessages(sessionID, 10)
		if listErr != nil || len(items) < 2 {
			return false
		}
		last := items[len(items)-1]
		if last.Role != "assistant" || last.Status != projectstate.StatusCompleted {
			return false
		}
		var payload struct {
			Text  string           `json:"text"`
			Tools []map[string]any `json:"tools"`
		}
		if err := json.Unmarshal([]byte(last.Content), &payload); err != nil {
			return false
		}
		return len(payload.Tools) > 0
	})
}
