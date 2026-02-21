package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"shellman/cli/internal/projectstate"
)

var validTaskStatus = map[string]struct{}{
	projectstate.StatusPending:         {},
	projectstate.StatusRunning:         {},
	projectstate.StatusWaitingUser:     {},
	projectstate.StatusWaitingChildren: {},
	projectstate.StatusCompleted:       {},
	projectstate.StatusFailed:          {},
	projectstate.StatusCanceled:        {},
}

func (s *Server) registerTaskRoutes() {
	s.mux.HandleFunc("/api/v1/projects/", s.handleProjectTree)
	s.mux.HandleFunc("/api/v1/tasks", s.handleCreateTask)
	s.mux.HandleFunc("/api/v1/tasks/", s.handleTaskActions)
}

func (s *Server) handleProjectTree(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/projects/"), "/")
	if len(parts) == 3 && parts[0] != "" && parts[1] == "panes" && parts[2] == "root" {
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.handleProjectRootPaneCreate(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "archive-done" {
		if r.Method != http.MethodPost {
			respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.handleProjectArchiveDone(w, r, parts[0])
		return
	}
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if len(parts) != 2 || parts[1] != "tree" || parts[0] == "" {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	projectID := parts[0]
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}
	store := projectstate.NewStore(repoRoot)
	rows, err := store.ListTasksByProject(projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TREE_LOAD_FAILED", err.Error())
		return
	}
	tree := projectstate.TaskTree{
		ProjectID: projectID,
		Nodes:     buildTreeNodesFromTaskRows(rows),
	}
	respondOK(w, tree)
}

func buildTreeNodesFromTaskRows(rows []projectstate.TaskRecordRow) []projectstate.TaskNode {
	ordered := make([]*projectstate.TaskNode, 0, len(rows))
	nodesByTaskID := make(map[string]*projectstate.TaskNode, len(rows))
	for _, row := range rows {
		node := &projectstate.TaskNode{
			TaskID:         row.TaskID,
			ParentTaskID:   row.ParentTaskID,
			Title:          row.Title,
			CurrentCommand: row.CurrentCommand,
			Description:    row.Description,
			Flag:           row.Flag,
			FlagDesc:       row.FlagDesc,
			FlagReaded:     row.FlagReaded,
			Checked:        row.Checked,
			Archived:       row.Archived,
			Status:         row.Status,
			LastModified:   row.LastModified,
		}
		ordered = append(ordered, node)
		nodesByTaskID[row.TaskID] = node
	}

	for _, row := range rows {
		parentID := strings.TrimSpace(row.ParentTaskID)
		if parentID == "" {
			continue
		}
		parentNode, ok := nodesByTaskID[parentID]
		if !ok {
			continue
		}
		parentNode.Children = append(parentNode.Children, row.TaskID)
		if !isTaskTerminalStatus(row.Status) {
			parentNode.PendingChildrenCount++
		}
	}

	result := make([]projectstate.TaskNode, 0, len(ordered))
	for _, node := range ordered {
		result = append(result, *node)
	}
	return result
}

func (s *Server) handleProjectArchiveDone(w http.ResponseWriter, _ *http.Request, projectID string) {
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return
	}
	store := projectstate.NewStore(repoRoot)
	affected, err := store.ArchiveCheckedTasksByProject(projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_ARCHIVE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.tree.updated", projectID, "", map[string]any{})
	respondOK(w, map[string]any{"project_id": projectID, "archived_count": affected})
}

func isTaskTerminalStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case projectstate.StatusCompleted, projectstate.StatusFailed, projectstate.StatusCanceled:
		return true
	default:
		return false
	}
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	var req struct {
		ProjectID    string `json:"project_id"`
		ParentTaskID string `json:"parent_task_id"`
		Title        string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	projectID := strings.TrimSpace(req.ProjectID)
	title := strings.TrimSpace(req.Title)
	if projectID == "" {
		respondError(w, http.StatusBadRequest, "INVALID_TASK", "project_id is required")
		return
	}
	if len(title) > 256 {
		respondError(w, http.StatusBadRequest, "INVALID_TITLE", "title is too long")
		return
	}
	newTaskID, err := s.createTask(projectID, req.ParentTaskID, title)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_CREATE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.created", projectID, newTaskID, map[string]any{})
	s.publishEvent("task.tree.updated", projectID, newTaskID, map[string]any{})
	respondOK(w, map[string]any{"task_id": newTaskID})
}

func (s *Server) handleTaskActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[0] == "" {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	taskID := parts[0]
	action := strings.Join(parts[1:], "/")
	switch {
	case r.Method == http.MethodGet && action == "diff":
		s.handleGetTaskDiff(w, r, taskID)
	case r.Method == http.MethodGet && action == "files":
		s.handleGetTaskFiles(w, r, taskID)
	case r.Method == http.MethodGet && action == "files/tree":
		s.handleGetTaskFileTree(w, r, taskID)
	case r.Method == http.MethodGet && action == "files/search":
		s.handleGetTaskFileSearch(w, r, taskID)
	case r.Method == http.MethodGet && action == "files/content":
		s.handleGetTaskFileContent(w, r, taskID)
	case r.Method == http.MethodGet && action == "files/raw":
		s.handleGetTaskFileRaw(w, r, taskID)
	case r.Method == http.MethodPost && action == "commit-message/generate":
		s.handlePostTaskCommitMessageGenerate(w, r, taskID)
	case r.Method == http.MethodPost && action == "commit":
		s.handlePostTaskCommit(w, r, taskID)
	case r.Method == http.MethodGet && action == "pane":
		s.handleGetTaskPane(w, r, taskID)
	case r.Method == http.MethodPost && action == "derive":
		s.handleDeriveTask(w, r, taskID)
	case r.Method == http.MethodPatch && action == "status":
		s.handleUpdateTaskStatus(w, r, taskID)
	case r.Method == http.MethodPatch && action == "check":
		s.handleUpdateTaskChecked(w, r, taskID)
	case r.Method == http.MethodPatch && action == "title":
		s.handleUpdateTaskTitle(w, r, taskID)
	case r.Method == http.MethodPatch && action == "description":
		s.handleUpdateTaskDescription(w, r, taskID)
	case r.Method == http.MethodPatch && action == "flag-readed":
		s.handleUpdateTaskFlagReaded(w, r, taskID)
	case r.Method == http.MethodGet && action == "notes":
		s.handleGetTaskNotes(w, r, taskID)
	case r.Method == http.MethodGet && action == "messages":
		s.handleGetTaskMessages(w, r, taskID)
	case r.Method == http.MethodGet && action == "autopilot":
		s.handleGetTaskAutopilot(w, r, taskID)
	case r.Method == http.MethodPatch && action == "autopilot":
		s.handlePatchTaskAutopilot(w, r, taskID)
	case r.Method == http.MethodPost && action == "messages":
		s.handlePostTaskMessage(w, r, taskID)
	case r.Method == http.MethodPost && action == "runs":
		s.handleCreateRun(w, r, taskID)
	case r.Method == http.MethodPost && action == "panes/sibling":
		s.handlePaneCreate(w, r, taskID, "sibling")
	case r.Method == http.MethodPost && action == "panes/child":
		s.handlePaneCreate(w, r, taskID, "child")
	case r.Method == http.MethodPost && action == "panes/reopen":
		s.handlePaneReopen(w, r, taskID)
	case r.Method == http.MethodPost && action == "adopt-pane":
		s.handleAdoptPane(w, r, taskID)
	default:
		respondError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	}
}

func (s *Server) handleGetTaskNotes(w http.ResponseWriter, _ *http.Request, taskID string) {
	_, store, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	notes, err := store.ListTaskNotes(taskID, 200)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "NOTES_LOAD_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{"task_id": taskID, "notes": notes})
}

func (s *Server) handleGetTaskMessages(w http.ResponseWriter, _ *http.Request, taskID string) {
	_, store, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	items, err := store.ListTaskMessages(taskID, 200)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_MESSAGES_LOAD_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{"task_id": taskID, "messages": items})
}

func (s *Server) handleGetTaskAutopilot(w http.ResponseWriter, _ *http.Request, taskID string) {
	if _, _, _, err := s.findTask(taskID); err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	enabled := false
	if s.taskAgentSupervisor != nil {
		enabled = s.taskAgentSupervisor.GetAutopilot(taskID)
	}
	respondOK(w, map[string]any{
		"task_id":   strings.TrimSpace(taskID),
		"autopilot": enabled,
	})
}

func (s *Server) handlePatchTaskAutopilot(w http.ResponseWriter, r *http.Request, taskID string) {
	if _, _, _, err := s.findTask(taskID); err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	var req struct {
		Autopilot *bool `json:"autopilot"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if req.Autopilot == nil {
		respondError(w, http.StatusBadRequest, "INVALID_AUTOPILOT", "autopilot is required")
		return
	}
	if s.taskAgentSupervisor == nil {
		respondError(w, http.StatusInternalServerError, "TASK_AGENT_SUPERVISOR_UNAVAILABLE", "task agent loop supervisor is unavailable")
		return
	}
	if err := s.taskAgentSupervisor.SetAutopilot(taskID, *req.Autopilot); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_AUTOPILOT", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"task_id":   strings.TrimSpace(taskID),
		"autopilot": s.taskAgentSupervisor.GetAutopilot(taskID),
	})
}

type agentLoopStreamingRunner interface {
	RunStream(ctx context.Context, userPrompt string, onTextDelta func(string)) (string, error)
}

type agentLoopStreamingWithToolsRunner interface {
	RunStreamWithTools(
		ctx context.Context,
		userPrompt string,
		onTextDelta func(string),
		onToolEvent func(map[string]any),
	) (string, error)
}

type assistantStructuredContent struct {
	Text  string           `json:"text"`
	Tools []map[string]any `json:"tools,omitempty"`
	Meta  map[string]any   `json:"meta,omitempty"`
}

func marshalAssistantStructuredContent(state assistantStructuredContent) string {
	raw, err := json.Marshal(state)
	if err != nil {
		return state.Text
	}
	return string(raw)
}

func (s *Server) handlePostTaskMessage(w http.ResponseWriter, r *http.Request, taskID string) {
	projectID, _, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	var req struct {
		Content     string `json:"content"`
		Source      string `json:"source"`
		ParentTask  string `json:"parent_task"`
		DisplayText string `json:"display_text"`
		Flag        string `json:"flag"`
		StatusMsg   string `json:"status_message"`
		Input       string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "user_input"
	}
	content := strings.TrimSpace(req.Content)
	if source == "user_input" || source == "parent_message" || source == "child_report" {
		if content == "" {
			respondError(w, http.StatusBadRequest, "INVALID_MESSAGE", "content is required")
			return
		}
	}
	displayContent := strings.TrimSpace(req.DisplayText)
	if displayContent == "" {
		displayContent = content
	}

	switch source {
	case "user_input", "parent_message", "child_report", "task_set_flag", "tty_write_stdin":
	default:
		respondError(w, http.StatusBadRequest, "INVALID_SOURCE", "source is invalid")
		return
	}

	if source == "task_set_flag" {
		flag := strings.TrimSpace(req.Flag)
		statusMessage := strings.TrimSpace(req.StatusMsg)
		if flag == "" {
			respondError(w, http.StatusBadRequest, "INVALID_FLAG_KEY", "flag is required")
			return
		}
		if statusMessage == "" {
			respondError(w, http.StatusBadRequest, "INVALID_STATUS_MESSAGE", "status_message is required")
			return
		}
		pid, store, _, findErr := s.findTask(taskID)
		if findErr != nil {
			respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", findErr.Error())
			return
		}
		if err := s.setTaskFlagInternal(store, pid, taskID, flag, statusMessage); err != nil {
			if strings.Contains(err.Error(), "unsupported task flag") {
				respondError(w, http.StatusBadRequest, "INVALID_FLAG_KEY", err.Error())
				return
			}
			respondError(w, http.StatusInternalServerError, "TASK_FLAG_UPDATE_FAILED", err.Error())
			return
		}
		respondOK(w, map[string]any{
			"task_id":        taskID,
			"flag":           flag,
			"status_message": statusMessage,
		})
		return
	}

	if source == "tty_write_stdin" {
		input := req.Input
		if input == "" {
			respondError(w, http.StatusBadRequest, "INVALID_INPUT", "input is required")
			return
		}
		if s.deps.TaskPromptSender == nil {
			respondError(w, http.StatusInternalServerError, "TASK_PROMPT_SENDER_UNAVAILABLE", "task prompt sender is unavailable")
			return
		}
		_, store, _, findErr := s.findTask(taskID)
		if findErr != nil {
			respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", findErr.Error())
			return
		}
		panes, err := store.LoadPanes()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "PANES_LOAD_FAILED", err.Error())
			return
		}
		binding, ok := panes[taskID]
		if !ok {
			respondError(w, http.StatusNotFound, "TASK_PANE_NOT_FOUND", "pane binding not found")
			return
		}
		target := strings.TrimSpace(binding.PaneTarget)
		if target == "" {
			target = strings.TrimSpace(binding.PaneID)
		}
		if target == "" {
			respondError(w, http.StatusNotFound, "TASK_PANE_NOT_FOUND", "pane target is empty")
			return
		}
		if err := s.deps.TaskPromptSender.SendInput(target, input); err != nil {
			respondError(w, http.StatusInternalServerError, "TASK_INPUT_SEND_FAILED", err.Error())
			return
		}
		respondOK(w, map[string]any{
			"task_id":         taskID,
			"pane_target":     target,
			"input_len":       len(input),
			"delivery_status": "sent",
		})
		return
	}

	if source == "parent_message" {
		parentTaskID := strings.TrimSpace(req.ParentTask)
		if parentTaskID == "" {
			respondError(w, http.StatusBadRequest, "INVALID_PARENT_TASK", "parent_task is required")
			return
		}
		_, _, entry, findErr := s.findTask(taskID)
		if findErr != nil {
			respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", findErr.Error())
			return
		}
		if strings.TrimSpace(entry.ParentTaskID) != parentTaskID {
			respondError(w, http.StatusBadRequest, "NOT_A_CHILD_TASK", "target task is not a child of parent_task")
			return
		}
	}

	if source == "child_report" {
		_, _, entry, findErr := s.findTask(taskID)
		if findErr != nil {
			respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", findErr.Error())
			return
		}
		parentTaskID := strings.TrimSpace(entry.ParentTaskID)
		if parentTaskID == "" {
			respondError(w, http.StatusBadRequest, "NO_PARENT_TASK", "task has no parent")
			return
		}
		parentPrompt := s.buildUserPrompt(parentTaskID, content)
		if err := s.sendTaskAgentLoop(r.Context(), TaskAgentLoopEvent{
			TaskID:         parentTaskID,
			ProjectID:      projectID,
			Source:         "child_report",
			DisplayContent: displayContent,
			AgentPrompt:    parentPrompt,
			TriggerMeta: map[string]any{
				"op":             "task.messages.send",
				"reported_by":    taskID,
				"reported_child": taskID,
			},
		}); err != nil {
			if errors.Is(err, ErrTaskAgentLoopUnavailable) {
				respondError(w, http.StatusInternalServerError, "AGENT_LOOP_UNAVAILABLE", err.Error())
				return
			}
			respondError(w, http.StatusInternalServerError, "TASK_MESSAGE_ENQUEUE_FAILED", err.Error())
			return
		}
		respondOK(w, map[string]any{
			"task_id":         taskID,
			"parent_task_id":  parentTaskID,
			"status":          "queued",
			"source":          "child_report",
			"display_content": displayContent,
		})
		return
	}

	agentPrompt := s.buildUserPrompt(taskID, content)
	if err := s.sendTaskAgentLoop(r.Context(), TaskAgentLoopEvent{
		TaskID:         taskID,
		ProjectID:      projectID,
		Source:         source,
		DisplayContent: displayContent,
		AgentPrompt:    agentPrompt,
		TriggerMeta: map[string]any{
			"op": "task.messages.send",
		},
	}); err != nil {
		if errors.Is(err, ErrTaskAgentLoopUnavailable) {
			respondError(w, http.StatusInternalServerError, "AGENT_LOOP_UNAVAILABLE", err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "TASK_MESSAGE_ENQUEUE_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"task_id": taskID,
		"status":  "queued",
		"source":  source,
	})
}

func (s *Server) handleAdoptPane(w http.ResponseWriter, r *http.Request, parentTaskID string) {
	var req struct {
		PaneTarget string `json:"pane_target"`
		Title      string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	paneTarget := strings.TrimSpace(req.PaneTarget)
	if paneTarget == "" {
		respondError(w, http.StatusBadRequest, "INVALID_PANE_TARGET", "pane_target is required")
		return
	}

	projectID, store, _, err := s.findTask(parentTaskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}

	panes, err := store.LoadPanes()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PANES_LOAD_FAILED", err.Error())
		return
	}
	for _, binding := range panes {
		if strings.TrimSpace(binding.PaneTarget) == paneTarget || strings.TrimSpace(binding.PaneID) == paneTarget {
			respondError(w, http.StatusConflict, "PANE_ALREADY_BOUND", "pane_target already bound")
			return
		}
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = fmt.Sprintf("Adopted %s", paneTarget)
	}

	taskID, err := s.createTask(projectID, parentTaskID, title)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_CREATE_FAILED", err.Error())
		return
	}

	paneUUID := uuid.NewString()
	panes[taskID] = projectstate.PaneBinding{
		TaskID:     taskID,
		PaneUUID:   paneUUID,
		PaneID:     paneTarget,
		PaneTarget: paneTarget,
	}
	if err := store.SavePanes(panes); err != nil {
		_ = s.rollbackTaskCreation(projectID, taskID)
		respondError(w, http.StatusInternalServerError, "PANES_SAVE_FAILED", err.Error())
		return
	}
	if err := s.updateTaskStatusInternal(store, taskID, projectID, projectstate.StatusRunning); err != nil {
		_ = s.rollbackTaskCreation(projectID, taskID)
		respondError(w, http.StatusInternalServerError, "STATUS_UPDATE_FAILED", err.Error())
		return
	}
	runID, err := s.createRunAndLiveBinding(store, taskID, paneTarget, paneTarget)
	if err != nil {
		_ = s.rollbackTaskCreation(projectID, taskID)
		respondError(w, http.StatusInternalServerError, "RUN_CREATE_FAILED", err.Error())
		return
	}

	s.publishEvent("task.status.updated", projectID, taskID, map[string]any{"status": projectstate.StatusRunning})
	s.publishEvent("pane.created", projectID, taskID, map[string]any{
		"relation":    "child",
		"run_id":      runID,
		"pane_uuid":   paneUUID,
		"pane_id":     paneTarget,
		"pane_target": paneTarget,
	})
	s.publishEvent("task.tree.updated", projectID, taskID, map[string]any{})
	respondOK(w, map[string]any{
		"task_id":     taskID,
		"run_id":      runID,
		"title":       title,
		"pane_uuid":   paneUUID,
		"pane_id":     paneTarget,
		"pane_target": paneTarget,
		"relation":    "child",
	})
}

func (s *Server) handleDeriveTask(w http.ResponseWriter, r *http.Request, parentTaskID string) {
	projectID, _, _, err := s.findTask(parentTaskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
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
	taskID, err := s.createTask(projectID, parentTaskID, title)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_DERIVE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.derived", projectID, taskID, map[string]any{"parent_task_id": parentTaskID})
	s.publishEvent("task.tree.updated", projectID, taskID, map[string]any{})
	respondOK(w, map[string]any{"task_id": taskID})
}

func (s *Server) handleUpdateTaskStatus(w http.ResponseWriter, r *http.Request, taskID string) {
	statusReq := struct {
		Status string `json:"status"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&statusReq); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	if _, ok := validTaskStatus[statusReq.Status]; !ok {
		respondError(w, http.StatusBadRequest, "INVALID_STATUS", "unsupported status")
		return
	}
	projectID, store, entry, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	wasCompleted := entry.Status == projectstate.StatusCompleted
	nextStatus := statusReq.Status
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:    taskID,
		ProjectID: projectID,
		Status:    &nextStatus,
	}); err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_UPDATE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.status.updated", projectID, taskID, map[string]any{"status": statusReq.Status})
	s.publishEvent("task.tree.updated", projectID, taskID, map[string]any{})
	if statusReq.Status == projectstate.StatusCompleted && !wasCompleted {
		s.enqueueTaskCompletionActions(projectID, taskID, "", "status.patch", buildTaskCompletionRequestMeta(r))
	}
	respondOK(w, map[string]any{"task_id": taskID, "status": statusReq.Status})
}

func (s *Server) handleUpdateTaskChecked(w http.ResponseWriter, r *http.Request, taskID string) {
	checkReq := struct {
		Checked bool `json:"checked"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&checkReq); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	projectID, store, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	nextChecked := checkReq.Checked
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:    taskID,
		ProjectID: projectID,
		Checked:   &nextChecked,
	}); err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_UPDATE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.tree.updated", projectID, taskID, map[string]any{"checked": checkReq.Checked})
	respondOK(w, map[string]any{"task_id": taskID, "checked": checkReq.Checked})
}

func (s *Server) handleUpdateTaskTitle(w http.ResponseWriter, r *http.Request, taskID string) {
	titleReq := struct {
		Title string `json:"title"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&titleReq); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	nextTitle := strings.TrimSpace(titleReq.Title)
	if nextTitle == "" {
		respondError(w, http.StatusBadRequest, "INVALID_TITLE", "title is required")
		return
	}
	if len(nextTitle) > 256 {
		respondError(w, http.StatusBadRequest, "INVALID_TITLE", "title is too long")
		return
	}

	projectID, store, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:    taskID,
		ProjectID: projectID,
		Title:     &nextTitle,
	}); err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_UPDATE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.title.updated", projectID, taskID, map[string]any{"title": nextTitle})
	s.publishEvent("task.tree.updated", projectID, taskID, map[string]any{})
	respondOK(w, map[string]any{"task_id": taskID, "title": nextTitle})
}

func (s *Server) handleUpdateTaskDescription(w http.ResponseWriter, r *http.Request, taskID string) {
	descReq := struct {
		Description string `json:"description"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&descReq); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	nextDescription := strings.TrimSpace(descReq.Description)
	if len(nextDescription) > 20000 {
		respondError(w, http.StatusBadRequest, "INVALID_DESCRIPTION", "description is too long")
		return
	}

	projectID, store, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:      taskID,
		ProjectID:   projectID,
		Description: &nextDescription,
	}); err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_UPDATE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.description.updated", projectID, taskID, map[string]any{"description": nextDescription})
	s.publishEvent("task.tree.updated", projectID, taskID, map[string]any{})
	respondOK(w, map[string]any{"task_id": taskID, "description": nextDescription})
}

func (s *Server) handleUpdateTaskFlagReaded(w http.ResponseWriter, r *http.Request, taskID string) {
	readReq := struct {
		FlagReaded bool `json:"flag_readed"`
	}{}
	if err := json.NewDecoder(r.Body).Decode(&readReq); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	projectID, store, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	nextFlagReaded := readReq.FlagReaded
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:     taskID,
		ProjectID:  projectID,
		FlagReaded: &nextFlagReaded,
	}); err != nil {
		respondError(w, http.StatusInternalServerError, "TASK_UPDATE_FAILED", err.Error())
		return
	}
	s.publishEvent("task.tree.updated", projectID, taskID, map[string]any{"flag_readed": readReq.FlagReaded})
	respondOK(w, map[string]any{"task_id": taskID, "flag_readed": readReq.FlagReaded})
}

func buildTaskCompletionRequestMeta(r *http.Request) map[string]any {
	if r == nil {
		return map[string]any{}
	}
	return map[string]any{
		"caller_method":         strings.TrimSpace(r.Method),
		"caller_path":           strings.TrimSpace(r.URL.Path),
		"caller_user_agent":     strings.TrimSpace(r.UserAgent()),
		"caller_turn_uuid":      strings.TrimSpace(r.Header.Get("X-Shellman-Turn-UUID")),
		"caller_gateway_source": strings.TrimSpace(r.Header.Get("X-Shellman-Gateway-Source")),
		"caller_active_pane":    strings.TrimSpace(r.Header.Get("X-Shellman-Active-Pane-Target")),
	}
}

func (s *Server) createTask(projectID, parentTaskID, title string) (string, error) {
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		return "", err
	}
	store := projectstate.NewStore(repoRoot)
	taskID := fmt.Sprintf("t_%d", time.Now().UnixNano())
	entry := projectstate.TaskRecord{
		TaskID:       taskID,
		ProjectID:    projectID,
		ParentTaskID: parentTaskID,
		Title:        title,
		Description:  "",
		Flag:         "",
		FlagDesc:     "",
		Checked:      false,
		Status:       projectstate.StatusPending,
	}
	if err := store.InsertTask(entry); err != nil {
		return "", err
	}
	parentTaskID = strings.TrimSpace(parentTaskID)
	if parentTaskID != "" {
		parent, ok, err := findTaskEntryInProject(store, projectID, parentTaskID)
		if err != nil {
			return "", err
		}
		if ok && parent.Status != projectstate.StatusWaitingChildren {
			nextStatus := projectstate.StatusWaitingChildren
			if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
				TaskID:    parentTaskID,
				ProjectID: projectID,
				Status:    &nextStatus,
			}); err != nil {
				return "", err
			}
		}
	}
	if err := s.writeTaskMarkdown(repoRoot, taskID, title); err != nil {
		return "", err
	}
	return taskID, nil
}

func (s *Server) writeTaskMarkdown(repoRoot, taskID, title string) error {
	tasksDir := filepath.Join(repoRoot, ".shellman", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("# %s\n\n- task_id: %s\n", title, taskID)
	return os.WriteFile(filepath.Join(tasksDir, taskID+".md"), []byte(content), 0o644)
}

func (s *Server) findProjectRepoRoot(projectID string) (string, error) {
	projects, err := s.deps.ProjectsStore.ListProjects()
	if err != nil {
		return "", err
	}
	for _, p := range projects {
		if p.ProjectID == projectID {
			return p.RepoRoot, nil
		}
	}
	return "", errors.New("project not found")
}

func (s *Server) findTask(taskID string) (string, *projectstate.Store, projectstate.TaskIndexEntry, error) {
	projects, err := s.deps.ProjectsStore.ListProjects()
	if err != nil {
		return "", nil, projectstate.TaskIndexEntry{}, err
	}
	for _, p := range projects {
		store := projectstate.NewStore(p.RepoRoot)
		entry, ok, err := findTaskEntryInProject(store, p.ProjectID, taskID)
		if err != nil {
			return "", nil, projectstate.TaskIndexEntry{}, err
		}
		if ok {
			return p.ProjectID, store, entry, nil
		}
	}
	return "", nil, projectstate.TaskIndexEntry{}, errors.New("task not found")
}

func (s *Server) updateTaskStatusInternal(store *projectstate.Store, taskID, projectID, status string) error {
	_, ok, err := findTaskEntryInProject(store, projectID, taskID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("task not found")
	}
	nextStatus := strings.TrimSpace(status)
	if nextStatus == "" {
		return errors.New("status is required")
	}
	return store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:    taskID,
		ProjectID: projectID,
		Status:    &nextStatus,
	})
}

func findTaskEntryInProject(store *projectstate.Store, projectID, taskID string) (projectstate.TaskIndexEntry, bool, error) {
	rows, err := store.ListTasksByProject(projectID)
	if err != nil {
		return projectstate.TaskIndexEntry{}, false, err
	}
	for _, row := range rows {
		if row.TaskID == taskID {
			return taskRowToTaskIndexEntry(row), true, nil
		}
	}
	return projectstate.TaskIndexEntry{}, false, nil
}

func taskRowToTaskIndexEntry(row projectstate.TaskRecordRow) projectstate.TaskIndexEntry {
	return projectstate.TaskIndexEntry{
		TaskID:       row.TaskID,
		ProjectID:    row.ProjectID,
		ParentTaskID: row.ParentTaskID,
		Title:        row.Title,
		Description:  row.Description,
		Flag:         row.Flag,
		FlagDesc:     row.FlagDesc,
		FlagReaded:   row.FlagReaded,
		Archived:     row.Archived,
		Status:       row.Status,
		LastModified: row.LastModified,
	}
}
