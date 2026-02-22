package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"shellman/cli/internal/projectstate"
	"shellman/cli/internal/protocol"
)

type PaneStateReport struct {
	PaneID         string
	PaneTarget     string
	CurrentCommand string
	RuntimeStatus  string
	Snapshot       string
	SnapshotHash   string
	CursorX        int
	CursorY        int
	HasCursor      bool
	UpdatedAt      int64
}

type TaskStateSink interface {
	OnPaneReport(PaneStateReport)
}

type taskStateProject struct {
	ProjectID string
	RepoRoot  string
}

type taskStateProjectProvider func() ([]taskStateProject, error)

type taskStateStore interface {
	LoadPanes() (projectstate.PanesIndex, error)
	BatchUpsertRuntime(projectstate.RuntimeBatchUpdate) error
	ListTasksByProject(projectID string) ([]projectstate.TaskRecordRow, error)
	GetProjectMaxTaskLastModified(projectID string) (int64, error)
}

type taskStateStoreFactory func(repoRoot string) taskStateStore

type taskStateEventEmitter func(context.Context, protocol.Message) error

type taskRowsCache struct {
	maxLastModified int64
	rowsByTaskID    map[string]projectstate.TaskRecordRow
}

type TaskTreeReparentDelta struct {
	TaskID          string `json:"task_id"`
	OldParentTaskID string `json:"old_parent_task_id,omitempty"`
	NewParentTaskID string `json:"new_parent_task_id,omitempty"`
}

type TaskTreeDelta struct {
	ProjectID  string                  `json:"project_id"`
	Added      []projectstate.TaskNode `json:"added,omitempty"`
	Removed    []string                `json:"removed,omitempty"`
	Updated    []projectstate.TaskNode `json:"updated,omitempty"`
	Reparented []TaskTreeReparentDelta `json:"reparented,omitempty"`
}

func (d TaskTreeDelta) HasChanges() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0 || len(d.Updated) > 0 || len(d.Reparented) > 0
}

type TaskStateActor struct {
	mu sync.RWMutex

	paneLatest   map[string]PaneStateReport
	dirtyPaneIDs map[string]struct{}

	projectProvider taskStateProjectProvider
	storeFactory    taskStateStoreFactory
	emitEvent       taskStateEventEmitter
	now             func() time.Time

	projectCache map[string]taskRowsCache
}

var runtimeSnapshotMaxLines = 2000

func NewTaskStateActor() *TaskStateActor {
	return &TaskStateActor{
		paneLatest:   map[string]PaneStateReport{},
		dirtyPaneIDs: map[string]struct{}{},
		storeFactory: func(repoRoot string) taskStateStore { return projectstate.NewStore(repoRoot) },
		now:          time.Now,
		projectCache: map[string]taskRowsCache{},
	}
}

func trimToRecentLines(text string, maxLines int) string {
	if maxLines <= 0 || text == "" {
		return text
	}
	hasTrailingNewline := strings.HasSuffix(text, "\n")
	working := text
	if hasTrailingNewline {
		working = strings.TrimSuffix(working, "\n")
	}
	if working == "" {
		return text
	}
	lines := strings.Split(working, "\n")
	if len(lines) <= maxLines {
		return text
	}
	out := strings.Join(lines[len(lines)-maxLines:], "\n")
	if hasTrailingNewline {
		out += "\n"
	}
	return out
}

func (a *TaskStateActor) SetProjectProvider(provider taskStateProjectProvider) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.projectProvider = provider
}

func (a *TaskStateActor) SetStoreFactory(factory taskStateStoreFactory) {
	if a == nil || factory == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.storeFactory = factory
}

func (a *TaskStateActor) SetEventEmitter(emitter taskStateEventEmitter) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.emitEvent = emitter
}

func (a *TaskStateActor) OnPaneReport(r PaneStateReport) {
	if a == nil {
		return
	}
	paneID := strings.TrimSpace(r.PaneID)
	if paneID == "" {
		return
	}
	r.PaneID = paneID

	a.mu.Lock()
	defer a.mu.Unlock()
	prev, ok := a.paneLatest[paneID]
	if ok && samePaneContent(prev, r) {
		return
	}
	a.paneLatest[paneID] = r
	a.dirtyPaneIDs[paneID] = struct{}{}
}

func (a *TaskStateActor) Tick(ctx context.Context) {
	if a == nil {
		return
	}
	projects, _ := a.loadProjects()
	runtimeDelta := a.flushDirtyRuntime(projects)
	treeDeltas := a.diffProjectsTree(projects)
	if len(runtimeDelta.Panes) == 0 && len(runtimeDelta.Tasks) == 0 && len(treeDeltas) == 0 {
		return
	}

	payload := map[string]any{"mode": "delta"}
	if len(runtimeDelta.Panes) > 0 || len(runtimeDelta.Tasks) > 0 {
		payload["runtime"] = map[string]any{
			"panes": runtimeDelta.Panes,
			"tasks": runtimeDelta.Tasks,
		}
	}
	if len(treeDeltas) == 1 {
		payload["tree"] = treeDeltas[0]
	} else if len(treeDeltas) > 1 {
		payload["tree"] = treeDeltas
	}
	a.emitTmuxStatusDelta(ctx, payload)
}

func runTaskStateActorLoop(ctx context.Context, actor *TaskStateActor, interval time.Duration) {
	if actor == nil {
		return
	}
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			actor.Tick(ctx)
		}
	}
}

func (a *TaskStateActor) IsPaneDirty(paneID string) bool {
	if a == nil {
		return false
	}
	paneID = strings.TrimSpace(paneID)
	if paneID == "" {
		return false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	_, ok := a.dirtyPaneIDs[paneID]
	return ok
}

func (a *TaskStateActor) ClearDirtyForTest() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.dirtyPaneIDs = map[string]struct{}{}
}

func (a *TaskStateActor) DirtyCountForTest() int {
	if a == nil {
		return 0
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.dirtyPaneIDs)
}

func (a *TaskStateActor) loadProjects() ([]taskStateProject, error) {
	a.mu.RLock()
	provider := a.projectProvider
	a.mu.RUnlock()
	if provider == nil {
		return nil, nil
	}
	projects, err := provider()
	if err != nil {
		return nil, err
	}
	out := make([]taskStateProject, 0, len(projects))
	for _, project := range projects {
		projectID := strings.TrimSpace(project.ProjectID)
		repoRoot := strings.TrimSpace(project.RepoRoot)
		if projectID == "" || repoRoot == "" {
			continue
		}
		out = append(out, taskStateProject{ProjectID: projectID, RepoRoot: repoRoot})
	}
	return out, nil
}

func (a *TaskStateActor) flushDirtyRuntime(projects []taskStateProject) projectstate.RuntimeBatchUpdate {
	reports := a.swapDirtyReports()
	if len(reports) == 0 || len(projects) == 0 {
		return projectstate.RuntimeBatchUpdate{}
	}

	a.mu.RLock()
	storeFactory := a.storeFactory
	a.mu.RUnlock()
	if storeFactory == nil {
		return projectstate.RuntimeBatchUpdate{}
	}

	batchByRepo := map[string]projectstate.RuntimeBatchUpdate{}
	paneSeenByRepo := map[string]map[string]struct{}{}
	taskSeenByRepo := map[string]map[string]struct{}{}

	appendPane := func(repoRoot string, pane projectstate.PaneRuntimeRecord) {
		seen := paneSeenByRepo[repoRoot]
		if seen == nil {
			seen = map[string]struct{}{}
			paneSeenByRepo[repoRoot] = seen
		}
		if _, ok := seen[pane.PaneID]; ok {
			return
		}
		seen[pane.PaneID] = struct{}{}
		batch := batchByRepo[repoRoot]
		batch.Panes = append(batch.Panes, pane)
		batchByRepo[repoRoot] = batch
	}
	appendTask := func(repoRoot string, task projectstate.TaskRuntimeRecord) {
		seen := taskSeenByRepo[repoRoot]
		if seen == nil {
			seen = map[string]struct{}{}
			taskSeenByRepo[repoRoot] = seen
		}
		if _, ok := seen[task.TaskID]; ok {
			return
		}
		seen[task.TaskID] = struct{}{}
		batch := batchByRepo[repoRoot]
		batch.Tasks = append(batch.Tasks, task)
		batchByRepo[repoRoot] = batch
	}

	for _, report := range reports {
		trimmedSnapshot := trimToRecentLines(report.Snapshot, runtimeSnapshotMaxLines)
		paneRecord := projectstate.PaneRuntimeRecord{
			PaneID:         report.PaneID,
			PaneTarget:     report.PaneTarget,
			CurrentCommand: report.CurrentCommand,
			RuntimeStatus:  report.RuntimeStatus,
			Snapshot:       trimmedSnapshot,
			SnapshotHash:   sha1Text(trimmedSnapshot),
			CursorX:        report.CursorX,
			CursorY:        report.CursorY,
			HasCursor:      report.HasCursor,
			UpdatedAt:      report.UpdatedAt,
		}

		matched := false
		for _, project := range projects {
			store := storeFactory(project.RepoRoot)
			if store == nil {
				continue
			}
			panes, err := store.LoadPanes()
			if err != nil {
				continue
			}
			for taskID, binding := range panes {
				if !paneReportMatchesBinding(report, binding) {
					continue
				}
				appendPane(project.RepoRoot, paneRecord)
				appendTask(project.RepoRoot, projectstate.TaskRuntimeRecord{
					TaskID:         taskID,
					SourcePaneID:   report.PaneID,
					CurrentCommand: report.CurrentCommand,
					RuntimeStatus:  report.RuntimeStatus,
					SnapshotHash:   report.SnapshotHash,
					UpdatedAt:      report.UpdatedAt,
				})
				matched = true
				break
			}
			if matched {
				break
			}
		}
		if !matched {
			appendPane(projects[0].RepoRoot, paneRecord)
		}
	}

	merged := projectstate.RuntimeBatchUpdate{}
	for repoRoot, batch := range batchByRepo {
		store := storeFactory(repoRoot)
		if store == nil {
			continue
		}
		if err := store.BatchUpsertRuntime(batch); err != nil {
			continue
		}
		merged.Panes = append(merged.Panes, batch.Panes...)
		merged.Tasks = append(merged.Tasks, batch.Tasks...)
	}
	return merged
}

func (a *TaskStateActor) diffProjectsTree(projects []taskStateProject) []TaskTreeDelta {
	if len(projects) == 0 {
		return nil
	}
	a.mu.RLock()
	storeFactory := a.storeFactory
	a.mu.RUnlock()
	if storeFactory == nil {
		return nil
	}

	deltas := make([]TaskTreeDelta, 0)
	for _, project := range projects {
		store := storeFactory(project.RepoRoot)
		if store == nil {
			continue
		}
		maxLastModified, err := store.GetProjectMaxTaskLastModified(project.ProjectID)
		if err != nil {
			continue
		}

		a.mu.RLock()
		cache, cached := a.projectCache[project.ProjectID]
		a.mu.RUnlock()
		if cached && cache.maxLastModified == maxLastModified {
			continue
		}

		rows, err := store.ListTasksByProject(project.ProjectID)
		if err != nil {
			continue
		}
		delta := diffTaskRows(project.ProjectID, cache.rowsByTaskID, rows)
		if delta.HasChanges() {
			deltas = append(deltas, delta)
		}

		a.mu.Lock()
		a.projectCache[project.ProjectID] = taskRowsCache{
			maxLastModified: maxLastModified,
			rowsByTaskID:    taskRowsByID(rows),
		}
		a.mu.Unlock()
	}
	return deltas
}

func (a *TaskStateActor) emitTmuxStatusDelta(ctx context.Context, payload map[string]any) {
	a.mu.RLock()
	emitter := a.emitEvent
	nowFn := a.now
	a.mu.RUnlock()
	if emitter == nil {
		return
	}
	if nowFn == nil {
		nowFn = time.Now
	}
	msg := protocol.Message{
		ID:      fmt.Sprintf("evt_tmux_status_delta_%d", nowFn().UnixNano()),
		Type:    "event",
		Op:      "tmux.status",
		Payload: protocol.MustRaw(payload),
	}
	_ = emitter(ctx, msg)
}

func (a *TaskStateActor) swapDirtyReports() []PaneStateReport {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.dirtyPaneIDs) == 0 {
		return nil
	}
	reports := make([]PaneStateReport, 0, len(a.dirtyPaneIDs))
	for paneID := range a.dirtyPaneIDs {
		if report, ok := a.paneLatest[paneID]; ok {
			reports = append(reports, report)
		}
	}
	a.dirtyPaneIDs = map[string]struct{}{}
	return reports
}

func paneReportMatchesBinding(report PaneStateReport, binding projectstate.PaneBinding) bool {
	reportPaneID := strings.TrimSpace(report.PaneID)
	reportTarget := strings.TrimSpace(report.PaneTarget)
	bindingPaneID := strings.TrimSpace(binding.PaneID)
	bindingTarget := strings.TrimSpace(binding.PaneTarget)
	bindingUUID := strings.TrimSpace(binding.PaneUUID)
	if reportPaneID != "" && (reportPaneID == bindingPaneID || reportPaneID == bindingTarget || reportPaneID == bindingUUID) {
		return true
	}
	if reportTarget != "" && (reportTarget == bindingPaneID || reportTarget == bindingTarget || reportTarget == bindingUUID) {
		return true
	}
	return false
}

func taskRowsByID(rows []projectstate.TaskRecordRow) map[string]projectstate.TaskRecordRow {
	out := make(map[string]projectstate.TaskRecordRow, len(rows))
	for _, row := range rows {
		out[row.TaskID] = row
	}
	return out
}

func diffTaskRows(projectID string, oldRows map[string]projectstate.TaskRecordRow, newRows []projectstate.TaskRecordRow) TaskTreeDelta {
	if oldRows == nil {
		oldRows = map[string]projectstate.TaskRecordRow{}
	}
	newByID := taskRowsByID(newRows)
	delta := TaskTreeDelta{ProjectID: projectID}
	for taskID, row := range newByID {
		prev, ok := oldRows[taskID]
		if !ok {
			delta.Added = append(delta.Added, taskRowToTaskNode(row))
			continue
		}
		if strings.TrimSpace(prev.ParentTaskID) != strings.TrimSpace(row.ParentTaskID) {
			delta.Reparented = append(delta.Reparented, TaskTreeReparentDelta{
				TaskID:          taskID,
				OldParentTaskID: prev.ParentTaskID,
				NewParentTaskID: row.ParentTaskID,
			})
		}
		if hasTaskRowContentChanged(prev, row) {
			delta.Updated = append(delta.Updated, taskRowToTaskNode(row))
		}
	}
	for taskID := range oldRows {
		if _, ok := newByID[taskID]; !ok {
			delta.Removed = append(delta.Removed, taskID)
		}
	}
	sort.Slice(delta.Added, func(i, j int) bool { return delta.Added[i].TaskID < delta.Added[j].TaskID })
	sort.Slice(delta.Updated, func(i, j int) bool { return delta.Updated[i].TaskID < delta.Updated[j].TaskID })
	sort.Strings(delta.Removed)
	sort.Slice(delta.Reparented, func(i, j int) bool { return delta.Reparented[i].TaskID < delta.Reparented[j].TaskID })
	return delta
}

func hasTaskRowContentChanged(oldRow, newRow projectstate.TaskRecordRow) bool {
	return strings.TrimSpace(oldRow.Title) != strings.TrimSpace(newRow.Title) ||
		strings.TrimSpace(oldRow.CurrentCommand) != strings.TrimSpace(newRow.CurrentCommand) ||
		strings.TrimSpace(oldRow.Status) != strings.TrimSpace(newRow.Status) ||
		strings.TrimSpace(oldRow.Description) != strings.TrimSpace(newRow.Description) ||
		strings.TrimSpace(oldRow.Flag) != strings.TrimSpace(newRow.Flag) ||
		strings.TrimSpace(oldRow.FlagDesc) != strings.TrimSpace(newRow.FlagDesc) ||
		oldRow.Checked != newRow.Checked ||
		oldRow.LastModified != newRow.LastModified
}

func taskRowToTaskNode(row projectstate.TaskRecordRow) projectstate.TaskNode {
	return projectstate.TaskNode{
		TaskID:         row.TaskID,
		ParentTaskID:   row.ParentTaskID,
		Title:          row.Title,
		CurrentCommand: row.CurrentCommand,
		Description:    row.Description,
		Flag:           row.Flag,
		FlagDesc:       row.FlagDesc,
		Checked:        row.Checked,
		Status:         row.Status,
		LastModified:   row.LastModified,
	}
}

func samePaneContent(a, b PaneStateReport) bool {
	return strings.TrimSpace(a.PaneTarget) == strings.TrimSpace(b.PaneTarget) &&
		strings.TrimSpace(a.CurrentCommand) == strings.TrimSpace(b.CurrentCommand) &&
		strings.TrimSpace(a.RuntimeStatus) == strings.TrimSpace(b.RuntimeStatus) &&
		a.Snapshot == b.Snapshot &&
		strings.TrimSpace(a.SnapshotHash) == strings.TrimSpace(b.SnapshotHash) &&
		a.CursorX == b.CursorX &&
		a.CursorY == b.CursorY &&
		a.HasCursor == b.HasCursor
}
