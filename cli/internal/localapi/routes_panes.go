package localapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"shellman/cli/internal/projectstate"
)

var childSpawnAutoProgressFallbackDelay = 6 * time.Second

func (s *Server) registerPaneRoutes() {}

func detectServerInstanceID() string {
	if got := strings.TrimSpace(os.Getenv("SHELLMAN_SERVER_INSTANCE_ID")); got != "" {
		return got
	}
	return "srv_local"
}

func (s *Server) createRunAndLiveBinding(store *projectstate.Store, taskID, paneID, paneTarget string) (string, error) {
	runID := newRunID()
	if err := store.InsertRun(projectstate.RunRecord{
		RunID:     runID,
		TaskID:    taskID,
		RunStatus: projectstate.RunStatusRunning,
	}); err != nil {
		return "", err
	}
	if err := store.UpsertRunBinding(projectstate.RunBinding{
		RunID:            runID,
		ServerInstanceID: detectServerInstanceID(),
		PaneID:           paneID,
		PaneTarget:       paneTarget,
		BindingStatus:    projectstate.BindingStatusLive,
	}); err != nil {
		return "", err
	}
	return runID, nil
}

func (s *Server) persistTaskCurrentCommand(store *projectstate.Store, taskID, projectID, paneTarget string) error {
	currentCommand := strings.TrimSpace(s.detectPaneCurrentCommand(paneTarget))
	if currentCommand == "" {
		return nil
	}
	return store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:         taskID,
		ProjectID:      projectID,
		CurrentCommand: &currentCommand,
	})
}

func (s *Server) handleProjectRootPaneCreate(w http.ResponseWriter, r *http.Request, projectID string) {
	var req struct {
		Title    string `json:"title"`
		TaskRole string `json:"task_role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	title := strings.TrimSpace(req.Title)
	if len(title) > 256 {
		respondError(w, http.StatusBadRequest, "INVALID_TITLE", "title is too long")
		return
	}
	if s.deps.PaneService == nil {
		respondError(w, http.StatusInternalServerError, "PANE_SERVICE_UNAVAILABLE", "pane service is not configured")
		return
	}

	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}
	newTaskID, err := s.createTask(projectID, "", title)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_CREATE_FAILED", err.Error())
		return
	}
	paneID, err := s.deps.PaneService.CreateRootPaneInDir(repoRoot)
	if err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "PANE_CREATE_FAILED", err.Error())
		return
	}

	store := projectstate.NewStore(repoRoot)
	panes, err := store.LoadPanes()
	if err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "PANES_LOAD_FAILED", err.Error())
		return
	}
	paneUUID := uuid.NewString()
	panes[newTaskID] = projectstate.PaneBinding{
		TaskID:             newTaskID,
		PaneUUID:           paneUUID,
		PaneID:             paneID,
		PaneTarget:         paneID,
		ShellReadyRequired: true,
		ShellReadyAcked:    false,
	}
	if err := store.SavePanes(panes); err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "PANES_SAVE_FAILED", err.Error())
		return
	}
	if err := s.updateTaskStatusInternal(store, newTaskID, projectID, projectstate.StatusRunning); err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "STATUS_UPDATE_FAILED", err.Error())
		return
	}
	runID, err := s.createRunAndLiveBinding(store, newTaskID, paneID, paneID)
	if err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "RUN_CREATE_FAILED", err.Error())
		return
	}
	if err := s.persistTaskCurrentCommand(store, newTaskID, projectID, paneID); err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "TASK_COMMAND_SAVE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.status.updated", projectID, newTaskID, map[string]any{"status": projectstate.StatusRunning})
	s.publishEvent("pane.created", projectID, newTaskID, map[string]any{
		"relation":    "root",
		"run_id":      runID,
		"pane_uuid":   paneUUID,
		"pane_id":     paneID,
		"pane_target": paneID,
	})
	s.publishEvent("task.tree.updated", projectID, newTaskID, map[string]any{})
	respondOK(w, map[string]any{
		"task_id":     newTaskID,
		"run_id":      runID,
		"pane_uuid":   paneUUID,
		"pane_id":     paneID,
		"pane_target": paneID,
		"relation":    "root",
	})
}

func (s *Server) handlePaneCreate(w http.ResponseWriter, r *http.Request, taskID, relation string) {
	if relation != "sibling" && relation != "child" {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	var req struct {
		Title    string `json:"title"`
		TaskRole string `json:"task_role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	title := strings.TrimSpace(req.Title)
	if len(title) > 256 {
		respondError(w, http.StatusBadRequest, "INVALID_TITLE", "title is too long")
		return
	}
	if s.deps.PaneService == nil {
		respondError(w, http.StatusInternalServerError, "PANE_SERVICE_UNAVAILABLE", "pane service is not configured")
		return
	}

	projectID, store, entry, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}
	panes, err := store.LoadPanes()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PANES_LOAD_FAILED", err.Error())
		return
	}
	target := taskID
	if binding, ok := panes[taskID]; ok && binding.PaneTarget != "" {
		target = binding.PaneTarget
	}

	parentTaskID := entry.ParentTaskID
	if relation == "child" {
		parentTaskID = taskID
	}
	newTaskID, err := s.createTaskWithRole(projectID, parentTaskID, title, req.TaskRole)
	if err != nil {
		if errors.Is(err, errInvalidTaskRole) {
			respondError(w, http.StatusBadRequest, "INVALID_TASK_ROLE", err.Error())
			return
		}
		if errors.Is(err, errExecutorCannotDelegate) {
			respondError(w, http.StatusBadRequest, "EXECUTOR_CANNOT_DELEGATE", err.Error())
			return
		}
		if errors.Is(err, errPlannerOnlySpawnExecutor) {
			respondError(w, http.StatusBadRequest, "PLANNER_ONLY_SPAWN_EXECUTOR", err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "TASK_CREATE_FAILED", err.Error())
		return
	}

	paneID := ""
	if relation == "sibling" {
		paneID, err = s.deps.PaneService.CreateSiblingPaneInDir(target, repoRoot)
	} else {
		paneID, err = s.deps.PaneService.CreateChildPaneInDir(target, repoRoot)
	}
	if err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "PANE_CREATE_FAILED", err.Error())
		return
	}

	paneUUID := uuid.NewString()
	panes[newTaskID] = projectstate.PaneBinding{
		TaskID:             newTaskID,
		PaneUUID:           paneUUID,
		PaneID:             paneID,
		PaneTarget:         paneID,
		ShellReadyRequired: true,
		ShellReadyAcked:    false,
	}
	if err := store.SavePanes(panes); err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "PANES_SAVE_FAILED", err.Error())
		return
	}
	if err := s.updateTaskStatusInternal(store, newTaskID, projectID, projectstate.StatusRunning); err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "STATUS_UPDATE_FAILED", err.Error())
		return
	}
	runID, err := s.createRunAndLiveBinding(store, newTaskID, paneID, paneID)
	if err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "RUN_CREATE_FAILED", err.Error())
		return
	}
	if err := s.persistTaskCurrentCommand(store, newTaskID, projectID, paneID); err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "TASK_COMMAND_SAVE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.status.updated", projectID, newTaskID, map[string]any{"status": projectstate.StatusRunning})
	s.publishEvent("pane.created", projectID, newTaskID, map[string]any{
		"relation":    relation,
		"run_id":      runID,
		"pane_uuid":   paneUUID,
		"pane_id":     paneID,
		"pane_target": paneID,
	})
	s.publishEvent("task.tree.updated", projectID, newTaskID, map[string]any{})
	s.scheduleChildSpawnAutoProgressFallback(projectID, entry.SidecarMode, newTaskID, paneID, relation)
	respondOK(w, map[string]any{
		"task_id":     newTaskID,
		"run_id":      runID,
		"pane_uuid":   paneUUID,
		"pane_id":     paneID,
		"pane_target": paneID,
		"relation":    relation,
	})
}

func (s *Server) scheduleChildSpawnAutoProgressFallback(projectID, sidecarMode, childTaskID, paneTarget, relation string) {
	if s == nil || !strings.EqualFold(strings.TrimSpace(relation), "child") {
		return
	}
	if normalizeSidecarMode(sidecarMode) != projectstate.SidecarModeAutopilot {
		return
	}
	projectID = strings.TrimSpace(projectID)
	childTaskID = strings.TrimSpace(childTaskID)
	paneTarget = strings.TrimSpace(paneTarget)
	if projectID == "" || childTaskID == "" || paneTarget == "" {
		return
	}
	delay := childSpawnAutoProgressFallbackDelay
	if delay <= 0 {
		delay = 1500 * time.Millisecond
	}
	time.AfterFunc(delay, func() {
		s.runChildSpawnAutoProgressFallback(projectID, childTaskID, paneTarget)
	})
}

func (s *Server) runChildSpawnAutoProgressFallback(projectID, childTaskID, paneTarget string) {
	if s == nil {
		return
	}
	projectID = strings.TrimSpace(projectID)
	childTaskID = strings.TrimSpace(childTaskID)
	paneTarget = strings.TrimSpace(paneTarget)
	if projectID == "" || childTaskID == "" || paneTarget == "" {
		return
	}

	resolvedProjectID, store, entry, err := s.findTask(childTaskID)
	if err != nil {
		return
	}
	if projectID == "" {
		projectID = strings.TrimSpace(resolvedProjectID)
	}
	if normalizeSidecarMode(entry.SidecarMode) != projectstate.SidecarModeAutopilot {
		return
	}
	if isTaskTerminalStatus(entry.Status) || !strings.EqualFold(strings.TrimSpace(entry.Status), projectstate.StatusRunning) {
		return
	}

	panes, err := store.LoadPanes()
	if err != nil {
		return
	}
	binding, ok := panes[childTaskID]
	if !ok {
		return
	}
	resolvedPaneTarget := strings.TrimSpace(binding.PaneTarget)
	if resolvedPaneTarget == "" {
		resolvedPaneTarget = strings.TrimSpace(binding.PaneID)
	}
	if resolvedPaneTarget == "" || resolvedPaneTarget != paneTarget {
		return
	}

	runtime, foundRuntime, err := store.GetPaneRuntimeByPaneID(strings.TrimSpace(binding.PaneID))
	if err != nil || !foundRuntime {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(runtime.RuntimeStatus), "ready") {
		return
	}

	reqMeta := map[string]any{
		"caller_method":         "INTERNAL",
		"caller_path":           "internal:child-spawn-fallback",
		"caller_user_agent":     "",
		"caller_turn_uuid":      "",
		"caller_gateway_source": "local-agent-gateway-http",
		"caller_active_pane":    resolvedPaneTarget,
	}
	out, runErr := s.AutoCompleteByPane(AutoCompleteByPaneInput{
		PaneTarget:           resolvedPaneTarget,
		Summary:              "auto-complete: child spawn fallback and pane stable",
		TriggerSource:        "spawn-fallback",
		ObservedLastActiveAt: runtime.UpdatedAt,
		RequestMeta:          reqMeta,
		CallerPath:           "internal:child-spawn-fallback",
		CallerActivePane:     resolvedPaneTarget,
	})
	if runErr != nil {
		return
	}
	stage := "spawn_fallback.skipped"
	if out.Triggered {
		stage = "spawn_fallback.triggered"
	}
	s.writeTaskCompletionAuditLog(projectID, childTaskID, stage, taskCompletionAuditFields(map[string]any{
		"source":      "spawn-fallback",
		"pane_target": resolvedPaneTarget,
		"status":      out.Status,
		"reason":      out.Reason,
		"run_id":      out.RunID,
	}, reqMeta))
}

func (s *Server) handleGetTaskPane(w http.ResponseWriter, _ *http.Request, taskID string) {
	projectID, store, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	panes, err := store.LoadPanes()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PANES_LOAD_FAILED", err.Error())
		return
	}
	binding, ok := panes[taskID]
	if !ok {
		respondError(w, http.StatusNotFound, "TASK_PANE_NOT_FOUND", "task pane binding not found")
		return
	}
	if binding.PaneUUID == "" {
		binding.PaneUUID = uuid.NewString()
		panes[taskID] = binding
		if err := store.SavePanes(panes); err != nil {
			respondError(w, http.StatusInternalServerError, "PANES_SAVE_FAILED", err.Error())
			return
		}
	}
	if binding.PaneTarget == "" {
		binding.PaneTarget = binding.PaneID
	}
	var snapshotPayload map[string]any
	currentCommand := ""
	runtimePaneID := strings.TrimSpace(binding.PaneID)
	if runtimePaneID == "" {
		runtimePaneID = strings.TrimSpace(binding.PaneTarget)
	}
	if runtimePaneID != "" {
		runtimeRow, ok, err := store.GetPaneRuntimeByPaneID(runtimePaneID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "PANE_RUNTIME_LOAD_FAILED", err.Error())
			return
		}
		if ok {
			currentCommand = strings.TrimSpace(runtimeRow.CurrentCommand)
			var cursor any
			if runtimeRow.HasCursor {
				cursor = map[string]any{
					"x": runtimeRow.CursorX,
					"y": runtimeRow.CursorY,
				}
			}
			snapshotPayload = map[string]any{
				"output": runtimeRow.Snapshot,
				"frame": map[string]any{
					"mode": "reset",
					"data": runtimeRow.Snapshot,
				},
				"cursor":     cursor,
				"updated_at": runtimeRow.UpdatedAt,
			}
		}
	}
	if currentCommand == "" {
		currentCommand = strings.TrimSpace(s.detectPaneCurrentCommand(binding.PaneTarget))
	}
	respondOK(w, map[string]any{
		"project_id":      projectID,
		"task_id":         binding.TaskID,
		"pane_uuid":       binding.PaneUUID,
		"pane_id":         binding.PaneID,
		"pane_target":     binding.PaneTarget,
		"current_command": currentCommand,
		"snapshot":        snapshotPayload,
	})
}

func parsePaneHistoryLines(raw string) int {
	lines := 2000
	value := strings.TrimSpace(raw)
	if value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			lines = parsed
		}
	}
	if lines < 200 {
		lines = 200
	}
	if lines > 10000 {
		lines = 10000
	}
	return lines
}

func (s *Server) handleGetTaskPaneHistory(w http.ResponseWriter, r *http.Request, taskID string) {
	if s.deps.PaneService == nil {
		respondError(w, http.StatusInternalServerError, "PANE_SERVICE_UNAVAILABLE", "pane service is not configured")
		return
	}
	projectID, store, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	panes, err := store.LoadPanes()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PANES_LOAD_FAILED", err.Error())
		return
	}
	binding, ok := panes[taskID]
	if !ok {
		respondError(w, http.StatusNotFound, "TASK_PANE_NOT_FOUND", "task pane binding not found")
		return
	}
	target := strings.TrimSpace(binding.PaneTarget)
	if target == "" {
		target = strings.TrimSpace(binding.PaneID)
	}
	if target == "" {
		respondError(w, http.StatusNotFound, "TASK_PANE_NOT_FOUND", "task pane target not found")
		return
	}
	lines := parsePaneHistoryLines(r.URL.Query().Get("lines"))
	snapshot, err := s.deps.PaneService.CaptureHistory(target, lines)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PANE_HISTORY_CAPTURE_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"project_id":  projectID,
		"task_id":     strings.TrimSpace(taskID),
		"pane_uuid":   binding.PaneUUID,
		"pane_id":     binding.PaneID,
		"pane_target": target,
		"lines":       lines,
		"snapshot": map[string]any{
			"output": snapshot,
			"frame": map[string]any{
				"mode": "reset",
				"data": snapshot,
			},
			"cursor": nil,
		},
	})
}

func (s *Server) handlePaneManualLaunch(w http.ResponseWriter, _ *http.Request, taskID string) {
	if s.deps.PaneService == nil {
		respondError(w, http.StatusInternalServerError, "PANE_SERVICE_UNAVAILABLE", "pane service is not configured")
		return
	}
	projectID, store, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}
	panes, err := store.LoadPanes()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PANES_LOAD_FAILED", err.Error())
		return
	}
	prev := panes[taskID]
	target := prev.PaneTarget

	paneID := ""
	if target != "" {
		paneID, err = s.deps.PaneService.CreateSiblingPaneInDirLoginShell(target, repoRoot)
	}
	if paneID == "" || err != nil {
		paneID, err = s.deps.PaneService.CreateRootPaneInDirLoginShell(repoRoot)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "PANE_CREATE_FAILED", err.Error())
			return
		}
	}

	paneUUID := uuid.NewString()
	panes[taskID] = projectstate.PaneBinding{
		TaskID:             taskID,
		PaneUUID:           paneUUID,
		PaneID:             paneID,
		PaneTarget:         paneID,
		ShellReadyRequired: false,
		ShellReadyAcked:    true,
	}
	if err := store.SavePanes(panes); err != nil {
		respondError(w, http.StatusInternalServerError, "PANES_SAVE_FAILED", err.Error())
		return
	}
	if err := s.updateTaskStatusInternal(store, taskID, projectID, projectstate.StatusRunning); err != nil {
		respondError(w, http.StatusInternalServerError, "STATUS_UPDATE_FAILED", err.Error())
		return
	}
	if err := s.persistTaskCurrentCommand(store, taskID, projectID, paneID); err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_COMMAND_SAVE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.status.updated", projectID, taskID, map[string]any{"status": projectstate.StatusRunning})
	s.publishEvent("pane.created", projectID, taskID, map[string]any{
		"relation":    "manual",
		"pane_uuid":   paneUUID,
		"pane_id":     paneID,
		"pane_target": paneID,
	})
	s.publishEvent("task.tree.updated", projectID, taskID, map[string]any{})
	respondOK(w, map[string]any{
		"task_id":     taskID,
		"pane_uuid":   paneUUID,
		"pane_id":     paneID,
		"pane_target": paneID,
		"relation":    "manual",
	})
}

func (s *Server) rollbackTaskCreation(projectID, taskID string) error {
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		return err
	}
	store := projectstate.NewStore(repoRoot)
	entry, ok, err := findTaskEntryInProject(store, projectID, taskID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := store.DeleteTask(taskID); err != nil {
		return err
	}
	rows, err := store.ListTasksByProject(projectID)
	if err != nil {
		return err
	}
	parentTaskID := strings.TrimSpace(entry.ParentTaskID)
	if parentTaskID != "" {
		hasChildren := false
		parentStatus := ""
		for _, row := range rows {
			if row.TaskID == parentTaskID {
				parentStatus = row.Status
			}
			if strings.TrimSpace(row.ParentTaskID) == parentTaskID {
				hasChildren = true
			}
		}
		if !hasChildren && parentStatus == projectstate.StatusWaitingChildren {
			nextStatus := projectstate.StatusPending
			if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
				TaskID:    parentTaskID,
				ProjectID: projectID,
				Status:    &nextStatus,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
