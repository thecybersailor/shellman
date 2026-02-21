package localapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"termteam/cli/internal/agentloop"
	"termteam/cli/internal/projectstate"
)

const taskAgentLoopQueueSize = 64

var ErrTaskAgentLoopUnavailable = errors.New("agent loop runner is unavailable")

type TaskAgentLoopEvent struct {
	TaskID         string
	ProjectID      string
	Source         string
	DisplayContent string
	AgentPrompt    string
	TriggerMeta    map[string]any
	SessionConfig  *TaskAgentSessionConfig
}

type TaskAgentSessionConfig struct {
	ResponsesStore      bool
	DisableStoreContext bool
}

type taskAgentLoopSupervisor struct {
	logger *slog.Logger

	mu     sync.Mutex
	actors map[string]*taskAgentLoopActor

	handler func(context.Context, TaskAgentLoopEvent) error
}

type taskAgentLoopActor struct {
	taskID        string
	queue         chan TaskAgentLoopEvent
	handler       func(context.Context, TaskAgentLoopEvent) error
	logger        *slog.Logger
	sessionConfig TaskAgentSessionConfig
	autopilot     bool
}

func newTaskAgentLoopSupervisor(
	logger *slog.Logger,
	handler func(context.Context, TaskAgentLoopEvent) error,
) *taskAgentLoopSupervisor {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return &taskAgentLoopSupervisor{
		logger:  logger,
		actors:  map[string]*taskAgentLoopActor{},
		handler: handler,
	}
}

func (s *taskAgentLoopSupervisor) Enqueue(ctx context.Context, evt TaskAgentLoopEvent) error {
	if s == nil {
		return errors.New("task agent loop supervisor is unavailable")
	}
	taskID := strings.TrimSpace(evt.TaskID)
	if taskID == "" {
		return errors.New("task_id is required")
	}
	actor := s.getOrCreateActor(taskID)
	if s.logger != nil {
		s.logger.Info("task agent loop enqueue attempt",
			"module", "task_agent_actor",
			"task_id", taskID,
			"source", strings.TrimSpace(evt.Source),
			"queue_len", len(actor.queue),
			"queue_cap", cap(actor.queue),
		)
	}
	select {
	case <-ctx.Done():
		if s.logger != nil {
			s.logger.Warn("task agent loop enqueue canceled",
				"module", "task_agent_actor",
				"task_id", taskID,
				"source", strings.TrimSpace(evt.Source),
				"err", ctx.Err(),
			)
		}
		return ctx.Err()
	case actor.queue <- evt:
		if s.logger != nil {
			s.logger.Info("task agent loop enqueued",
				"module", "task_agent_actor",
				"task_id", taskID,
				"source", strings.TrimSpace(evt.Source),
				"queue_len", len(actor.queue),
				"queue_cap", cap(actor.queue),
			)
		}
		return nil
	}
}

func (s *taskAgentLoopSupervisor) getOrCreateActor(taskID string) *taskAgentLoopActor {
	s.mu.Lock()
	defer s.mu.Unlock()
	if actor, ok := s.actors[taskID]; ok {
		return actor
	}
	actor := &taskAgentLoopActor{
		taskID:  taskID,
		queue:   make(chan TaskAgentLoopEvent, taskAgentLoopQueueSize),
		handler: s.handler,
		logger:  s.logger.With("module", "task_agent_actor", "task_id", taskID),
		sessionConfig: TaskAgentSessionConfig{
			ResponsesStore:      false,
			DisableStoreContext: true,
		},
		autopilot: false,
	}
	actor.start()
	s.actors[taskID] = actor
	return actor
}

func (s *taskAgentLoopSupervisor) SetAutopilot(taskID string, enabled bool) error {
	if s == nil {
		return errors.New("task agent loop supervisor is unavailable")
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("task_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	actor, ok := s.actors[taskID]
	if !ok {
		actor = &taskAgentLoopActor{
			taskID:  taskID,
			queue:   make(chan TaskAgentLoopEvent, taskAgentLoopQueueSize),
			handler: s.handler,
			logger:  s.logger.With("module", "task_agent_actor", "task_id", taskID),
			sessionConfig: TaskAgentSessionConfig{
				ResponsesStore:      false,
				DisableStoreContext: true,
			},
			autopilot: false,
		}
		actor.start()
		s.actors[taskID] = actor
	}
	actor.autopilot = enabled
	return nil
}

func (s *taskAgentLoopSupervisor) GetAutopilot(taskID string) bool {
	if s == nil {
		return false
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	actor, ok := s.actors[taskID]
	if !ok {
		return false
	}
	return actor.autopilot
}

func (a *taskAgentLoopActor) start() {
	go func() {
		for evt := range a.queue {
			startAt := time.Now()
			a.logger.Info("task agent loop dequeued",
				"source", strings.TrimSpace(evt.Source),
				"queue_len", len(a.queue),
				"queue_cap", cap(a.queue),
			)
			if a.handler == nil {
				a.logger.Warn("task agent loop handler missing",
					"source", strings.TrimSpace(evt.Source),
				)
				continue
			}
			if evt.SessionConfig == nil {
				cfg := a.sessionConfig
				evt.SessionConfig = &cfg
			}
			a.logger.Info("task agent loop processing",
				"source", strings.TrimSpace(evt.Source),
				"responses_store", evt.SessionConfig != nil && evt.SessionConfig.ResponsesStore,
				"disable_store_context", evt.SessionConfig != nil && evt.SessionConfig.DisableStoreContext,
			)
			if err := a.handler(context.Background(), evt); err != nil {
				a.logger.Warn("task agent loop event failed", "source", strings.TrimSpace(evt.Source), "err", err)
				continue
			}
			a.logger.Info("task agent loop processed",
				"source", strings.TrimSpace(evt.Source),
				"duration_ms", time.Since(startAt).Milliseconds(),
			)
		}
	}()
}

func (s *Server) sendTaskAgentLoop(ctx context.Context, evt TaskAgentLoopEvent) error {
	if s == nil {
		return errors.New("server is nil")
	}
	if s.deps.AgentLoopRunner == nil {
		return ErrTaskAgentLoopUnavailable
	}
	if s.taskAgentSupervisor == nil {
		return errors.New("task agent loop supervisor is unavailable")
	}
	evt.TaskID = strings.TrimSpace(evt.TaskID)
	evt.ProjectID = strings.TrimSpace(evt.ProjectID)
	evt.Source = strings.TrimSpace(evt.Source)
	evt.DisplayContent = strings.TrimSpace(evt.DisplayContent)
	evt.AgentPrompt = strings.TrimSpace(evt.AgentPrompt)
	if evt.DisplayContent == "" {
		evt.DisplayContent = evt.AgentPrompt
	}
	if evt.AgentPrompt == "" {
		evt.AgentPrompt = evt.DisplayContent
	}
	return s.taskAgentSupervisor.Enqueue(ctx, evt)
}

func (s *Server) handleTaskAgentLoopEvent(ctx context.Context, evt TaskAgentLoopEvent) error {
	taskID := strings.TrimSpace(evt.TaskID)
	if taskID == "" {
		return errors.New("task_id is required")
	}
	projectID, store, _, err := s.findTask(taskID)
	if err != nil {
		return err
	}
	return s.runTaskAgentLoopEvent(ctx, projectID, store, TaskAgentLoopEvent{
		TaskID:         taskID,
		ProjectID:      strings.TrimSpace(projectID),
		Source:         strings.TrimSpace(evt.Source),
		DisplayContent: strings.TrimSpace(evt.DisplayContent),
		AgentPrompt:    strings.TrimSpace(evt.AgentPrompt),
		TriggerMeta:    evt.TriggerMeta,
	})
}

func (s *Server) runTaskAgentLoopEvent(ctx context.Context, projectID string, store *projectstate.Store, evt TaskAgentLoopEvent) error {
	taskID := strings.TrimSpace(evt.TaskID)
	displayContent := strings.TrimSpace(evt.DisplayContent)
	agentPrompt := strings.TrimSpace(evt.AgentPrompt)
	if taskID == "" {
		return errors.New("task_id is required")
	}
	if displayContent == "" {
		return errors.New("display_content is required")
	}
	if agentPrompt == "" {
		return errors.New("agent_prompt is required")
	}

	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		return err
	}
	logger := newTaskMessagesAuditLogger(repoRoot)
	defer logger.Close()

	userMessageID, err := store.InsertTaskMessage(taskID, "user", displayContent, "completed", "")
	if err != nil {
		return err
	}
	assistantMessageID, err := store.InsertTaskMessage(taskID, "assistant", "", "running", "")
	if err != nil {
		return err
	}
	logger.Log("task.message.send.started", map[string]any{
		"task_id":               taskID,
		"source":                strings.TrimSpace(evt.Source),
		"user_message_id":       userMessageID,
		"assistant_message_id":  assistantMessageID,
		"user_content_preview":  clipTaskAuditText(displayContent, 400),
		"agent_prompt_preview":  clipTaskAuditText(agentPrompt, 200),
		"agent_loop_configured": s.deps.AgentLoopRunner != nil,
	})
	s.publishEvent("task.messages.updated", projectID, taskID, map[string]any{})

	if s.deps.AgentLoopRunner == nil {
		errText := ErrTaskAgentLoopUnavailable.Error()
		_ = store.UpdateTaskMessage(assistantMessageID, "", "error", errText)
		logger.Log("task.message.send.failed", map[string]any{
			"task_id": taskID,
			"error":   errText,
			"stage":   "agent_loop_unavailable",
			"source":  strings.TrimSpace(evt.Source),
		})
		s.publishEvent("task.messages.updated", projectID, taskID, map[string]any{})
		return ErrTaskAgentLoopUnavailable
	}

	scopeCtx := agentloop.WithTaskScope(ctx, agentloop.TaskScope{
		TaskID:              taskID,
		ProjectID:           projectID,
		Source:              strings.TrimSpace(evt.Source),
		ResponsesStore:      evt.SessionConfig != nil && evt.SessionConfig.ResponsesStore,
		DisableStoreContext: evt.SessionConfig != nil && evt.SessionConfig.DisableStoreContext,
	})
	toolMode, currentCommand, allowedToolNames := resolveTaskAgentToolModeAndNames(store, projectID, taskID)
	if len(allowedToolNames) > 0 {
		scopeCtx = agentloop.WithAllowedToolNames(scopeCtx, allowedToolNames)
	}
	reply := ""
	runErr := error(nil)
	responsesStore := evt.SessionConfig != nil && evt.SessionConfig.ResponsesStore
	disableStoreContext := evt.SessionConfig != nil && evt.SessionConfig.DisableStoreContext
	if streamRunner, ok := s.deps.AgentLoopRunner.(agentLoopStreamingWithToolsRunner); ok {
		logger.Log("task.message.send.agentloop.invoke", map[string]any{
			"task_id":               taskID,
			"source":                strings.TrimSpace(evt.Source),
			"user_message_id":       userMessageID,
			"assistant_message_id":  assistantMessageID,
			"mode":                  "stream_with_tools",
			"tool_mode":             toolMode,
			"current_command":       currentCommand,
			"allowed_tools":         allowedToolNames,
			"responses_store":       responsesStore,
			"disable_store_context": disableStoreContext,
		})
		var (
			streamState   = assistantStructuredContent{Text: "", Tools: []map[string]any{}}
			lastPublishAt time.Time
			toolIndexByID = map[string]int{}
		)
		flushRunning := func(force bool) {
			now := time.Now()
			if !force && !lastPublishAt.IsZero() && now.Sub(lastPublishAt) < 120*time.Millisecond {
				return
			}
			next := marshalAssistantStructuredContent(streamState)
			if err := store.UpdateTaskMessage(assistantMessageID, next, "running", ""); err != nil {
				return
			}
			lastPublishAt = now
			s.publishEvent("task.messages.updated", projectID, taskID, map[string]any{})
		}
		reply, runErr = streamRunner.RunStreamWithTools(scopeCtx, agentPrompt, func(delta string) {
			if delta == "" {
				return
			}
			streamState.Text += delta
			flushRunning(false)
		}, func(event map[string]any) {
			if strings.TrimSpace(fmt.Sprint(event["type"])) == "agent-debug" {
				logger.Log("task.message.send.agentloop.debug", map[string]any{
					"task_id":              taskID,
					"source":               strings.TrimSpace(evt.Source),
					"user_message_id":      userMessageID,
					"assistant_message_id": assistantMessageID,
					"stage":                strings.TrimSpace(fmt.Sprint(event["stage"])),
					"iteration":            intFromAny(event["iteration"]),
					"request":              strings.TrimSpace(fmt.Sprint(event["request"])),
					"response_id":          strings.TrimSpace(fmt.Sprint(event["response_id"])),
					"tool_calls":           intFromAny(event["tool_calls"]),
					"tool_calls_summary":   strings.TrimSpace(fmt.Sprint(event["tool_calls_summary"])),
					"final_text_len":       intFromAny(event["final_text_len"]),
					"event_trace":          strings.TrimSpace(fmt.Sprint(event["event_trace"])),
					"event_count":          intFromAny(event["event_count"]),
					"previous_response":    strings.TrimSpace(fmt.Sprint(event["previous_response"])),
					"previous_response_id": strings.TrimSpace(fmt.Sprint(event["previous_response_id"])),
					"call_id":              strings.TrimSpace(fmt.Sprint(event["call_id"])),
					"tool_name":            strings.TrimSpace(fmt.Sprint(event["tool_name"])),
					"items_count":          intFromAny(event["items_count"]),
					"items_summary":        strings.TrimSpace(fmt.Sprint(event["items_summary"])),
					"output_len":           intFromAny(event["output_len"]),
					"function_output":      strings.TrimSpace(fmt.Sprint(event["function_output"])),
					"request_raw":          fmt.Sprint(event["request_raw"]),
					"response_raw":         fmt.Sprint(event["response_raw"]),
					"event_raw":            fmt.Sprint(event["event_raw"]),
				})
				return
			}
			callID := strings.TrimSpace(fmt.Sprint(event["call_id"]))
			responseID := strings.TrimSpace(fmt.Sprint(event["response_id"]))
			toolName := strings.TrimSpace(fmt.Sprint(event["tool_name"]))
			toolState := strings.TrimSpace(fmt.Sprint(event["state"]))
			next := map[string]any{
				"type":      event["type"],
				"tool_name": toolName,
				"state":     toolState,
			}
			hasInput := false
			if input, ok := event["input"]; ok {
				next["input"] = input
				hasInput = true
			}
			hasOutput := false
			if output, ok := event["output"]; ok {
				next["output"] = output
				hasOutput = true
			}
			hasErrorText := false
			errorPreview := ""
			if errText, ok := event["error_text"]; ok {
				next["error_text"] = errText
				hasErrorText = strings.TrimSpace(fmt.Sprint(errText)) != ""
				errorPreview = strings.TrimSpace(fmt.Sprint(errText))
			}
			inputPreview := strings.TrimSpace(fmt.Sprint(event["input_preview"]))
			outputLen := intFromAny(event["output_len"])
			outputPreview := strings.TrimSpace(fmt.Sprint(event["output"]))
			if idx, ok := toolIndexByID[callID]; ok && idx >= 0 && idx < len(streamState.Tools) {
				current := streamState.Tools[idx]
				for k, v := range next {
					if v != nil && fmt.Sprint(v) != "" {
						current[k] = v
					}
				}
				streamState.Tools[idx] = current
			} else {
				streamState.Tools = append(streamState.Tools, next)
				if callID != "" {
					toolIndexByID[callID] = len(streamState.Tools) - 1
				}
			}
			logger.Log("task.message.send.agentloop.tool.event", map[string]any{
				"task_id":         taskID,
				"source":          strings.TrimSpace(evt.Source),
				"user_message_id": userMessageID,
				"call_id":         callID,
				"response_id":     responseID,
				"tool_name":       toolName,
				"state":           toolState,
				"has_input":       hasInput,
				"input_raw_len":   intFromAny(event["input_raw_len"]),
				"has_output":      hasOutput,
				"has_error_text":  hasErrorText,
				"input_preview":   inputPreview,
				"output_len":      outputLen,
				"output_preview":  outputPreview,
				"error_preview":   errorPreview,
			})
			flushRunning(false)
		})
		if runErr != nil {
			partial := strings.TrimSpace(streamState.Text)
			next := marshalAssistantStructuredContent(streamState)
			if partial == "" {
				next = streamState.Text
			}
			_ = store.UpdateTaskMessage(assistantMessageID, next, "error", runErr.Error())
			logger.Log("task.message.send.failed", map[string]any{
				"task_id": taskID,
				"error":   runErr.Error(),
				"source":  strings.TrimSpace(evt.Source),
			})
			s.publishEvent("task.messages.updated", projectID, taskID, map[string]any{})
			return runErr
		}
		if strings.TrimSpace(reply) == "" {
			reply = streamState.Text
		}
		streamState.Text = strings.TrimSpace(reply)
		reply = marshalAssistantStructuredContent(streamState)
	} else if streamRunner, ok := s.deps.AgentLoopRunner.(agentLoopStreamingRunner); ok {
		logger.Log("task.message.send.agentloop.invoke", map[string]any{
			"task_id":               taskID,
			"source":                strings.TrimSpace(evt.Source),
			"user_message_id":       userMessageID,
			"assistant_message_id":  assistantMessageID,
			"mode":                  "stream",
			"tool_mode":             toolMode,
			"current_command":       currentCommand,
			"allowed_tools":         allowedToolNames,
			"responses_store":       responsesStore,
			"disable_store_context": disableStoreContext,
		})
		var (
			streamText    strings.Builder
			lastPublishAt time.Time
		)
		flushRunning := func(force bool) {
			now := time.Now()
			if !force && !lastPublishAt.IsZero() && now.Sub(lastPublishAt) < 120*time.Millisecond {
				return
			}
			next := marshalAssistantStructuredContent(assistantStructuredContent{
				Text: streamText.String(),
			})
			if err := store.UpdateTaskMessage(assistantMessageID, next, "running", ""); err != nil {
				return
			}
			lastPublishAt = now
			s.publishEvent("task.messages.updated", projectID, taskID, map[string]any{})
		}
		reply, runErr = streamRunner.RunStream(scopeCtx, agentPrompt, func(delta string) {
			if delta == "" {
				return
			}
			streamText.WriteString(delta)
			flushRunning(false)
		})
		if runErr != nil {
			partial := strings.TrimSpace(streamText.String())
			next := marshalAssistantStructuredContent(assistantStructuredContent{Text: partial})
			_ = store.UpdateTaskMessage(assistantMessageID, next, "error", runErr.Error())
			logger.Log("task.message.send.failed", map[string]any{
				"task_id": taskID,
				"error":   runErr.Error(),
				"source":  strings.TrimSpace(evt.Source),
			})
			s.publishEvent("task.messages.updated", projectID, taskID, map[string]any{})
			return runErr
		}
		if strings.TrimSpace(reply) == "" {
			reply = streamText.String()
		}
		reply = marshalAssistantStructuredContent(assistantStructuredContent{
			Text: strings.TrimSpace(reply),
		})
	} else {
		logger.Log("task.message.send.agentloop.invoke", map[string]any{
			"task_id":               taskID,
			"source":                strings.TrimSpace(evt.Source),
			"user_message_id":       userMessageID,
			"assistant_message_id":  assistantMessageID,
			"mode":                  "run",
			"tool_mode":             toolMode,
			"current_command":       currentCommand,
			"allowed_tools":         allowedToolNames,
			"responses_store":       responsesStore,
			"disable_store_context": disableStoreContext,
		})
		reply, runErr = s.deps.AgentLoopRunner.Run(scopeCtx, agentPrompt)
	}
	if runErr != nil {
		_ = store.UpdateTaskMessage(assistantMessageID, "", "error", runErr.Error())
		logger.Log("task.message.send.failed", map[string]any{
			"task_id": taskID,
			"error":   runErr.Error(),
			"source":  strings.TrimSpace(evt.Source),
		})
		s.publishEvent("task.messages.updated", projectID, taskID, map[string]any{})
		return runErr
	}
	if err := store.UpdateTaskMessage(assistantMessageID, strings.TrimSpace(reply), "completed", ""); err != nil {
		return err
	}
	logger.Log("task.message.send.completed", map[string]any{
		"task_id":              taskID,
		"source":               strings.TrimSpace(evt.Source),
		"user_message_id":      userMessageID,
		"assistant_message_id": assistantMessageID,
	})
	s.publishEvent("task.messages.updated", projectID, taskID, map[string]any{})
	return nil
}

type taskAgentToolMode string

const (
	taskAgentToolModeDefault taskAgentToolMode = "default"
	taskAgentToolModeShell   taskAgentToolMode = "shell"
	taskAgentToolModeAIAgent taskAgentToolMode = "ai_agent"
)

func resolveTaskAgentToolModeAndNames(store *projectstate.Store, projectID, taskID string) (string, string, []string) {
	mode := taskAgentToolModeDefault
	currentCommand := ""
	if store != nil && strings.TrimSpace(projectID) != "" && strings.TrimSpace(taskID) != "" {
		if rows, err := store.ListTasksByProject(strings.TrimSpace(projectID)); err == nil {
			targetTaskID := strings.TrimSpace(taskID)
			for _, row := range rows {
				if strings.TrimSpace(row.TaskID) == targetTaskID {
					currentCommand = strings.TrimSpace(row.CurrentCommand)
					break
				}
			}
		}
	}
	mode = resolveTaskAgentToolModeFromCommand(currentCommand)
	names := []string{
		"task.current.set_flag",
		"task.child.get_context",
		"task.child.get_tty_output",
		"task.child.spawn",
		"task.child.send_message",
		"task.parent.report",
	}
	switch mode {
	case taskAgentToolModeAIAgent:
		names = append(names, "task.input_prompt")
	case taskAgentToolModeShell:
		names = append(names, "exec_command", "write_stdin")
	default:
		names = append(names, "write_stdin")
	}
	return string(mode), currentCommand, names
}

func resolveTaskAgentToolModeFromCommand(command string) taskAgentToolMode {
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) == 0 {
		return taskAgentToolModeDefault
	}
	switch strings.ToLower(strings.TrimSpace(parts[0])) {
	case "codex", "claude", "cursor", "gemini":
		return taskAgentToolModeAIAgent
	case "bash", "zsh":
		return taskAgentToolModeShell
	default:
		return taskAgentToolModeShell
	}
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	}
	if n, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v))); err == nil {
		return n
	}
	return 0
}

func clipTaskAuditText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "...(truncated)"
}
