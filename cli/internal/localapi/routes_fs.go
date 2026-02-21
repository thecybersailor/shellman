package localapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) registerFSRoutes() {
	s.mux.HandleFunc("/api/v1/fs/roots", s.handleFSRoots)
	s.mux.HandleFunc("/api/v1/fs/list", s.handleFSList)
	s.mux.HandleFunc("/api/v1/fs/resolve", s.handleFSResolve)
	s.mux.HandleFunc("/api/v1/fs/search", s.handleFSSearch)
	s.mux.HandleFunc("/api/v1/fs/history", s.handleFSHistory)
}

func (s *Server) handleFSRoots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if s.deps.FSBrowser == nil {
		respondError(w, http.StatusNotImplemented, "FS_BROWSER_UNAVAILABLE", "filesystem browser is unavailable")
		return
	}
	roots, err := s.deps.FSBrowser.Roots()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "FS_ROOTS_LIST_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{"roots": roots})
}

func (s *Server) handleFSList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if s.deps.FSBrowser == nil {
		respondError(w, http.StatusNotImplemented, "FS_BROWSER_UNAVAILABLE", "filesystem browser is unavailable")
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	if path == "" {
		respondError(w, http.StatusBadRequest, "FS_PATH_REQUIRED", "path is required")
		return
	}
	out, err := s.deps.FSBrowser.List(path)
	if err != nil {
		respondError(w, http.StatusBadRequest, "FS_LIST_FAILED", err.Error())
		return
	}
	respondOK(w, out)
}

func (s *Server) handleFSResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if s.deps.FSBrowser == nil {
		respondError(w, http.StatusNotImplemented, "FS_BROWSER_UNAVAILABLE", "filesystem browser is unavailable")
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	resolved, err := s.deps.FSBrowser.Resolve(req.Path)
	if err != nil {
		respondError(w, http.StatusBadRequest, "FS_RESOLVE_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{"path": resolved})
}

func (s *Server) handleFSSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if s.deps.FSBrowser == nil {
		respondError(w, http.StatusNotImplemented, "FS_BROWSER_UNAVAILABLE", "filesystem browser is unavailable")
		return
	}
	base := strings.TrimSpace(r.URL.Query().Get("base"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if base == "" {
		respondError(w, http.StatusBadRequest, "FS_BASE_REQUIRED", "base is required")
		return
	}
	if q == "" {
		respondError(w, http.StatusBadRequest, "FS_QUERY_REQUIRED", "q is required")
		return
	}
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			respondError(w, http.StatusBadRequest, "FS_LIMIT_INVALID", "limit must be positive integer")
			return
		}
		limit = parsed
	}
	items, err := s.deps.FSBrowser.Search(base, q, limit)
	if err != nil {
		respondError(w, http.StatusBadRequest, "FS_SEARCH_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{"items": items})
}

func (s *Server) handleFSHistory(w http.ResponseWriter, r *http.Request) {
	if s.deps.DirHistory == nil {
		respondError(w, http.StatusNotImplemented, "FS_HISTORY_UNAVAILABLE", "directory history is unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		limit := 20
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				respondError(w, http.StatusBadRequest, "FS_LIMIT_INVALID", "limit must be positive integer")
				return
			}
			limit = parsed
		}
		rows, err := s.deps.DirHistory.List(limit)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "FS_HISTORY_LIST_FAILED", err.Error())
			return
		}
		items := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			items = append(items, map[string]any{
				"path":              row.Path,
				"first_accessed_at": row.FirstAccessed.Unix(),
				"last_accessed_at":  row.LastAccessed.Unix(),
				"access_count":      row.AccessCount,
			})
		}
		respondOK(w, map[string]any{"items": items})
	case http.MethodPost:
		var req struct {
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
			return
		}
		path := strings.TrimSpace(req.Path)
		if path == "" {
			respondError(w, http.StatusBadRequest, "FS_PATH_REQUIRED", "path is required")
			return
		}
		if err := s.deps.DirHistory.Upsert(path); err != nil {
			respondError(w, http.StatusInternalServerError, "FS_HISTORY_WRITE_FAILED", err.Error())
			return
		}
		respondOK(w, map[string]any{"path": path})
	case http.MethodDelete:
		if err := s.deps.DirHistory.Clear(); err != nil {
			respondError(w, http.StatusInternalServerError, "FS_HISTORY_CLEAR_FAILED", err.Error())
			return
		}
		respondOK(w, map[string]any{"cleared": true})
	default:
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
	}
}
