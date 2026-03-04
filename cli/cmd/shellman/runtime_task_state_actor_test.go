package main

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"shellman/cli/internal/projectstate"
	"shellman/cli/internal/protocol"
)

type fakeTaskStateStore struct {
	panesByTask    projectstate.PanesIndex
	tasksByProject map[string][]projectstate.TaskRecordRow
	maxByProject   map[string]int64

	batchCalls    int
	lastBatch     projectstate.RuntimeBatchUpdate
	batchSignal   chan struct{}
	maxQueryCalls atomic.Int32
}

func (f *fakeTaskStateStore) LoadPanes() (projectstate.PanesIndex, error) {
	out := projectstate.PanesIndex{}
	for k, v := range f.panesByTask {
		out[k] = v
	}
	return out, nil
}

func (f *fakeTaskStateStore) BatchUpsertRuntime(input projectstate.RuntimeBatchUpdate) error {
	f.batchCalls++
	f.lastBatch = input
	if f.batchSignal != nil {
		select {
		case f.batchSignal <- struct{}{}:
		default:
		}
	}
	return nil
}

func (f *fakeTaskStateStore) ListTasksByProject(projectID string) ([]projectstate.TaskRecordRow, error) {
	rows := f.tasksByProject[projectID]
	out := make([]projectstate.TaskRecordRow, len(rows))
	copy(out, rows)
	return out, nil
}

func (f *fakeTaskStateStore) GetProjectMaxTaskLastModified(projectID string) (int64, error) {
	f.maxQueryCalls.Add(1)
	return f.maxByProject[projectID], nil
}

type fakeTaskStateEmitter struct {
	messages []protocol.Message
}

func (f *fakeTaskStateEmitter) emit(_ context.Context, msg protocol.Message) error {
	f.messages = append(f.messages, msg)
	return nil
}

func TestTaskStateActor_OnPaneReport_MarksDirtyByPaneID(t *testing.T) {
	actor := NewTaskStateActor()
	actor.OnPaneReport(PaneStateReport{PaneID: "e2e:0.0", SnapshotHash: "h1"})
	if !actor.IsPaneDirty("e2e:0.0") {
		t.Fatal("expected dirty")
	}
}

func TestTaskStateActor_OnPaneReport_NoDirtyWhenContentUnchanged(t *testing.T) {
	actor := NewTaskStateActor()
	r := PaneStateReport{PaneID: "e2e:0.0", CurrentCommand: "npm", RuntimeStatus: "running", SnapshotHash: "h1"}
	actor.OnPaneReport(r)
	actor.ClearDirtyForTest()
	actor.OnPaneReport(r)
	if actor.DirtyCountForTest() != 0 {
		t.Fatal("should not be dirty")
	}
}

func TestTaskStateActor_Tick_FlushesOnlyChangedAndEmitsTmuxStatusDelta(t *testing.T) {
	store := &fakeTaskStateStore{
		panesByTask: projectstate.PanesIndex{
			"t1": {TaskID: "t1", PaneID: "e2e:0.0", PaneTarget: "e2e:0.0"},
		},
		tasksByProject: map[string][]projectstate.TaskRecordRow{},
		maxByProject:   map[string]int64{"p1": 0},
	}
	emitter := &fakeTaskStateEmitter{}
	actor := NewTaskStateActor()
	actor.SetProjectProvider(func() ([]taskStateProject, error) {
		return []taskStateProject{{ProjectID: "p1", RepoRoot: "/tmp/p1"}}, nil
	})
	actor.SetStoreFactory(func(repoRoot string) taskStateStore {
		return store
	})
	actor.SetEventEmitter(emitter.emit)

	r := PaneStateReport{PaneID: "e2e:0.0", PaneTarget: "e2e:0.0", SnapshotHash: "h1", Snapshot: "hello", RuntimeStatus: "running", UpdatedAt: 100}
	actor.OnPaneReport(r)
	actor.OnPaneReport(r)
	actor.Tick(context.Background())

	if store.batchCalls != 1 {
		t.Fatalf("expected one runtime batch upsert, got %d", store.batchCalls)
	}
	if len(store.lastBatch.Panes) != 1 || len(store.lastBatch.Tasks) != 1 {
		t.Fatalf("unexpected runtime batch: %#v", store.lastBatch)
	}
	if len(emitter.messages) != 1 {
		t.Fatalf("expected one tmux.status event, got %d", len(emitter.messages))
	}
	if emitter.messages[0].Op != "tmux.status" {
		t.Fatalf("unexpected op: %s", emitter.messages[0].Op)
	}
	var payload struct {
		Mode    string `json:"mode"`
		Runtime struct {
			Tasks []struct {
				TaskID         string `json:"task_id"`
				CurrentCommand string `json:"current_command"`
			} `json:"tasks"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal(emitter.messages[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if payload.Mode != "delta" {
		t.Fatalf("expected mode=delta, got %q", payload.Mode)
	}
	if len(payload.Runtime.Tasks) != 1 || payload.Runtime.Tasks[0].TaskID != "t1" {
		t.Fatalf("unexpected runtime task payload: %#v", payload.Runtime.Tasks)
	}
}

func TestTaskStateActor_Tick_KeepsCommandTransitionSequencePerPane(t *testing.T) {
	store := &fakeTaskStateStore{
		panesByTask: projectstate.PanesIndex{
			"t1": {TaskID: "t1", PaneID: "e2e:0.0", PaneTarget: "e2e:0.0"},
		},
		tasksByProject: map[string][]projectstate.TaskRecordRow{},
		maxByProject:   map[string]int64{"p1": 0},
	}
	actor := NewTaskStateActor()
	actor.SetProjectProvider(func() ([]taskStateProject, error) {
		return []taskStateProject{{ProjectID: "p1", RepoRoot: "/tmp/p1"}}, nil
	})
	actor.SetStoreFactory(func(repoRoot string) taskStateStore {
		return store
	})

	actor.OnPaneReport(PaneStateReport{
		PaneID:         "e2e:0.0",
		PaneTarget:     "e2e:0.0",
		CurrentCommand: "codex",
		RuntimeStatus:  "running",
		SnapshotHash:   "h1",
		UpdatedAt:      100,
	})
	actor.OnPaneReport(PaneStateReport{
		PaneID:         "e2e:0.0",
		PaneTarget:     "e2e:0.0",
		CurrentCommand: "node",
		RuntimeStatus:  "running",
		SnapshotHash:   "h2",
		UpdatedAt:      101,
	})

	actor.Tick(context.Background())

	if store.batchCalls != 1 {
		t.Fatalf("expected one runtime batch upsert, got %d", store.batchCalls)
	}
	if len(store.lastBatch.Tasks) != 2 {
		t.Fatalf("expected two task runtime updates in order, got %#v", store.lastBatch.Tasks)
	}
	if store.lastBatch.Tasks[0].CurrentCommand != "codex" || store.lastBatch.Tasks[1].CurrentCommand != "node" {
		t.Fatalf("unexpected command sequence: %#v", store.lastBatch.Tasks)
	}
}

func TestTaskStateActor_Tick_TrimsSnapshotToRecentLines(t *testing.T) {
	oldMaxLines := runtimeSnapshotMaxLines
	runtimeSnapshotMaxLines = 3
	defer func() { runtimeSnapshotMaxLines = oldMaxLines }()

	store := &fakeTaskStateStore{
		panesByTask: projectstate.PanesIndex{
			"t1": {TaskID: "t1", PaneID: "e2e:0.0", PaneTarget: "e2e:0.0"},
		},
		tasksByProject: map[string][]projectstate.TaskRecordRow{},
		maxByProject:   map[string]int64{"p1": 0},
	}
	actor := NewTaskStateActor()
	actor.SetProjectProvider(func() ([]taskStateProject, error) {
		return []taskStateProject{{ProjectID: "p1", RepoRoot: "/tmp/p1"}}, nil
	})
	actor.SetStoreFactory(func(repoRoot string) taskStateStore {
		return store
	})

	actor.OnPaneReport(PaneStateReport{
		PaneID:        "e2e:0.0",
		PaneTarget:    "e2e:0.0",
		RuntimeStatus: "running",
		Snapshot:      "l1\nl2\nl3\nl4\nl5\nl6\n",
		UpdatedAt:     100,
	})

	actor.Tick(context.Background())

	if store.batchCalls != 1 {
		t.Fatalf("expected one runtime batch upsert, got %d", store.batchCalls)
	}
	if len(store.lastBatch.Panes) != 1 {
		t.Fatalf("expected one pane runtime row, got %d", len(store.lastBatch.Panes))
	}
	if got := store.lastBatch.Panes[0].Snapshot; got != "l4\nl5\nl6\n" {
		t.Fatalf("expected trimmed snapshot, got %q", got)
	}
	if got := store.lastBatch.Panes[0].SnapshotHash; got != sha1Text("l4\nl5\nl6\n") {
		t.Fatalf("unexpected snapshot hash, got %q", got)
	}
}

func TestTaskStateActor_Tick_EmitsTreeDeltaWhenTasksChanged(t *testing.T) {
	store := &fakeTaskStateStore{
		panesByTask: projectstate.PanesIndex{},
		tasksByProject: map[string][]projectstate.TaskRecordRow{
			"p1": {
				{TaskID: "t1", ProjectID: "p1", ParentTaskID: "", Title: "root", Status: projectstate.StatusPending, LastModified: 1},
				{TaskID: "t2", ProjectID: "p1", ParentTaskID: "t1", Title: "child", Status: projectstate.StatusRunning, LastModified: 1},
				{TaskID: "t4", ProjectID: "p1", ParentTaskID: "", Title: "removed", Status: projectstate.StatusPending, LastModified: 1},
			},
		},
		maxByProject: map[string]int64{"p1": 1},
	}
	emitter := &fakeTaskStateEmitter{}
	actor := NewTaskStateActor()
	actor.SetProjectProvider(func() ([]taskStateProject, error) {
		return []taskStateProject{{ProjectID: "p1", RepoRoot: "/tmp/p1"}}, nil
	})
	actor.SetStoreFactory(func(repoRoot string) taskStateStore {
		return store
	})
	actor.SetEventEmitter(emitter.emit)

	actor.Tick(context.Background())
	emitter.messages = nil

	store.tasksByProject["p1"] = []projectstate.TaskRecordRow{
		{TaskID: "t1", ProjectID: "p1", ParentTaskID: "", Title: "root-v2", Status: projectstate.StatusPending, LastModified: 2},
		{TaskID: "t2", ProjectID: "p1", ParentTaskID: "", Title: "child", Status: projectstate.StatusRunning, LastModified: 2},
		{TaskID: "t3", ProjectID: "p1", ParentTaskID: "t1", Title: "added", Status: projectstate.StatusPending, LastModified: 2},
	}
	store.maxByProject["p1"] = 2

	actor.Tick(context.Background())

	if len(emitter.messages) != 1 {
		t.Fatalf("expected one delta event after task change, got %d", len(emitter.messages))
	}
	var payload struct {
		Tree TaskTreeDelta `json:"tree"`
	}
	if err := json.Unmarshal(emitter.messages[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
	if payload.Tree.ProjectID != "p1" {
		t.Fatalf("unexpected project id: %s", payload.Tree.ProjectID)
	}
	if len(payload.Tree.Added) != 1 || payload.Tree.Added[0].TaskID != "t3" {
		t.Fatalf("unexpected added delta: %#v", payload.Tree.Added)
	}
	if len(payload.Tree.Removed) != 1 || payload.Tree.Removed[0] != "t4" {
		t.Fatalf("unexpected removed delta: %#v", payload.Tree.Removed)
	}
	if len(payload.Tree.Updated) == 0 {
		t.Fatal("expected updated delta")
	}
	if len(payload.Tree.Reparented) != 1 || payload.Tree.Reparented[0].TaskID != "t2" {
		t.Fatalf("unexpected reparented delta: %#v", payload.Tree.Reparented)
	}
}

func TestTaskStateActor_EventLoop_NoPollingWithoutTrigger(t *testing.T) {
	store := &fakeTaskStateStore{
		panesByTask:      projectstate.PanesIndex{},
		tasksByProject:   map[string][]projectstate.TaskRecordRow{},
		maxByProject:     map[string]int64{"p1": 0},
		batchSignal:      make(chan struct{}, 1),
	}
	actor := NewTaskStateActor()
	actor.SetProjectProvider(func() ([]taskStateProject, error) {
		return []taskStateProject{{ProjectID: "p1", RepoRoot: "/tmp/p1"}}, nil
	})
	actor.SetStoreFactory(func(repoRoot string) taskStateStore {
		return store
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runTaskStateActorLoop(ctx, actor)
		close(done)
	}()
	time.Sleep(40 * time.Millisecond)
	if got := store.maxQueryCalls.Load(); got != 0 {
		t.Fatalf("expected no task-state polling query, got %d", got)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("task state loop did not stop after context cancel")
	}
}

func TestTaskStateActor_EventLoop_TriggeredByPaneReport(t *testing.T) {
	store := &fakeTaskStateStore{
		panesByTask: projectstate.PanesIndex{
			"t1": {TaskID: "t1", PaneID: "e2e:0.0", PaneTarget: "e2e:0.0"},
		},
		tasksByProject: map[string][]projectstate.TaskRecordRow{},
		maxByProject:   map[string]int64{"p1": 0},
		batchSignal:    make(chan struct{}, 1),
	}
	actor := NewTaskStateActor()
	actor.SetProjectProvider(func() ([]taskStateProject, error) {
		return []taskStateProject{{ProjectID: "p1", RepoRoot: "/tmp/p1"}}, nil
	})
	actor.SetStoreFactory(func(repoRoot string) taskStateStore {
		return store
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go runTaskStateActorLoop(ctx, actor)
	actor.OnPaneReport(PaneStateReport{
		PaneID:        "e2e:0.0",
		PaneTarget:    "e2e:0.0",
		RuntimeStatus: "running",
		Snapshot:      "hello\n",
		SnapshotHash:  "h1",
		UpdatedAt:     100,
	})

	select {
	case <-store.batchSignal:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected pane report to trigger runtime flush without ticker polling")
	}
}

func TestTaskStateActor_OnPaneReport_DropsTriggerWhenQueueFull(t *testing.T) {
	actor := NewTaskStateActor()
	actor.triggerQueue <- struct{}{}

	start := time.Now()
	actor.OnPaneReport(PaneStateReport{
		PaneID:       "e2e:0.0",
		SnapshotHash: "h1",
	})
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("OnPaneReport should not block when trigger queue is full")
	}
	if got := len(actor.triggerQueue); got != 1 {
		t.Fatalf("expected trigger queue to stay full with one signal, got %d", got)
	}
	if !actor.IsPaneDirty("e2e:0.0") {
		t.Fatal("expected pane to be marked dirty even when trigger signal is dropped")
	}
}

func TestTaskStateActor_EventLoop_BurstReports_CoalescedSingleFlush(t *testing.T) {
	store := &fakeTaskStateStore{
		panesByTask: projectstate.PanesIndex{
			"t1": {TaskID: "t1", PaneID: "e2e:0.0", PaneTarget: "e2e:0.0"},
		},
		tasksByProject: map[string][]projectstate.TaskRecordRow{},
		maxByProject:   map[string]int64{"p1": 0},
		batchSignal:    make(chan struct{}, 1),
	}
	actor := NewTaskStateActor()
	actor.SetProjectProvider(func() ([]taskStateProject, error) {
		return []taskStateProject{{ProjectID: "p1", RepoRoot: "/tmp/p1"}}, nil
	})
	actor.SetStoreFactory(func(repoRoot string) taskStateStore {
		return store
	})

	// Queue several updates before the loop starts; one trigger should flush all buffered reports.
	actor.OnPaneReport(PaneStateReport{PaneID: "e2e:0.0", PaneTarget: "e2e:0.0", SnapshotHash: "h1", UpdatedAt: 100})
	actor.OnPaneReport(PaneStateReport{PaneID: "e2e:0.0", PaneTarget: "e2e:0.0", SnapshotHash: "h2", UpdatedAt: 101})
	actor.OnPaneReport(PaneStateReport{PaneID: "e2e:0.0", PaneTarget: "e2e:0.0", SnapshotHash: "h3", UpdatedAt: 102})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runTaskStateActorLoop(ctx, actor)
		close(done)
	}()

	select {
	case <-store.batchSignal:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected buffered reports to flush after loop start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("task state loop did not stop after context cancel")
	}
	if store.batchCalls != 1 {
		t.Fatalf("expected one coalesced batch flush, got %d", store.batchCalls)
	}
	if len(store.lastBatch.Tasks) != 3 {
		t.Fatalf("expected three task runtime records in coalesced flush, got %d", len(store.lastBatch.Tasks))
	}
}

func TestTaskStateActor_RunLoop_ReturnsWhenTriggerQueueNil(t *testing.T) {
	actor := NewTaskStateActor()
	actor.triggerQueue = nil
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		actor.runLoop(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected runLoop to return immediately when trigger queue is nil")
	}
}

func TestTaskStateActorLoop_NilActor_ReturnsImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		runTaskStateActorLoop(ctx, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected runTaskStateActorLoop(nil) to return immediately")
	}
}
