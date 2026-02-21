package localapi

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestTaskAgentLoopSupervisor_StopTaskCancelsRunningEvent(t *testing.T) {
	started := make(chan struct{})
	done := make(chan error, 1)
	supervisor := newTaskAgentLoopSupervisor(nil, func(ctx context.Context, evt TaskAgentLoopEvent) error {
		select {
		case <-started:
		default:
			close(started)
		}
		<-ctx.Done()
		done <- ctx.Err()
		return ctx.Err()
	})
	if err := supervisor.Enqueue(context.Background(), TaskAgentLoopEvent{
		TaskID:         "t1",
		DisplayContent: "x",
		AgentPrompt:    "x",
	}); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("event did not start")
	}
	if !supervisor.StopTask("t1") {
		t.Fatal("expected stop returns true")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("running event not canceled")
	}
}
