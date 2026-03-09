package localapi

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flaboy/agentloop"
	"shellman/cli/internal/agentloopadapter"
	"shellman/cli/internal/global"
	"shellman/cli/internal/projectstate"
)

func TestTaskAgentLoopSupervisor_SerializesEventsPerTask(t *testing.T) {
	var (
		mu      sync.Mutex
		started []string
	)
	firstRelease := make(chan struct{})
	secondRelease := make(chan struct{})
	supervisor := newTaskAgentLoopSupervisor(nil, func(_ context.Context, evt TaskAgentLoopEvent) error {
		mu.Lock()
		started = append(started, strings.TrimSpace(evt.DisplayContent))
		mu.Unlock()
		switch strings.TrimSpace(evt.DisplayContent) {
		case "first":
			<-firstRelease
		case "second":
			<-secondRelease
		}
		return nil
	})

	if err := supervisor.Enqueue(context.Background(), TaskAgentLoopEvent{
		TaskID:         "t1",
		DisplayContent: "first",
		AgentPrompt:    "first",
	}); err != nil {
		t.Fatalf("enqueue first failed: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		count := len(started)
		mu.Unlock()
		if count >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("first event did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := supervisor.Enqueue(context.Background(), TaskAgentLoopEvent{
		TaskID:         "t1",
		DisplayContent: "second",
		AgentPrompt:    "second",
	}); err != nil {
		t.Fatalf("enqueue second failed: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	mu.Lock()
	countBeforeRelease := len(started)
	mu.Unlock()
	if countBeforeRelease != 1 {
		t.Fatalf("expected second event not started before first release, got started=%d", countBeforeRelease)
	}
	close(firstRelease)

	deadline = time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		count := len(started)
		second := ""
		if len(started) > 1 {
			second = started[1]
		}
		mu.Unlock()
		if count >= 2 {
			if second != "second" {
				t.Fatalf("expected second event content=second, got %q", second)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second event did not start after first release")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(secondRelease)
}

func TestTaskAgentLoopSupervisor_AllowsParallelAcrossTasks(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	supervisor := newTaskAgentLoopSupervisor(nil, func(_ context.Context, evt TaskAgentLoopEvent) error {
		started <- evt.TaskID
		<-release
		return nil
	})

	if err := supervisor.Enqueue(context.Background(), TaskAgentLoopEvent{TaskID: "task_a", DisplayContent: "a", AgentPrompt: "a"}); err != nil {
		t.Fatalf("enqueue task_a failed: %v", err)
	}
	if err := supervisor.Enqueue(context.Background(), TaskAgentLoopEvent{TaskID: "task_b", DisplayContent: "b", AgentPrompt: "b"}); err != nil {
		t.Fatalf("enqueue task_b failed: %v", err)
	}

	timeout := time.After(2 * time.Second)
	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case taskID := <-started:
			got[taskID] = true
		case <-timeout:
			t.Fatalf("expected both tasks started in parallel, got %#v", got)
		}
	}
	close(release)
}

func TestSendTaskAgentLoop_ReturnsUnavailableWhenRunnerMissing(t *testing.T) {
	srv := NewServer(Deps{
		ConfigStore:   &staticConfigStore{},
		ProjectsStore: &memProjectsStore{},
	})
	err := srv.sendTaskAgentLoop(context.Background(), TaskAgentLoopEvent{
		TaskID:         "t1",
		DisplayContent: "hello",
		AgentPrompt:    "hello",
	})
	if !errors.Is(err, ErrTaskAgentLoopUnavailable) {
		t.Fatalf("expected ErrTaskAgentLoopUnavailable, got %v", err)
	}
}

func TestTaskAgentLoopSupervisor_SidecarModeDefaultsAdvisor(t *testing.T) {
	supervisor := newTaskAgentLoopSupervisor(nil, nil)
	if got := supervisor.GetSidecarMode("missing-task"); got != "advisor" {
		t.Fatalf("expected missing task sidecar_mode=advisor, got %q", got)
	}
}

func TestTaskAgentLoopSupervisor_SetAndGetSidecarMode(t *testing.T) {
	supervisor := newTaskAgentLoopSupervisor(nil, nil)
	if err := supervisor.SetSidecarMode("task-1", "observer"); err != nil {
		t.Fatalf("set sidecar_mode observer failed: %v", err)
	}
	if got := supervisor.GetSidecarMode("task-1"); got != "observer" {
		t.Fatalf("expected sidecar_mode=observer, got %q", got)
	}
	if err := supervisor.SetSidecarMode("task-1", "autopilot"); err != nil {
		t.Fatalf("set sidecar_mode autopilot failed: %v", err)
	}
	if got := supervisor.GetSidecarMode("task-1"); got != "autopilot" {
		t.Fatalf("expected sidecar_mode=autopilot, got %q", got)
	}
}

type taskScopeAwareRunner struct {
	mu           sync.Mutex
	allowedTools []string
	scope        agentloopadapter.TaskScope
	calls        int
}

func (r *taskScopeAwareRunner) Run(ctx context.Context, _ string) (string, error) {
	names, _ := agentloopadapter.AllowedToolNamesFromContext(ctx)
	scope, _ := agentloopadapter.TaskScopeFromContext(ctx)
	r.mu.Lock()
	r.allowedTools = append([]string{}, names...)
	r.scope = scope
	r.calls++
	r.mu.Unlock()
	return "ok", nil
}

type taskContextResultRunner struct {
	mu     sync.Mutex
	req    agentloop.ContextBuildRequest
	calls  int
	result agentloop.RunResult
}

func (r *taskContextResultRunner) Run(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (r *taskContextResultRunner) RunWithContextResult(_ context.Context, req agentloop.ContextBuildRequest) (agentloop.RunResult, error) {
	r.mu.Lock()
	r.req = req
	r.calls++
	r.mu.Unlock()
	return r.result, nil
}

func TestTaskAgentActor_InjectsAdapterScopeAndAllowedTools(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{
		projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}},
	}
	runner := &taskScopeAwareRunner{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	taskID, err := srv.createTask("p1", "", "root")
	if err != nil {
		t.Fatalf("createTask failed: %v", err)
	}
	if err := srv.sendTaskAgentLoop(context.Background(), TaskAgentLoopEvent{
		TaskID:         taskID,
		ProjectID:      "p1",
		Source:         "user_input",
		DisplayContent: "hello",
		AgentPrompt:    "hello",
	}); err != nil {
		t.Fatalf("sendTaskAgentLoop failed: %v", err)
	}
	waitUntil(t, 3*time.Second, func() bool {
		runner.mu.Lock()
		defer runner.mu.Unlock()
		return runner.calls > 0
	})
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if strings.TrimSpace(runner.scope.TaskID) != taskID {
		t.Fatalf("unexpected task scope task_id: %q", runner.scope.TaskID)
	}
	if strings.TrimSpace(runner.scope.ProjectID) != "p1" {
		t.Fatalf("unexpected task scope project_id: %q", runner.scope.ProjectID)
	}
	if strings.TrimSpace(runner.scope.Source) != "user_input" {
		t.Fatalf("unexpected task scope source: %q", runner.scope.Source)
	}
	if len(runner.allowedTools) == 0 {
		t.Fatal("expected allowed tools injected from adapter context")
	}
}

func TestTaskAgentActor_UsesStructuredHistoryAndPersistsResponseID(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{
		projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}},
	}
	runner := &taskContextResultRunner{
		result: agentloop.RunResult{
			FinalText:          "assistant reply",
			FinalResponseID:    "resp-task-new-1",
			AppliedHistoryMode: agentloop.HistoryModeProviderState,
		},
	}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	taskID, err := srv.createTask("p1", "", "root")
	if err != nil {
		t.Fatalf("createTask failed: %v", err)
	}
	store := projectstate.NewStore(repo)
	if _, err := store.InsertTaskMessageWithResponseID(taskID, "user", "old user", projectstate.StatusCompleted, "", ""); err != nil {
		t.Fatalf("insert old user failed: %v", err)
	}
	if _, err := store.InsertTaskMessageWithResponseID(taskID, "assistant", "old assistant", projectstate.StatusCompleted, "", "resp-task-prev-1"); err != nil {
		t.Fatalf("insert old assistant failed: %v", err)
	}

	if err := srv.sendTaskAgentLoop(context.Background(), TaskAgentLoopEvent{
		TaskID:         taskID,
		ProjectID:      "p1",
		Source:         "user_input",
		DisplayContent: "hello",
		AgentPrompt:    "hello",
		HistoryBlock:   "[user#1] old user\n[assistant#2] old assistant",
		SessionConfig: &TaskAgentSessionConfig{
			ResponsesStore:      true,
			DisableStoreContext: false,
		},
	}); err != nil {
		t.Fatalf("sendTaskAgentLoop failed: %v", err)
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
	if req.PreviousResponseID != "resp-task-prev-1" {
		t.Fatalf("expected previous_response_id from latest assistant message, got %#v", req)
	}
	if strings.TrimSpace(req.ConversationHistory) == "" {
		t.Fatalf("expected conversation history on context request, got %#v", req)
	}
	if req.Store == nil || !*req.Store {
		t.Fatalf("expected store=true on context request, got %#v", req.Store)
	}

	waitUntil(t, 3*time.Second, func() bool {
		items, err := store.ListTaskMessages(taskID, 10)
		if err != nil || len(items) < 4 {
			return false
		}
		last := items[len(items)-1]
		return last.Role == "assistant" && last.ResponseID == "resp-task-new-1" && last.Status == projectstate.StatusCompleted
	})
}
