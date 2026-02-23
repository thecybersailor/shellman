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
			ProjectID string `json:"project_id"`
			RepoRoot  string `json:"repo_root"`
			IsGitRepo bool   `json:"is_git_repo"`
		}
		payload := make([]projectPayload, 0, len(projects))
		for _, p := range projects {
			payload = append(payload, projectPayload{
				ProjectID: p.ProjectID,
				RepoRoot:  p.RepoRoot,
				IsGitRepo: isGitWorkTree(p.RepoRoot),
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
		respondOK(w, map[string]any{
			"project_id":  req.ProjectID,
			"is_git_repo": isGitWorkTree(req.RepoRoot),
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
	if r.Method != http.MethodDelete {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	prefix := "/api/v1/projects/active/"
	projectID := strings.TrimPrefix(r.URL.Path, prefix)
	if projectID == "" || strings.Contains(projectID, "/") {
		respondError(w, http.StatusBadRequest, "INVALID_PROJECT_ID", "invalid project id")
		return
	}
	if err := s.deps.ProjectsStore.RemoveProject(projectID); err != nil {
		respondError(w, http.StatusInternalServerError, "PROJECT_REMOVE_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{"project_id": projectID})
}
