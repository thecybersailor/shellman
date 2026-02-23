package localapi

import (
	"context"
	"errors"
	"strings"

	"shellman/cli/internal/agentloop"
	"shellman/cli/internal/projectstate"
)

var ErrProjectManagerLoopUnavailable = errors.New("project manager loop runner is unavailable")

type PMAgentLoopEvent struct {
	SessionID      string
	ProjectID      string
	Source         string
	DisplayContent string
	AgentPrompt    string
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

	return s.runProjectManagerLoopEvent(ctx, store, evt)
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

	if _, err := store.InsertPMMessage(sessionID, "user", displayContent, projectstate.StatusCompleted, ""); err != nil {
		return err
	}
	assistantMessageID, err := store.InsertPMMessage(sessionID, "assistant", "", projectstate.StatusRunning, "")
	if err != nil {
		return err
	}
	s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source})

	runCtx := agentloop.WithPMScope(ctx, agentloop.PMScope{
		SessionID: sessionID,
		ProjectID: projectID,
		Source:    source,
	})
	runCtx = agentloop.WithAllowedToolNames(runCtx, s.resolveProjectManagerAllowedToolNames(projectID, sessionID, source))

	reply, runErr := s.deps.AgentLoopRunner.Run(runCtx, agentPrompt)
	if runErr != nil {
		_ = store.UpdatePMMessage(assistantMessageID, "", projectstate.StatusFailed, runErr.Error())
		s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source, "error": runErr.Error()})
		return runErr
	}
	if err := store.UpdatePMMessage(assistantMessageID, strings.TrimSpace(reply), projectstate.StatusCompleted, ""); err != nil {
		return err
	}
	s.publishEvent("project.pm.messages.updated", projectID, "", map[string]any{"session_id": sessionID, "source": source})
	return nil
}

func (s *Server) resolveProjectManagerAllowedToolNames(projectID, sessionID, source string) []string {
	_ = projectID
	_ = sessionID
	_ = source
	return []string{}
}
