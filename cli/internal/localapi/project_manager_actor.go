package localapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/flaboy/agentloop"
	"shellman/cli/internal/agentloopadapter"
	"shellman/cli/internal/projectstate"
)

var ErrProjectManagerLoopUnavailable = errors.New("project manager loop runner is unavailable")

type PMAgentLoopEvent struct {
	SessionID      string
	ProjectID      string
	Source         string
	DisplayContent string
	AgentPrompt    string
	HistoryBlock   string
	TriggerMeta    map[string]any
}

type projectManagerLoopSupervisor struct {
	runtime *ConversationRuntime
	handler func(context.Context, PMAgentLoopEvent) error
}

func newProjectManagerLoopSupervisor(handler func(context.Context, PMAgentLoopEvent) error) *projectManagerLoopSupervisor {
	return &projectManagerLoopSupervisor{
		handler: handler,
		runtime: NewConversationRuntime(func(ctx context.Context, evt ConversationEvent) error {
			if handler == nil {
				return errors.New("project manager loop handler is unavailable")
			}
			payload, ok := evt.Payload.(PMAgentLoopEvent)
			if !ok {
				return errors.New("invalid project manager loop payload")
			}
			return handler(ctx, payload)
		}),
	}
}

func (s *projectManagerLoopSupervisor) Enqueue(ctx context.Context, evt PMAgentLoopEvent) error {
	if s == nil || s.runtime == nil {
		return errors.New("project manager loop supervisor is unavailable")
	}
	sessionID := strings.TrimSpace(evt.SessionID)
	if sessionID == "" {
		return errors.New("session_id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.runtime.Enqueue(ctx, ConversationEvent{Key: sessionID, Payload: evt})
}

func (s *Server) sendProjectManagerLoop(ctx context.Context, evt PMAgentLoopEvent) error {
	if s == nil {
		return errors.New("server is nil")
	}
	if s.deps.AgentLoopRunner == nil {
		return ErrProjectManagerLoopUnavailable
	}
	if s.pmAgentSupervisor == nil {
		return errors.New("project manager loop supervisor is unavailable")
	}
	evt.SessionID = strings.TrimSpace(evt.SessionID)
	evt.ProjectID = strings.TrimSpace(evt.ProjectID)
	evt.Source = strings.TrimSpace(evt.Source)
	evt.DisplayContent = strings.TrimSpace(evt.DisplayContent)
	evt.AgentPrompt = strings.TrimSpace(evt.AgentPrompt)
	evt.HistoryBlock = strings.TrimSpace(evt.HistoryBlock)
	if evt.DisplayContent == "" {
		evt.DisplayContent = evt.AgentPrompt
	}
	if evt.AgentPrompt == "" {
		evt.AgentPrompt = evt.DisplayContent
	}
	return s.pmAgentSupervisor.Enqueue(ctx, evt)
}

func (s *Server) handleProjectManagerLoopEvent(ctx context.Context, evt PMAgentLoopEvent) error {
	sessionID := strings.TrimSpace(evt.SessionID)
	projectID := strings.TrimSpace(evt.ProjectID)
	if sessionID == "" {
		return errors.New("session_id is required")
	}
	if projectID == "" {
		return errors.New("project_id is required")
	}

	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		return err
	}
	store := projectstate.NewStore(repoRoot)
	session, ok, err := store.GetPMSession(sessionID)
	if err != nil {
		return err
	}
	if !ok || strings.TrimSpace(session.ProjectID) != projectID {
		return errors.New("project manager session not found")
	}

	return s.runProjectManagerLoopEventHybrid(ctx, store, evt)
}

func (s *Server) runProjectManagerLoopEvent(ctx context.Context, store *projectstate.Store, evt PMAgentLoopEvent) error {
	sessionID := strings.TrimSpace(evt.SessionID)
	projectID := strings.TrimSpace(evt.ProjectID)
	source := strings.TrimSpace(evt.Source)
	displayContent := strings.TrimSpace(evt.DisplayContent)
	agentPrompt := strings.TrimSpace(evt.AgentPrompt)
	if displayContent == "" {
		return errors.New("display_content is required")
	}
	if agentPrompt == "" {
		return errors.New("agent_prompt is required")
	}

	logger := newPMMessagesAuditLogger()
	defer logger.Close()

	if _, err := store.InsertPMMessage(sessionID, "user", displayContent, projectstate.StatusCompleted, ""); err != nil {
		return err
	}
	assistantMessageID, err := store.InsertPMMessage(sessionID, "assistant", "", projectstate.StatusRunning, "")
	if err != nil {
		return err
	}
	startedFields := map[string]any{
		"project_id":           projectID,
		"session_id":           sessionID,
		"source":               source,
		"assistant_message_id": assistantMessageID,
		"user_content_preview": clipTaskAuditText(displayContent, 400),
		"agent_prompt_preview": clipTaskAuditText(agentPrompt, 200),
	}
	for k, v := range evt.TriggerMeta {
		startedFields[k] = v
	}
	logger.Log("pm.message.send.started", startedFields)
	s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source})
	runCtx := agentloopadapter.WithPMScope(ctx, agentloopadapter.PMScope{
		SessionID: sessionID,
		ProjectID: projectID,
		Source:    source,
	})
	runCtx = agentloopadapter.WithAllowedToolNamesResolver(runCtx, func() []string {
		return s.resolveProjectManagerAllowedToolNames(projectID, sessionID, source)
	})

	reply := ""
	runErr := error(nil)
	if streamRunner, ok := s.deps.AgentLoopRunner.(agentLoopStreamingWithToolsRunner); ok {
		invokeFields := map[string]any{
			"project_id":           projectID,
			"session_id":           sessionID,
			"source":               source,
			"assistant_message_id": assistantMessageID,
			"mode":                 "stream_with_tools",
		}
		for k, v := range evt.TriggerMeta {
			invokeFields[k] = v
		}
		logger.Log("pm.message.send.agentloop.invoke", invokeFields)
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
			if err := store.UpdatePMMessage(assistantMessageID, next, projectstate.StatusRunning, ""); err != nil {
				return
			}
			lastPublishAt = now
			s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source})
		}
		reply, runErr = streamRunner.RunStreamWithTools(runCtx, agentPrompt, func(delta string) {
			if delta == "" {
				return
			}
			streamState.Text += delta
			flushRunning(false)
		}, func(event map[string]any) {
			toolEvent, ok := agentloopadapter.ParseLegacyToolEvent(event)
			if !ok {
				return
			}
			callID := strings.TrimSpace(toolEvent.CallID)
			next := toolEvent.ToToolStatePatch()
			if idx, ok := toolIndexByID[callID]; ok && idx >= 0 && idx < len(streamState.Tools) {
				streamState.Tools[idx] = agentloopadapter.MergeToolStatePatch(streamState.Tools[idx], next)
			} else {
				streamState.Tools = append(streamState.Tools, next)
				if callID != "" {
					toolIndexByID[callID] = len(streamState.Tools) - 1
				}
			}
			logger.Log("pm.message.send.agentloop.tool.event", map[string]any{
				"project_id":     projectID,
				"session_id":     sessionID,
				"source":         source,
				"call_id":        callID,
				"response_id":    strings.TrimSpace(toolEvent.ResponseID),
				"tool_name":      strings.TrimSpace(toolEvent.ToolName),
				"state":          strings.TrimSpace(toolEvent.State),
				"input_preview":  strings.TrimSpace(toolEvent.InputPreview),
				"output_len":     toolEvent.OutputLen,
				"output_preview": strings.TrimSpace(toolEvent.Output),
				"error_preview":  strings.TrimSpace(toolEvent.ErrorText),
			})
			flushRunning(false)
		})
		if runErr == nil {
			if strings.TrimSpace(reply) == "" {
				reply = streamState.Text
			}
			streamState.Text = strings.TrimSpace(reply)
			reply = marshalAssistantStructuredContent(streamState)
		}
	} else {
		invokeFields := map[string]any{
			"project_id":           projectID,
			"session_id":           sessionID,
			"source":               source,
			"assistant_message_id": assistantMessageID,
			"mode":                 "run",
		}
		for k, v := range evt.TriggerMeta {
			invokeFields[k] = v
		}
		logger.Log("pm.message.send.agentloop.invoke", invokeFields)
		reply, runErr = s.deps.AgentLoopRunner.Run(runCtx, agentPrompt)
	}
	if runErr != nil {
		_ = store.UpdatePMMessage(assistantMessageID, "", projectstate.StatusFailed, runErr.Error())
		logger.Log("pm.message.send.failed", map[string]any{
			"project_id": projectID,
			"session_id": sessionID,
			"source":     source,
			"error":      runErr.Error(),
		})
		s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source, "error": runErr.Error()})
		return runErr
	}
	if err := store.UpdatePMMessage(assistantMessageID, strings.TrimSpace(reply), projectstate.StatusCompleted, ""); err != nil {
		return err
	}
	logger.Log("pm.message.send.completed", map[string]any{
		"project_id":           projectID,
		"session_id":           sessionID,
		"source":               source,
		"assistant_message_id": assistantMessageID,
	})
	s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source})
	return nil
}

func (s *Server) runProjectManagerLoopEventHybrid(ctx context.Context, store *projectstate.Store, evt PMAgentLoopEvent) error {
	sessionID := strings.TrimSpace(evt.SessionID)
	projectID := strings.TrimSpace(evt.ProjectID)
	source := strings.TrimSpace(evt.Source)
	displayContent := strings.TrimSpace(evt.DisplayContent)
	agentPrompt := strings.TrimSpace(evt.AgentPrompt)
	historyBlock := strings.TrimSpace(evt.HistoryBlock)
	if displayContent == "" {
		return errors.New("display_content is required")
	}
	if agentPrompt == "" {
		return errors.New("agent_prompt is required")
	}
	if _, ok := s.deps.AgentLoopRunner.(agentLoopStreamingWithContextAndToolsResultRunner); !ok {
		if _, ok := s.deps.AgentLoopRunner.(agentLoopContextResultRunner); !ok {
			return s.runProjectManagerLoopEvent(ctx, store, evt)
		}
	}

	logger := newPMMessagesAuditLogger()
	defer logger.Close()

	previousResponseID, err := store.GetLatestPMAssistantResponseID(sessionID)
	if err != nil {
		return err
	}
	if _, err := store.InsertPMMessage(sessionID, "user", displayContent, projectstate.StatusCompleted, ""); err != nil {
		return err
	}
	assistantMessageID, err := store.InsertPMMessage(sessionID, "assistant", "", projectstate.StatusRunning, "")
	if err != nil {
		return err
	}
	startedFields := map[string]any{
		"project_id":           projectID,
		"session_id":           sessionID,
		"source":               source,
		"assistant_message_id": assistantMessageID,
		"user_content_preview": clipTaskAuditText(displayContent, 400),
		"agent_prompt_preview": clipTaskAuditText(agentPrompt, 200),
	}
	for k, v := range evt.TriggerMeta {
		startedFields[k] = v
	}
	logger.Log("pm.message.send.started", startedFields)
	s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source})

	runCtx := agentloopadapter.WithPMScope(ctx, agentloopadapter.PMScope{
		SessionID: sessionID,
		ProjectID: projectID,
		Source:    source,
	})
	runCtx = agentloopadapter.WithAllowedToolNamesResolver(runCtx, func() []string {
		return s.resolveProjectManagerAllowedToolNames(projectID, sessionID, source)
	})
	storeValue := true
	contextReq := buildAgentLoopContextRequest(agentPrompt, historyBlock, previousResponseID, &storeValue)

	reply := ""
	finalResponseID := ""
	runErr := error(nil)
	if streamRunner, ok := s.deps.AgentLoopRunner.(agentLoopStreamingWithContextAndToolsResultRunner); ok {
		invokeFields := map[string]any{
			"project_id":           projectID,
			"session_id":           sessionID,
			"source":               source,
			"assistant_message_id": assistantMessageID,
			"mode":                 "stream_with_context_and_tools",
		}
		for k, v := range evt.TriggerMeta {
			invokeFields[k] = v
		}
		logger.Log("pm.message.send.agentloop.invoke", invokeFields)
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
			if err := store.UpdatePMMessage(assistantMessageID, next, projectstate.StatusRunning, ""); err != nil {
				return
			}
			lastPublishAt = now
			s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source})
		}
		var out agentloop.RunResult
		out, runErr = streamRunner.RunStreamWithContextAndToolsResult(runCtx, contextReq, func(delta string) {
			if delta == "" {
				return
			}
			streamState.Text += delta
			flushRunning(false)
		}, func(event map[string]any) {
			toolEvent, ok := agentloopadapter.ParseLegacyToolEvent(event)
			if !ok {
				return
			}
			callID := strings.TrimSpace(toolEvent.CallID)
			next := toolEvent.ToToolStatePatch()
			if idx, ok := toolIndexByID[callID]; ok && idx >= 0 && idx < len(streamState.Tools) {
				streamState.Tools[idx] = agentloopadapter.MergeToolStatePatch(streamState.Tools[idx], next)
			} else {
				streamState.Tools = append(streamState.Tools, next)
				if callID != "" {
					toolIndexByID[callID] = len(streamState.Tools) - 1
				}
			}
			logger.Log("pm.message.send.agentloop.tool.event", map[string]any{
				"project_id":     projectID,
				"session_id":     sessionID,
				"source":         source,
				"call_id":        callID,
				"response_id":    strings.TrimSpace(toolEvent.ResponseID),
				"tool_name":      strings.TrimSpace(toolEvent.ToolName),
				"state":          strings.TrimSpace(toolEvent.State),
				"input_preview":  strings.TrimSpace(toolEvent.InputPreview),
				"output_len":     toolEvent.OutputLen,
				"output_preview": strings.TrimSpace(toolEvent.Output),
				"error_preview":  strings.TrimSpace(toolEvent.ErrorText),
			})
			flushRunning(false)
		})
		if runErr == nil {
			finalResponseID = strings.TrimSpace(out.FinalResponseID)
			reply = out.FinalText
			if strings.TrimSpace(reply) == "" {
				reply = streamState.Text
			}
			streamState.Text = strings.TrimSpace(reply)
			reply = marshalAssistantStructuredContent(streamState)
		}
	} else if contextRunner, ok := s.deps.AgentLoopRunner.(agentLoopContextResultRunner); ok {
		invokeFields := map[string]any{
			"project_id":           projectID,
			"session_id":           sessionID,
			"source":               source,
			"assistant_message_id": assistantMessageID,
			"mode":                 "run_with_context",
		}
		for k, v := range evt.TriggerMeta {
			invokeFields[k] = v
		}
		logger.Log("pm.message.send.agentloop.invoke", invokeFields)
		var out agentloop.RunResult
		out, runErr = contextRunner.RunWithContextResult(runCtx, contextReq)
		reply = out.FinalText
		finalResponseID = strings.TrimSpace(out.FinalResponseID)
	} else {
		return s.runProjectManagerLoopEvent(ctx, store, evt)
	}
	if runErr != nil {
		_ = store.UpdatePMMessage(assistantMessageID, "", projectstate.StatusFailed, runErr.Error())
		logger.Log("pm.message.send.failed", map[string]any{
			"project_id": projectID,
			"session_id": sessionID,
			"source":     source,
			"error":      runErr.Error(),
		})
		s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source, "error": runErr.Error()})
		return runErr
	}
	if err := store.UpdatePMMessageWithResponseID(assistantMessageID, strings.TrimSpace(reply), projectstate.StatusCompleted, "", finalResponseID); err != nil {
		return err
	}
	logger.Log("pm.message.send.completed", map[string]any{
		"project_id":           projectID,
		"session_id":           sessionID,
		"source":               source,
		"assistant_message_id": assistantMessageID,
		"response_id":          finalResponseID,
	})
	s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source})
	return nil
}

func (s *Server) resolveProjectManagerAllowedToolNames(projectID, sessionID, source string) []string {
	_ = projectID
	_ = sessionID
	_ = source
	profile := buildPMToolProfileCodexParity()
	return profile.ResolveAllowedTools(PMToolPolicy{})
}
