package localapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"shellman/cli/internal/global"
)

var projectsActiveWriteInflight atomic.Int64
var projectsActiveWriteSeq atomic.Int64

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
			SortOrder   int64  `json:"sort_order"`
			Collapsed   bool   `json:"collapsed"`
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
				SortOrder:   p.SortOrder,
				Collapsed:   p.Collapsed,
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
		source := strings.TrimSpace(r.Header.Get("X-Shellman-Gateway-Source"))
		sortOrder := optionalSortOrder(req.SortOrder)
		seq, startedAt := logProjectsActiveWriteStart(http.MethodPost, req.ProjectID, source, sortOrder, &req.Collapsed)
		var opErr error
		defer func() {
			logProjectsActiveWriteEnd(seq, http.MethodPost, req.ProjectID, source, startedAt, opErr)
		}()

		if err := s.deps.ProjectsStore.AddProject(req); err != nil {
			opErr = err
			respondError(w, http.StatusInternalServerError, "PROJECT_ADD_FAILED", err.Error())
			return
		}
		projects, err := s.deps.ProjectsStore.ListProjects()
		if err != nil {
			opErr = err
			respondError(w, http.StatusInternalServerError, "PROJECTS_LIST_FAILED", err.Error())
			return
		}
		var saved *global.ActiveProject
		for i := range projects {
			if projects[i].ProjectID == req.ProjectID {
				saved = &projects[i]
				break
			}
		}
		if saved == nil {
			opErr = errors.New("project not found after save")
			respondError(w, http.StatusInternalServerError, "PROJECT_ADD_FAILED", "project not found after save")
			return
		}
		displayName := strings.TrimSpace(saved.DisplayName)
		if displayName == "" {
			displayName = saved.ProjectID
		}
		respondOK(w, map[string]any{
			"project_id":   req.ProjectID,
			"display_name": displayName,
			"is_git_repo":  isGitWorkTree(req.RepoRoot),
			"sort_order":   saved.SortOrder,
			"collapsed":    saved.Collapsed,
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
			DisplayName *string `json:"display_name"`
			SortOrder   *int64  `json:"sort_order"`
			Collapsed   *bool   `json:"collapsed"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		if req.DisplayName == nil && req.SortOrder == nil && req.Collapsed == nil {
			respondError(w, http.StatusBadRequest, "INVALID_PROJECT_PATCH", "at least one field is required")
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
		next := *target
		if req.DisplayName != nil {
			displayName := strings.TrimSpace(*req.DisplayName)
			if displayName == "" {
				respondError(w, http.StatusBadRequest, "INVALID_DISPLAY_NAME", "display_name is required")
				return
			}
			next.DisplayName = displayName
		}
		if req.SortOrder != nil {
			if *req.SortOrder <= 0 {
				respondError(w, http.StatusBadRequest, "INVALID_SORT_ORDER", "sort_order must be positive")
				return
			}
			next.SortOrder = *req.SortOrder
		}
		if req.Collapsed != nil {
			next.Collapsed = *req.Collapsed
		}

		source := strings.TrimSpace(r.Header.Get("X-Shellman-Gateway-Source"))
		seq, startedAt := logProjectsActiveWriteStart(http.MethodPatch, projectID, source, req.SortOrder, req.Collapsed)
		var opErr error
		defer func() {
			logProjectsActiveWriteEnd(seq, http.MethodPatch, projectID, source, startedAt, opErr)
		}()

		if err := s.deps.ProjectsStore.AddProject(global.ActiveProject{
			ProjectID:   next.ProjectID,
			RepoRoot:    next.RepoRoot,
			DisplayName: next.DisplayName,
			SortOrder:   next.SortOrder,
			Collapsed:   next.Collapsed,
		}); err != nil {
			opErr = err
			respondError(w, http.StatusInternalServerError, "PROJECT_RENAME_FAILED", err.Error())
			return
		}
		respondOK(w, map[string]any{
			"project_id":   next.ProjectID,
			"display_name": next.DisplayName,
			"sort_order":   next.SortOrder,
			"collapsed":    next.Collapsed,
		})
	default:
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}

func optionalSortOrder(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	v := value
	return &v
}

func logProjectsActiveWriteStart(method, projectID, source string, sortOrder *int64, collapsed *bool) (int64, time.Time) {
	seq := projectsActiveWriteSeq.Add(1)
	inflight := projectsActiveWriteInflight.Add(1)
	startedAt := time.Now().UTC()
	slog.Info(
		"projects.active.write.start",
		"seq",
		seq,
		"method",
		strings.TrimSpace(method),
		"project_id",
		strings.TrimSpace(projectID),
		"source",
		strings.TrimSpace(source),
		"sort_order",
		formatOptionalInt64(sortOrder),
		"collapsed",
		formatOptionalBool(collapsed),
		"inflight",
		inflight,
	)
	return seq, startedAt
}

func logProjectsActiveWriteEnd(seq int64, method, projectID, source string, startedAt time.Time, opErr error) {
	inflight := projectsActiveWriteInflight.Add(-1)
	durationMs := time.Since(startedAt).Milliseconds()
	if opErr != nil {
		slog.Error(
			"projects.active.write.end",
			"seq",
			seq,
			"method",
			strings.TrimSpace(method),
			"project_id",
			strings.TrimSpace(projectID),
			"source",
			strings.TrimSpace(source),
			"status",
			"error",
			"duration_ms",
			durationMs,
			"inflight",
			inflight,
			"err",
			opErr.Error(),
		)
		return
	}
	slog.Info(
		"projects.active.write.end",
		"seq",
		seq,
		"method",
		strings.TrimSpace(method),
		"project_id",
		strings.TrimSpace(projectID),
		"source",
		strings.TrimSpace(source),
		"status",
		"ok",
		"duration_ms",
		durationMs,
		"inflight",
		inflight,
	)
}

func formatOptionalInt64(value *int64) string {
	if value == nil {
		return "null"
	}
	return strconv.FormatInt(*value, 10)
}

func formatOptionalBool(value *bool) string {
	if value == nil {
		return "null"
	}
	if *value {
		return "true"
	}
	return "false"
}
