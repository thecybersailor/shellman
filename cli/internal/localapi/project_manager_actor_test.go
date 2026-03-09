package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flaboy/agentloop"
	"shellman/cli/internal/agentloopadapter"
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
	mu           sync.Mutex
	allowedTools []string
	scope        agentloopadapter.PMScope
	calls        int
}

func (r *pmToolAwareRunner) Run(ctx context.Context, _ string) (string, error) {
	names, _ := agentloopadapter.AllowedToolNamesFromContext(ctx)
	scope, _ := agentloopadapter.PMScopeFromContext(ctx)
	r.mu.Lock()
	r.allowedTools = append([]string{}, names...)
	r.scope = scope
	r.calls++
	r.mu.Unlock()
	return "ok", nil
}

type pmContextResultRunner struct {
	mu     sync.Mutex
	req    agentloop.ContextBuildRequest
	calls  int
	result agentloop.RunResult
}

func (r *pmContextResultRunner) Run(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (r *pmContextResultRunner) RunWithContextResult(_ context.Context, req agentloop.ContextBuildRequest) (agentloop.RunResult, error) {
	r.mu.Lock()
	r.req = req
	r.calls++
	r.mu.Unlock()
	return r.result, nil
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
			"type":          "tool_input",
			"call_id":       "pm_call_1",
			"response_id":   "pm_resp_1",
			"tool_name":     "exec_command",
			"state":         "input-available",
			"input":         "{\"cmd\":\"ls\"}",
			"input_preview": "ls",
			"input_raw_len": 12,
		})
		onToolEvent(map[string]any{
			"type":        "tool_output",
			"call_id":     "pm_call_1",
			"response_id": "pm_resp_1",
			"tool_name":   "exec_command",
			"state":       "output-available",
			"output":      "{\"ok\":true}",
			"output_len":  11,
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
		runner.mu.Lock()
		defer runner.mu.Unlock()
		return runner.calls > 0
	})
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.allowedTools) == 0 {
		t.Fatal("expected allowed tools injected")
	}
	if strings.TrimSpace(runner.scope.ProjectID) != "p1" {
		t.Fatalf("unexpected pm scope project_id: %q", runner.scope.ProjectID)
	}
	if strings.TrimSpace(runner.scope.SessionID) != sessionID {
		t.Fatalf("unexpected pm scope session_id: %q", runner.scope.SessionID)
	}
	if strings.TrimSpace(runner.scope.Source) != "user_input" {
		t.Fatalf("unexpected pm scope source: %q", runner.scope.Source)
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
		if len(payload.Tools) != 1 {
			return false
		}
		tool := payload.Tools[0]
		return tool["tool_name"] == "exec_command" &&
			tool["state"] == "output-available" &&
			tool["input"] == "{\"cmd\":\"ls\"}" &&
			tool["output"] == "{\"ok\":true}"
	})
}

func TestProjectManagerActor_LogsHistoryMetaAndPromptPreview(t *testing.T) {
	configDir := t.TempDir()
	if err := os.Setenv("SHELLMAN_CONFIG_DIR", configDir); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
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
		AgentPrompt:    "event_type: user_input\nuser_input:\nhello\n\nconversation_history:\n[user#1] first",
		TriggerMeta: map[string]any{
			"history_total":    3,
			"history_included": 2,
			"history_dropped":  1,
			"history_chars":    42,
		},
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
		return last.Role == "assistant" && last.Status == projectstate.StatusCompleted
	})

	b, err := os.ReadFile(filepath.Join(configDir, "logs", "pm-messages.log"))
	if err != nil {
		t.Fatalf("read pm message log failed: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"msg":"pm.message.send.started"`) {
		t.Fatalf("expected started log, got %s", got)
	}
	if !strings.Contains(got, `"history_total":3`) {
		t.Fatalf("expected history_total in logs, got %s", got)
	}
	if !strings.Contains(got, `"history_included":2`) {
		t.Fatalf("expected history_included in logs, got %s", got)
	}
	if !strings.Contains(got, `"history_dropped":1`) {
		t.Fatalf("expected history_dropped in logs, got %s", got)
	}
	if !strings.Contains(got, `"history_chars":42`) {
		t.Fatalf("expected history_chars in logs, got %s", got)
	}
	if !strings.Contains(got, `"agent_prompt_preview":"event_type: user_input`) {
		t.Fatalf("expected agent_prompt_preview in logs, got %s", got)
	}
}

func TestProjectManagerActor_UsesStructuredHistoryAndPersistsResponseID(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}}}
	runner := &pmContextResultRunner{
		result: agentloop.RunResult{
			FinalText:          "pm reply",
			FinalResponseID:    "resp-pm-new-1",
			AppliedHistoryMode: agentloop.HistoryModeProviderState,
		},
	}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	store := projectstate.NewStore(repo)
	sessionID, err := store.CreatePMSession("p1", "session-a")
	if err != nil {
		t.Fatalf("CreatePMSession failed: %v", err)
	}
	if _, err := store.InsertPMMessageWithResponseID(sessionID, "user", "old user", projectstate.StatusCompleted, "", ""); err != nil {
		t.Fatalf("InsertPMMessageWithResponseID user failed: %v", err)
	}
	if _, err := store.InsertPMMessageWithResponseID(sessionID, "assistant", "old assistant", projectstate.StatusCompleted, "", "resp-pm-prev-1"); err != nil {
		t.Fatalf("InsertPMMessageWithResponseID assistant failed: %v", err)
	}

	if err := srv.sendProjectManagerLoop(context.Background(), PMAgentLoopEvent{
		SessionID:      sessionID,
		ProjectID:      "p1",
		Source:         "user_input",
		DisplayContent: "hello",
		AgentPrompt:    "hello",
		HistoryBlock:   "[user#1] old user\n[assistant#2] old assistant",
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool {
		runner.mu.Lock()
		defer runner.mu.Unlock()
		return runner.calls > 0
	})

	runner.mu.Lock()
	req := runner.req
	runner.mu.Unlock()
	if req.HistoryMode != agentloop.HistoryModeHybridAuto {
		t.Fatalf("expected hybrid history mode, got %#v", req)
	}
	if req.PreviousResponseID != "resp-pm-prev-1" {
		t.Fatalf("expected previous_response_id from latest assistant message, got %#v", req)
	}
	if strings.TrimSpace(req.ConversationHistory) == "" {
		t.Fatalf("expected conversation history on context request, got %#v", req)
	}
	if req.Store == nil || !*req.Store {
		t.Fatalf("expected store=true on context request, got %#v", req.Store)
	}

	waitUntil(t, 3*time.Second, func() bool {
		items, err := store.ListPMMessages(sessionID, 10)
		if err != nil || len(items) < 4 {
			return false
		}
		last := items[len(items)-1]
		return last.Role == "assistant" && last.ResponseID == "resp-pm-new-1" && last.Status == projectstate.StatusCompleted
	})
}
