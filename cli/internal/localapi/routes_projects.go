package localapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"shellman/cli/internal/global"
)

func (s *Server) registerProjectsRoutes() {
	s.mux.HandleFunc("/api/v1/projects/active", s.handleProjectsActive)
	s.mux.HandleFunc("/api/v1/projects/active/", s.handleProjectsActiveByID)
}

func (s *Server) handleProjectsActive(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		projects, err := s.deps.ProjectsStore.ListProjects()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "PROJECTS_LIST_FAILED", err.Error())
			return
		}
		type projectPayload struct {
			ProjectID   string `json:"project_id"`
			DisplayName string `json:"display_name"`
			RepoRoot    string `json:"repo_root"`
			IsGitRepo   bool   `json:"is_git_repo"`
		}
		payload := make([]projectPayload, 0, len(projects))
		for _, p := range projects {
			displayName := strings.TrimSpace(p.DisplayName)
			if displayName == "" {
				displayName = p.ProjectID
			}
			payload = append(payload, projectPayload{
				ProjectID:   p.ProjectID,
				DisplayName: displayName,
				RepoRoot:    p.RepoRoot,
				IsGitRepo:   isGitWorkTree(p.RepoRoot),
			})
		}
		respondOK(w, payload)
	case http.MethodPost:
		var req global.ActiveProject
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		if req.ProjectID == "" || req.RepoRoot == "" {
			respondError(w, http.StatusBadRequest, "INVALID_PROJECT", "project_id and repo_root are required")
			return
		}
		if err := validateRepoRoot(req.RepoRoot); err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_PROJECT_ROOT", err.Error())
			return
		}
		if err := s.deps.ProjectsStore.AddProject(req); err != nil {
			respondError(w, http.StatusInternalServerError, "PROJECT_ADD_FAILED", err.Error())
			return
		}
		displayName := strings.TrimSpace(req.DisplayName)
		if displayName == "" {
			displayName = req.ProjectID
		}
		respondOK(w, map[string]any{
			"project_id":   req.ProjectID,
			"display_name": displayName,
			"is_git_repo":  isGitWorkTree(req.RepoRoot),
		})
	default:
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func validateRepoRoot(repoRoot string) error {
	info, err := os.Stat(repoRoot)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("repo_root must be a directory")
	}
	return nil
}

func isGitWorkTree(repoRoot string) bool {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--is-inside-work-tree")
	out, err := cmd.CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func (s *Server) handleProjectsActiveByID(w http.ResponseWriter, r *http.Request) {
	prefix := "/api/v1/projects/active/"
	projectID := strings.TrimPrefix(r.URL.Path, prefix)
	if projectID == "" || strings.Contains(projectID, "/") {
		respondError(w, http.StatusBadRequest, "INVALID_PROJECT_ID", "invalid project id")
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.deps.ProjectsStore.RemoveProject(projectID); err != nil {
			respondError(w, http.StatusInternalServerError, "PROJECT_REMOVE_FAILED", err.Error())
			return
		}
		respondOK(w, map[string]any{"project_id": projectID})
	case http.MethodPatch:
		var req struct {
			DisplayName string `json:"display_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		displayName := strings.TrimSpace(req.DisplayName)
		if displayName == "" {
			respondError(w, http.StatusBadRequest, "INVALID_DISPLAY_NAME", "display_name is required")
			return
		}
		projects, err := s.deps.ProjectsStore.ListProjects()
		if err != nil {
			respondError(w, http.StatusInternalServerError, "PROJECTS_LIST_FAILED", err.Error())
			return
		}
		var target *global.ActiveProject
		for i := range projects {
			if projects[i].ProjectID == projectID {
				target = &projects[i]
				break
			}
		}
		if target == nil {
			respondError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "project not found")
			return
		}
		if err := s.deps.ProjectsStore.AddProject(global.ActiveProject{
			ProjectID:   projectID,
			RepoRoot:    target.RepoRoot,
			DisplayName: displayName,
		}); err != nil {
			respondError(w, http.StatusInternalServerError, "PROJECT_RENAME_FAILED", err.Error())
			return
		}
		respondOK(w, map[string]any{"project_id": projectID, "display_name": displayName})
	default:
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
