package localapi

import (
	"log/slog"
	"net/http"
	"strings"

	"termteam/cli/internal/projectstate"
)

type AutoCompleteByPaneInput struct {
	PaneTarget           string
	Summary              string
	TriggerSource        string
	ObservedLastActiveAt int64
	RequestMeta          map[string]any
	CallerPath           string
	CallerActivePane     string
}

type AutoCompleteByPaneResult struct {
	Triggered   bool
	PaneTarget  string
	Reason      string
	RunID       string
	TaskID      string
	Status      string
	SummaryUsed string
}

type AutoCompleteByPaneError struct {
	HTTPStatus int
	Code       string
	Message    string
}

func (e *AutoCompleteByPaneError) Error() string {
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.Message)
}

func (s *Server) AutoCompleteByPane(input AutoCompleteByPaneInput) (AutoCompleteByPaneResult, *AutoCompleteByPaneError) {
	paneTarget := strings.TrimSpace(input.PaneTarget)
	if paneTarget == "" {
		return AutoCompleteByPaneResult{}, &AutoCompleteByPaneError{
			HTTPStatus: http.StatusBadRequest,
			Code:       "INVALID_PANE_TARGET",
			Message:    "pane_target is required",
		}
	}
	triggerSource := strings.TrimSpace(input.TriggerSource)
	callerPath := strings.TrimSpace(input.CallerPath)
	if callerPath == "" {
		callerPath = "internal:auto-progress"
	}
	callerActivePane := strings.TrimSpace(input.CallerActivePane)
	currentServerInstanceID := detectServerInstanceID()
	slog.Info(
		"run auto-complete lookup start",
		"pane_target", paneTarget,
		"trigger_source", triggerSource,
		"current_server_instance_id", currentServerInstanceID,
		"caller_path", callerPath,
		"caller_active_pane_target", callerActivePane,
	)
	projectID, store, taskEntry, foundTask, taskDiag, err := s.findTaskByPaneTarget(paneTarget)
	if err != nil {
		slog.Error(
			"run auto-complete task lookup failed",
			"pane_target", paneTarget,
			"err", err,
			"projects_scanned", taskDiag.ProjectsScanned,
		)
		return AutoCompleteByPaneResult{}, &AutoCompleteByPaneError{
			HTTPStatus: http.StatusInternalServerError,
			Code:       "TASK_LOOKUP_FAILED",
			Message:    err.Error(),
		}
	}
	if !foundTask {
		slog.Error(
			"run auto-complete task lookup miss",
			"pane_target", paneTarget,
			"projects_scanned", taskDiag.ProjectsScanned,
			"bindings_scanned", taskDiag.BindingsScanned,
			"bindings_with_pane_ref", taskDiag.BindingsWithPaneRef,
			"candidate_total", taskDiag.CandidateTotal,
			"candidate_with_task_record", taskDiag.CandidateWithTaskRecord,
			"samples", taskDiag.Samples,
			"pane_samples", taskDiag.PaneSamples,
		)
		return AutoCompleteByPaneResult{
			Triggered:  false,
			PaneTarget: paneTarget,
			Reason:     "no-task-pane-binding",
			RunID:      "",
			TaskID:     "",
			Status:     "skipped",
		}, nil
	}
	taskID := strings.TrimSpace(taskEntry.TaskID)
	if taskID == "" {
		return AutoCompleteByPaneResult{}, &AutoCompleteByPaneError{
			HTTPStatus: http.StatusInternalServerError,
			Code:       "TASK_LOOKUP_FAILED",
			Message:    "task id is empty",
		}
	}
	slog.Info(
		"run auto-complete task lookup hit",
		"pane_target", paneTarget,
		"project_id", projectID,
		"task_id", taskID,
		"task_status", strings.TrimSpace(taskEntry.Status),
		"projects_scanned", taskDiag.ProjectsScanned,
		"candidate_total", taskDiag.CandidateTotal,
	)
	if strings.EqualFold(triggerSource, "pane-actor") {
		autopilotEnabled := false
		if s.taskAgentSupervisor != nil {
			autopilotEnabled = s.taskAgentSupervisor.GetAutopilot(taskID)
		}
		if !autopilotEnabled {
			return AutoCompleteByPaneResult{
				Triggered:  false,
				PaneTarget: paneTarget,
				Reason:     "autopilot-disabled",
				RunID:      "",
				TaskID:     taskID,
				Status:     "skipped",
			}, nil
		}
	}
	if strings.EqualFold(triggerSource, "pane-actor") {
		observedSec := input.ObservedLastActiveAt
		if observedSec < 0 {
			return AutoCompleteByPaneResult{}, &AutoCompleteByPaneError{
				HTTPStatus: http.StatusBadRequest,
				Code:       "INVALID_OBSERVED_LAST_ACTIVE_AT",
				Message:    "observed_last_active_at must be unix timestamp in seconds",
			}
		}
		if observedSec > 0 {
			accepted, markErr := store.TryMarkTaskAutoProgressObserved(projectstate.TaskRecord{
				TaskID:       taskID,
				ProjectID:    projectID,
				ParentTaskID: taskEntry.ParentTaskID,
				Title:        taskEntry.Title,
				Status:       taskEntry.Status,
			}, observedSec)
			if markErr != nil {
				return AutoCompleteByPaneResult{}, &AutoCompleteByPaneError{
					HTTPStatus: http.StatusInternalServerError,
					Code:       "TASK_AUTOPROGRESS_MARK_FAILED",
					Message:    markErr.Error(),
				}
			}
			if !accepted {
				return AutoCompleteByPaneResult{
					Triggered:  false,
					PaneTarget: paneTarget,
					Reason:     "duplicate-observed-last-active-at",
					RunID:      "",
					TaskID:     taskID,
					Status:     "skipped",
				}, nil
			}
		}
	}
	run, foundRun, runDiag, err := s.findLiveRunningRunByPaneTargetForTask(store, paneTarget, taskID)
	if err != nil {
		slog.Error(
			"run auto-complete optional run lookup failed",
			"pane_target", paneTarget,
			"project_id", projectID,
			"task_id", taskID,
			"err", err,
		)
		return AutoCompleteByPaneResult{}, &AutoCompleteByPaneError{
			HTTPStatus: http.StatusInternalServerError,
			Code:       "RUN_LOOKUP_FAILED",
			Message:    err.Error(),
		}
	}
	slog.Info(
		"run auto-complete optional run lookup finished",
		"pane_target", paneTarget,
		"project_id", projectID,
		"task_id", taskID,
		"candidate_total", runDiag.CandidateTotal,
		"candidate_task_match", runDiag.CandidateTaskMatch,
		"candidate_running", runDiag.CandidateRunning,
		"candidate_live", runDiag.CandidateLive,
		"candidate_live_running", runDiag.CandidateLiveRunning,
		"candidate_server_match", runDiag.CandidateServerMatch,
		"candidate_server_diff", runDiag.CandidateServerDiff,
		"found_live_running_run", foundRun,
		"samples", runDiag.Samples,
	)
	summary := strings.TrimSpace(input.Summary)
	if summary == "" {
		summary = "auto-complete: pane idle and output stable"
	}
	reqMeta := copyTaskCompletionRequestMeta(input.RequestMeta)
	if foundRun {
		slog.Info(
			"run auto-complete proceeding with live running run",
			"pane_target", paneTarget,
			"project_id", projectID,
			"run_id", run.RunID,
			"task_id", run.TaskID,
			"run_status", run.RunStatus,
		)
		if invalidated, err := s.invalidateBindingIfServerChanged(store, run.RunID); err != nil {
			return AutoCompleteByPaneResult{}, &AutoCompleteByPaneError{
				HTTPStatus: http.StatusInternalServerError,
				Code:       "RUN_BINDING_CHECK_FAILED",
				Message:    err.Error(),
			}
		} else if invalidated {
			slog.Warn(
				"run auto-complete binding invalidated due to tmux server change",
				"pane_target", paneTarget,
				"project_id", projectID,
				"run_id", run.RunID,
				"task_id", run.TaskID,
			)
			return AutoCompleteByPaneResult{
				Triggered:  false,
				PaneTarget: paneTarget,
				Reason:     "binding-invalidated-server-changed",
				RunID:      run.RunID,
				TaskID:     run.TaskID,
				Status:     projectstate.RunStatusNeedsRebind,
			}, nil
		}
		_ = store.AppendRunEvent(run.RunID, "auto_complete.pane_idle", map[string]any{
			"pane_target": paneTarget,
			"trigger":     "status.ready",
		})
		if err := s.completeRunAndEnqueueActions(run.RunID, summary, "pane-idle", reqMeta); err != nil {
			return AutoCompleteByPaneResult{}, &AutoCompleteByPaneError{
				HTTPStatus: http.StatusInternalServerError,
				Code:       "RUN_COMPLETE_FAILED",
				Message:    err.Error(),
			}
		}
		return AutoCompleteByPaneResult{
			Triggered:   true,
			PaneTarget:  paneTarget,
			RunID:       run.RunID,
			TaskID:      run.TaskID,
			Status:      projectstate.RunStatusCompleted,
			SummaryUsed: summary,
		}, nil
	}
	if err := s.updateTaskStatusInternal(store, taskID, projectID, projectstate.StatusCompleted); err != nil {
		return AutoCompleteByPaneResult{}, &AutoCompleteByPaneError{
			HTTPStatus: http.StatusInternalServerError,
			Code:       "TASK_COMPLETE_FAILED",
			Message:    err.Error(),
		}
	}
	if err := s.writeTaskReturnSummary(projectID, taskID, summary); err != nil {
		return AutoCompleteByPaneResult{}, &AutoCompleteByPaneError{
			HTTPStatus: http.StatusInternalServerError,
			Code:       "TASK_RETURN_WRITE_FAILED",
			Message:    err.Error(),
		}
	}
	s.enqueueTaskCompletionActions(projectID, taskID, summary, "pane-idle", reqMeta)
	s.publishEvent("task.status.updated", projectID, taskID, map[string]any{"status": projectstate.StatusCompleted})
	s.publishEvent("task.return.reported", projectID, taskID, map[string]any{"summary": summary, "run_id": ""})
	s.publishEvent("task.tree.updated", projectID, taskID, map[string]any{})
	return AutoCompleteByPaneResult{
		Triggered:   true,
		PaneTarget:  paneTarget,
		RunID:       "",
		TaskID:      taskID,
		Status:      projectstate.StatusCompleted,
		SummaryUsed: summary,
	}, nil
}

func copyTaskCompletionRequestMeta(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}
