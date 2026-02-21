package localapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"shellman/cli/internal/projectstate"
)

var numberRegexp = regexp.MustCompile(`\d+`)

var validTaskFlag = map[string]struct{}{
	"success": {},
	"notify":  {},
	"error":   {},
}

type completionDispatchDecision struct {
	Dispatch         bool
	Reason           string
	NotifyEnabled    bool
	NotifyCommandSet bool
}

func (s *Server) evaluateTaskCompletionDispatch() completionDispatchDecision {
	decision := completionDispatchDecision{}
	cfg, err := s.deps.ConfigStore.LoadOrInit()
	if err != nil {
		decision.Reason = "config-load-failed"
		return decision
	}
	command := strings.TrimSpace(cfg.TaskCompletion.NotifyCommand)
	decision.NotifyEnabled = cfg.TaskCompletion.NotifyEnabled
	decision.NotifyCommandSet = command != ""
	if decision.NotifyEnabled && decision.NotifyCommandSet {
		decision.Dispatch = true
		decision.Reason = "notify-command-enabled"
		return decision
	}
	decision.Reason = "no-enabled-actions"
	return decision
}

func (s *Server) shouldDispatchTaskCompletionActions() bool {
	return s.evaluateTaskCompletionDispatch().Dispatch
}

func (s *Server) completeRunAndEnqueueActions(runID, summary, source string, reqMeta map[string]any) error {
	projectID, store, run, err := s.findRun(runID)
	if err != nil {
		return err
	}
	if err := store.MarkRunCompleted(runID); err != nil {
		return err
	}
	_, taskStore, _, err := s.findTask(run.TaskID)
	if err != nil {
		return err
	}
	if err := s.updateTaskStatusInternal(taskStore, run.TaskID, projectID, projectstate.StatusCompleted); err != nil {
		return err
	}
	if err := s.writeTaskReturnSummary(projectID, run.TaskID, summary); err != nil {
		return err
	}

	if err := store.EnqueueRunAction(runID, "run_completion_dispatch", map[string]any{
		"task_id": run.TaskID,
		"summary": strings.TrimSpace(summary),
		"source":  strings.TrimSpace(source),
	}); err != nil {
		return err
	}
	s.enqueueRunCompletionActions(runID, projectID, run.TaskID, summary, source, reqMeta)
	s.publishEvent("task.status.updated", projectID, run.TaskID, map[string]any{"status": projectstate.StatusCompleted})
	s.publishEvent("task.return.reported", projectID, run.TaskID, map[string]any{"summary": summary, "run_id": runID})
	s.publishEvent("task.tree.updated", projectID, run.TaskID, map[string]any{})
	return nil
}

func (s *Server) writeTaskReturnSummary(projectID, taskID, summary string) error {
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		return err
	}
	returnsDir := filepath.Join(repoRoot, ".shellman", "returns")
	if err := os.MkdirAll(returnsDir, 0o755); err != nil {
		return err
	}
	returnPath := filepath.Join(returnsDir, fmt.Sprintf("%s.return.md", taskID))
	return os.WriteFile(returnPath, []byte(summary+"\n"), 0o644)
}

func (s *Server) enqueueTaskCompletionActions(projectID, taskID, summary, source string, reqMeta map[string]any) {
	if strings.EqualFold(strings.TrimSpace(source), "pane-idle") {
		promptInput := s.buildAutoProgressPromptInput(projectID, taskID, summary, "")
		repoRoot, repoErr := s.findProjectRepoRoot(projectID)
		if repoErr == nil {
			s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "trigger.received", taskCompletionAuditFields(map[string]any{
				"source":       "pane-idle",
				"will_enqueue": true,
				"target":       "task-agent-loop",
			}, reqMeta))
		}
		prompt := buildTaskAgentAutoProgressPrompt(promptInput)
		displayContent := buildTaskAgentAutoProgressDisplayContent(taskID, promptInput.Summary, "")
		if err := s.sendTaskAgentLoop(context.Background(), TaskAgentLoopEvent{
			TaskID:         strings.TrimSpace(taskID),
			ProjectID:      strings.TrimSpace(projectID),
			Source:         "tty_output",
			DisplayContent: displayContent,
			AgentPrompt:    prompt,
			TriggerMeta:    reqMeta,
		}); err != nil {
			if repoErr == nil {
				s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "trigger.skipped", taskCompletionAuditFields(map[string]any{
					"source": "pane-idle",
					"reason": "agent-loop-enqueue-failed",
					"error":  err.Error(),
				}, reqMeta))
			}
			return
		}
		if repoErr == nil {
			s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "trigger.enqueued", taskCompletionAuditFields(map[string]any{
				"source": "pane-idle",
				"status": "queued",
			}, reqMeta))
		}
		return
	}
	decision := s.evaluateTaskCompletionDispatch()
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err == nil {
		fields := map[string]any{
			"source":             strings.TrimSpace(source),
			"will_dispatch":      decision.Dispatch,
			"reason":             decision.Reason,
			"notify_enabled":     decision.NotifyEnabled,
			"notify_command_set": decision.NotifyCommandSet,
		}
		for k, v := range reqMeta {
			fields[k] = v
		}
		s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "trigger.received", fields)
		if !decision.Dispatch {
			s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "trigger.skipped", map[string]any{
				"source": strings.TrimSpace(source),
				"reason": decision.Reason,
			})
		}
	}
	if !decision.Dispatch {
		return
	}
	go s.dispatchTaskCompletionActions(projectID, taskID, summary)
}

func (s *Server) enqueueRunCompletionActions(runID, projectID, taskID, summary, source string, reqMeta map[string]any) {
	if strings.EqualFold(strings.TrimSpace(source), "pane-idle") {
		promptInput := s.buildAutoProgressPromptInput(projectID, taskID, summary, runID)
		repoRoot, repoErr := s.findProjectRepoRoot(projectID)
		if repoErr == nil {
			s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "trigger.received", taskCompletionAuditFields(map[string]any{
				"run_id":       strings.TrimSpace(runID),
				"source":       "pane-idle",
				"will_enqueue": true,
				"target":       "task-agent-loop",
			}, reqMeta))
		}
		prompt := buildTaskAgentAutoProgressPrompt(promptInput)
		displayContent := buildTaskAgentAutoProgressDisplayContent(taskID, promptInput.Summary, runID)
		if err := s.sendTaskAgentLoop(context.Background(), TaskAgentLoopEvent{
			TaskID:         strings.TrimSpace(taskID),
			ProjectID:      strings.TrimSpace(projectID),
			Source:         "tty_output",
			DisplayContent: displayContent,
			AgentPrompt:    prompt,
			TriggerMeta:    reqMeta,
		}); err != nil {
			if repoErr == nil {
				s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "trigger.skipped", taskCompletionAuditFields(map[string]any{
					"run_id": strings.TrimSpace(runID),
					"source": "pane-idle",
					"reason": "agent-loop-enqueue-failed",
					"error":  err.Error(),
				}, reqMeta))
			}
			return
		}
		if repoErr == nil {
			s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "trigger.enqueued", taskCompletionAuditFields(map[string]any{
				"run_id": strings.TrimSpace(runID),
				"source": "pane-idle",
				"status": "queued",
			}, reqMeta))
		}
		return
	}
	decision := s.evaluateTaskCompletionDispatch()
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err == nil {
		fields := map[string]any{
			"run_id":             strings.TrimSpace(runID),
			"source":             strings.TrimSpace(source),
			"will_dispatch":      decision.Dispatch,
			"reason":             decision.Reason,
			"notify_enabled":     decision.NotifyEnabled,
			"notify_command_set": decision.NotifyCommandSet,
		}
		for k, v := range reqMeta {
			fields[k] = v
		}
		s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "trigger.received", fields)
		if !decision.Dispatch {
			s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "trigger.skipped", map[string]any{
				"run_id": strings.TrimSpace(runID),
				"source": strings.TrimSpace(source),
				"reason": decision.Reason,
			})
		}
	}
	if !decision.Dispatch {
		return
	}
	s.dispatchRunCompletionActions(runID, projectID, taskID, summary)
}

func (s *Server) dispatchTaskCompletionActions(projectID, taskID, summary string) {
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		return
	}
	s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "start", map[string]any{
		"summary_len": len(strings.TrimSpace(summary)),
	})

	cfg, err := s.deps.ConfigStore.LoadOrInit()
	if err != nil {
		s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "command.config.error", map[string]any{"error": err.Error()})
	} else {
		command := strings.TrimSpace(cfg.TaskCompletion.NotifyCommand)
		if cfg.TaskCompletion.NotifyEnabled && command != "" {
			if cfg.TaskCompletion.NotifyIdleDuration > 0 {
				s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "command.idle.ignored", map[string]any{
					"idle_threshold": cfg.TaskCompletion.NotifyIdleDuration,
				})
			}
			now := time.Now().UTC()
			payload := map[string]string{
				"task_id":      taskID,
				"project_id":   projectID,
				"status":       "completed",
				"summary":      strings.TrimSpace(summary),
				"finished_at":  strconv.FormatInt(now.Unix(), 10),
				"idle_seconds": strconv.Itoa(cfg.TaskCompletion.NotifyIdleDuration),
			}
			if err := runTaskCompletionCommand(taskID, command, payload); err != nil {
				s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "command.error", map[string]any{
					"error":   err.Error(),
					"command": command,
				})
			} else {
				s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "command.done", map[string]any{
					"command": command,
				})
			}
		}
	}

	s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "finish", nil)
}

func (s *Server) dispatchRunCompletionActions(runID, projectID, taskID, summary string) {
	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		return
	}
	s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "start", map[string]any{
		"run_id":      strings.TrimSpace(runID),
		"summary_len": len(strings.TrimSpace(summary)),
	})

	cfg, err := s.deps.ConfigStore.LoadOrInit()
	if err != nil {
		s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "command.config.error", map[string]any{"error": err.Error()})
	} else {
		command := strings.TrimSpace(cfg.TaskCompletion.NotifyCommand)
		if cfg.TaskCompletion.NotifyEnabled && command != "" {
			if cfg.TaskCompletion.NotifyIdleDuration > 0 {
				s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "command.idle.ignored", map[string]any{
					"idle_threshold": cfg.TaskCompletion.NotifyIdleDuration,
				})
			}
			now := time.Now().UTC()
			payload := map[string]string{
				"task_id":      taskID,
				"project_id":   projectID,
				"status":       "completed",
				"summary":      strings.TrimSpace(summary),
				"finished_at":  strconv.FormatInt(now.Unix(), 10),
				"idle_seconds": strconv.Itoa(cfg.TaskCompletion.NotifyIdleDuration),
			}
			if err := runTaskCompletionCommand(taskID, command, payload); err != nil {
				s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "command.error", map[string]any{
					"error":   err.Error(),
					"command": command,
				})
			} else {
				s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "command.done", map[string]any{
					"command": command,
				})
			}
		}
	}

	s.writeTaskCompletionAuditLog(repoRoot, projectID, taskID, "finish", map[string]any{
		"run_id": strings.TrimSpace(runID),
	})
}

func (s *Server) taskCompletionIdleSatisfied(threshold int) bool {
	if threshold <= 0 {
		return true
	}
	idle, err := estimateIdleSeconds()
	if err != nil {
		return true
	}
	return idle >= threshold
}

func runTaskCompletionCommand(taskID, command string, payload map[string]string) error {
	shell, shellArg, err := resolveShell()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, shellArg, command)
	cmd.Env = append(os.Environ(), "SHELLMAN_TASK_ID="+taskID)
	cmd.Env = append(cmd.Env, "SHELLMAN_TASK_COMPLETED_AT="+payload["finished_at"])
	cmd.Env = append(cmd.Env, "SHELLMAN_TASK_SUMMARY="+payload["summary"])
	cmd.Env = append(cmd.Env, "SHELLMAN_TASK_PROJECT_ID="+payload["project_id"])
	cmd.Env = append(cmd.Env, "SHELLMAN_TASK_COMPLETION_IDLE_SECONDS="+payload["idle_seconds"])
	_, err = cmd.CombinedOutput()
	return err
}

func (s *Server) buildAutoProgressPromptInput(projectID, taskID, summary, runID string) TaskAgentAutoProgressPromptInput {
	input := TaskAgentAutoProgressPromptInput{
		TaskID:  strings.TrimSpace(taskID),
		RunID:   strings.TrimSpace(runID),
		Summary: strings.TrimSpace(summary),
	}
	if input.Summary == "" {
		input.Summary = "auto-complete: pane idle and output stable"
	}

	resolvedProjectID, store, entry, err := s.findTask(taskID)
	if err != nil {
		return input
	}
	if strings.TrimSpace(projectID) == "" {
		projectID = resolvedProjectID
	}

	input.Name = strings.TrimSpace(entry.Title)
	input.Description = strings.TrimSpace(entry.Description)
	input.PrevFlag = strings.TrimSpace(entry.Flag)
	input.PrevStatusMessage = strings.TrimSpace(entry.FlagDesc)
	input.TTY = s.buildTaskTTYContext(store, entry, input.TaskID)
	input.ParentTask, input.ChildTasks = s.buildTaskFamilyContext(store, strings.TrimSpace(projectID), input.TaskID)
	return input
}

func (s *Server) buildUserPrompt(taskID, userInput string) string {
	projectID, store, entry, err := s.findTask(taskID)
	if err != nil {
		return buildTaskAgentUserPrompt(userInput, "", "", TaskAgentTTYContext{}, nil, nil)
	}
	tty := s.buildTaskTTYContext(store, entry, strings.TrimSpace(taskID))
	parent, children := s.buildTaskFamilyContext(store, strings.TrimSpace(projectID), strings.TrimSpace(taskID))
	return buildTaskAgentUserPrompt(userInput, strings.TrimSpace(entry.Flag), strings.TrimSpace(entry.FlagDesc), tty, parent, children)
}

func (s *Server) buildTaskTTYContext(store *projectstate.Store, entry projectstate.TaskIndexEntry, taskID string) TaskAgentTTYContext {
	tty := TaskAgentTTYContext{}
	if store == nil {
		return tty
	}
	panes, err := store.LoadPanes()
	if err != nil {
		return tty
	}
	binding, hasBinding := panes[strings.TrimSpace(taskID)]
	if !hasBinding {
		return tty
	}

	runtimePaneID := strings.TrimSpace(binding.PaneID)
	if runtimePaneID == "" {
		runtimePaneID = strings.TrimSpace(binding.PaneTarget)
	}
	if runtimePaneID == "" {
		runtimePaneID = strings.TrimSpace(binding.PaneUUID)
	}
	if runtimePaneID != "" {
		runtimeRow, ok, runtimeErr := store.GetPaneRuntimeByPaneID(runtimePaneID)
		if runtimeErr == nil && ok {
			if strings.TrimSpace(tty.CurrentCommand) == "" {
				tty.CurrentCommand = strings.TrimSpace(runtimeRow.CurrentCommand)
			}
			tty.OutputTail = tailText(strings.TrimSpace(runtimeRow.Snapshot), 4000)
		}
	}
	if strings.TrimSpace(tty.CurrentCommand) == "" {
		tty.CurrentCommand = strings.TrimSpace(s.detectPaneCurrentCommand(binding.PaneTarget))
	}
	if strings.TrimSpace(tty.CurrentCommand) == "" {
		tty.CurrentCommand = strings.TrimSpace(binding.PaneTarget)
	}
	tty.Cwd = strings.TrimSpace(s.detectPaneCurrentPath(binding.PaneTarget))
	return tty
}

func (s *Server) buildTaskFamilyContext(store *projectstate.Store, projectID, taskID string) (*TaskAgentParentContext, []TaskAgentChildContext) {
	if store == nil || strings.TrimSpace(projectID) == "" || strings.TrimSpace(taskID) == "" {
		return nil, []TaskAgentChildContext{}
	}
	rows, err := store.ListTasksByProject(strings.TrimSpace(projectID))
	if err != nil {
		return nil, []TaskAgentChildContext{}
	}
	parentID := ""
	for _, row := range rows {
		if strings.TrimSpace(row.TaskID) == strings.TrimSpace(taskID) {
			parentID = strings.TrimSpace(row.ParentTaskID)
			break
		}
	}
	var parent *TaskAgentParentContext
	if parentID != "" {
		for _, row := range rows {
			if strings.TrimSpace(row.TaskID) != parentID {
				continue
			}
			parent = &TaskAgentParentContext{
				Name:          strings.TrimSpace(row.Title),
				Description:   strings.TrimSpace(row.Description),
				Flag:          strings.TrimSpace(row.Flag),
				StatusMessage: strings.TrimSpace(row.FlagDesc),
			}
			break
		}
	}
	children := make([]TaskAgentChildContext, 0)
	for _, row := range rows {
		if strings.TrimSpace(row.ParentTaskID) != strings.TrimSpace(taskID) {
			continue
		}
		children = append(children, TaskAgentChildContext{
			TaskID:        strings.TrimSpace(row.TaskID),
			Name:          strings.TrimSpace(row.Title),
			Description:   strings.TrimSpace(row.Description),
			Flag:          strings.TrimSpace(row.Flag),
			StatusMessage: strings.TrimSpace(row.FlagDesc),
			ReportMessage: "",
		})
	}
	return parent, children
}

func (s *Server) detectPaneCurrentCommand(paneTarget string) string {
	target := strings.TrimSpace(paneTarget)
	if target == "" {
		return ""
	}
	execute := s.deps.ExecuteCommand
	if execute == nil {
		execute = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	args := []string{}
	if socket := strings.TrimSpace(os.Getenv("SHELLMAN_TMUX_SOCKET")); socket != "" {
		args = append(args, "-L", socket)
	}
	args = append(args, "display-message", "-p", "-t", target, "#{pane_current_command}")
	out, err := execute(ctx, "tmux", args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (s *Server) detectPaneCurrentPath(paneTarget string) string {
	target := strings.TrimSpace(paneTarget)
	if target == "" {
		return ""
	}
	execute := s.deps.ExecuteCommand
	if execute == nil {
		execute = func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	args := []string{}
	if socket := strings.TrimSpace(os.Getenv("SHELLMAN_TMUX_SOCKET")); socket != "" {
		args = append(args, "-L", socket)
	}
	args = append(args, "display-message", "-p", "-t", target, "#{pane_current_path}")
	out, err := execute(ctx, "tmux", args...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func tailText(input string, max int) string {
	if max <= 0 || len(input) <= max {
		return input
	}
	return input[len(input)-max:]
}

func (s *Server) setTaskFlagInternal(store *projectstate.Store, projectID, taskID, flag, flagDesc string) error {
	if flag != "" {
		if _, ok := validTaskFlag[flag]; !ok {
			return errors.New("unsupported task flag")
		}
	}
	_, ok, err := findTaskEntryInProject(store, projectID, taskID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("task not found")
	}
	nextFlag := strings.TrimSpace(flag)
	nextFlagDesc := strings.TrimSpace(flagDesc)
	nextFlagReaded := false
	if err := store.UpsertTaskMeta(projectstate.TaskMetaUpsert{
		TaskID:     taskID,
		ProjectID:  projectID,
		Flag:       &nextFlag,
		FlagDesc:   &nextFlagDesc,
		FlagReaded: &nextFlagReaded,
	}); err != nil {
		return err
	}
	s.publishEvent("task.flag.updated", projectID, taskID, map[string]any{
		"flag":      nextFlag,
		"flag_desc": nextFlagDesc,
	})
	s.publishEvent("task.tree.updated", projectID, taskID, map[string]any{})
	return nil
}

func (s *Server) writeTaskCompletionAuditLog(repoRoot, projectID, taskID, stage string, extra map[string]any) {
	logger := newTaskCompletionAuditLogger(repoRoot)
	if logger == nil {
		return
	}
	defer logger.Close()

	fields := map[string]any{
		"ts":         time.Now().UTC().Unix(),
		"project_id": strings.TrimSpace(projectID),
		"task_id":    strings.TrimSpace(taskID),
	}
	for k, v := range extra {
		fields[k] = v
	}
	logger.Log(stage, fields)
}

func taskCompletionAuditFields(base map[string]any, reqMeta map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	for k, v := range reqMeta {
		base[k] = v
	}
	return base
}

func resolveShell() (string, string, error) {
	switch runtime.GOOS {
	case "windows":
		return "cmd", "/C", nil
	case "darwin", "linux", "freebsd", "openbsd", "netbsd":
		return "sh", "-c", nil
	default:
		return "", "", errors.New("unsupported shell")
	}
}

func estimateIdleSeconds() (int, error) {
	switch runtime.GOOS {
	case "darwin":
		return estimateIdleSecondsDarwin()
	case "linux":
		return estimateIdleSecondsLinux()
	default:
		return 0, errors.New("idle detection not supported")
	}
}

func estimateIdleSecondsDarwin() (int, error) {
	out, err := exec.Command("ioreg", "-w0", "-c", "IOHIDSystem").Output()
	if err != nil {
		return 0, err
	}
	nanos, err := parseFirstNumber(string(out))
	if err != nil {
		return 0, err
	}
	return nanos / 1000000000, nil
}

func estimateIdleSecondsLinux() (int, error) {
	out, err := exec.Command("xprintidle").Output()
	if err != nil {
		return 0, err
	}
	millis, err := parseFirstNumber(string(out))
	if err != nil {
		return 0, err
	}
	return millis / 1000, nil
}

func parseFirstNumber(text string) (int, error) {
	matches := numberRegexp.FindString(text)
	if matches == "" {
		return 0, errors.New("no number in output")
	}
	v, err := strconv.Atoi(matches)
	if err != nil {
		return 0, err
	}
	return v, nil
}
