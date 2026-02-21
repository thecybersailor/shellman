package localapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"termteam/cli/internal/projectstate"
)

func (s *Server) registerPaneRoutes() {}

func detectServerInstanceID() string {
	if got := strings.TrimSpace(os.Getenv("MUXT_SERVER_INSTANCE_ID")); got != "" {
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
		Title string `json:"title"`
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
	paneID, err := s.deps.PaneService.CreateRootPane()
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
	panes[newTaskID] = projectstate.PaneBinding{TaskID: newTaskID, PaneUUID: paneUUID, PaneID: paneID, PaneTarget: paneID}
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
		Title string `json:"title"`
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
	newTaskID, err := s.createTask(projectID, parentTaskID, title)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_CREATE_FAILED", err.Error())
		return
	}

	paneID := ""
	if relation == "sibling" {
		paneID, err = s.deps.PaneService.CreateSiblingPane(target)
	} else {
		paneID, err = s.deps.PaneService.CreateChildPane(target)
	}
	if err != nil {
		_ = s.rollbackTaskCreation(projectID, newTaskID)
		respondError(w, http.StatusInternalServerError, "PANE_CREATE_FAILED", err.Error())
		return
	}

	paneUUID := uuid.NewString()
	panes[newTaskID] = projectstate.PaneBinding{TaskID: newTaskID, PaneUUID: paneUUID, PaneID: paneID, PaneTarget: paneID}
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
	respondOK(w, map[string]any{
		"task_id":     newTaskID,
		"run_id":      runID,
		"pane_uuid":   paneUUID,
		"pane_id":     paneID,
		"pane_target": paneID,
		"relation":    relation,
	})
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

func (s *Server) handlePaneReopen(w http.ResponseWriter, _ *http.Request, taskID string) {
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
	prev := panes[taskID]
	target := prev.PaneTarget

	paneID := ""
	if target != "" {
		paneID, err = s.deps.PaneService.CreateSiblingPane(target)
	}
	if paneID == "" || err != nil {
		paneID, err = s.deps.PaneService.CreateRootPane()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "PANE_CREATE_FAILED", err.Error())
			return
		}
	}

	paneUUID := uuid.NewString()
	panes[taskID] = projectstate.PaneBinding{
		TaskID:     taskID,
		PaneUUID:   paneUUID,
		PaneID:     paneID,
		PaneTarget: paneID,
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
		"relation":    "reopen",
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
		"relation":    "reopen",
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
	err = os.Remove(filepath.Join(repoRoot, ".muxt", "tasks", taskID+".md"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
