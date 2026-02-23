package localapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"shellman/cli/internal/projectstate"
)

func (s *Server) handleProjectManagerRoutes(w http.ResponseWriter, r *http.Request, projectID string, parts []string) bool {
	if len(parts) < 3 || strings.TrimSpace(parts[1]) != "pm" {
		return false
	}
	if strings.TrimSpace(parts[2]) != "sessions" {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return true
	}

	repoRoot, err := s.findProjectRepoRoot(projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
		return true
	}
	store := projectstate.NewStore(repoRoot)

	if len(parts) == 3 {
		switch r.Method {
		case http.MethodGet:
			s.handleProjectManagerListSessions(w, store, projectID)
		case http.MethodPost:
			s.handleProjectManagerCreateSession(w, r, store, projectID)
		default:
			respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		}
		return true
	}

	if len(parts) == 5 && r.Method == http.MethodPost && strings.TrimSpace(parts[4]) == "messages" {
		sessionID := strings.TrimSpace(parts[3])
		s.handleProjectManagerSendMessage(w, r, store, projectID, sessionID)
		return true
	}

	respondError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
	return true
}

func (s *Server) handleProjectManagerListSessions(w http.ResponseWriter, store *projectstate.Store, projectID string) {
	items, err := store.ListPMSessionsByProject(strings.TrimSpace(projectID), 200)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PM_SESSIONS_LOAD_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"project_id": strings.TrimSpace(projectID),
		"items":      items,
	})
}

func (s *Server) handleProjectManagerCreateSession(w http.ResponseWriter, r *http.Request, store *projectstate.Store, projectID string) {
	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	title := strings.TrimSpace(req.Title)
	sessionID, err := store.CreatePMSession(strings.TrimSpace(projectID), title)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PM_SESSION_CREATE_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"session_id": sessionID,
		"project_id": strings.TrimSpace(projectID),
		"title":      title,
	})
}

func (s *Server) handleProjectManagerSendMessage(w http.ResponseWriter, r *http.Request, store *projectstate.Store, projectID, sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		respondError(w, http.StatusBadRequest, "INVALID_SESSION_ID", "session_id is required")
		return
	}
	session, ok, err := store.GetPMSession(sessionID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "PM_SESSION_LOAD_FAILED", err.Error())
		return
	}
	if !ok || strings.TrimSpace(session.ProjectID) != strings.TrimSpace(projectID) {
		respondError(w, http.StatusNotFound, "PM_SESSION_NOT_FOUND", "project manager session not found")
		return
	}

	var req struct {
		Content string `json:"content"`
		Source  string `json:"source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		respondError(w, http.StatusBadRequest, "INVALID_MESSAGE", "content is required")
		return
	}
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "user_input"
	}

	if _, err := store.InsertPMMessage(sessionID, "user", content, projectstate.StatusCompleted, ""); err != nil {
		respondError(w, http.StatusInternalServerError, "PM_MESSAGE_INSERT_FAILED", err.Error())
		return
	}
	s.publishEvent("project.pm.messages.updated", strings.TrimSpace(projectID), "", map[string]any{"session_id": sessionID})

	go s.runProjectManagerReply(context.Background(), store, strings.TrimSpace(projectID), sessionID, content, source)

	respondOK(w, map[string]any{
		"project_id": projectID,
		"session_id": sessionID,
		"status":     "queued",
		"source":     source,
	})
}

func (s *Server) runProjectManagerReply(ctx context.Context, store *projectstate.Store, projectID, sessionID, content, source string) {
	if s == nil || store == nil {
		return
	}
	if s.deps.AgentLoopRunner == nil {
		return
	}
	assistantID, err := store.InsertPMMessage(sessionID, "assistant", "", projectstate.StatusRunning, "")
	if err != nil {
		return
	}
	reply, runErr := s.deps.AgentLoopRunner.Run(ctx, content)
	if runErr != nil {
		_ = store.UpdatePMMessage(assistantID, "", projectstate.StatusFailed, runErr.Error())
		s.publishEvent("project.pm.messages.updated", strings.TrimSpace(projectID), "", map[string]any{"session_id": sessionID, "error": runErr.Error(), "source": source})
		return
	}
	if err := store.UpdatePMMessage(assistantID, strings.TrimSpace(reply), projectstate.StatusCompleted, ""); err != nil {
		return
	}
	s.publishEvent("project.pm.messages.updated", strings.TrimSpace(projectID), "", map[string]any{"session_id": sessionID, "source": source})
}
