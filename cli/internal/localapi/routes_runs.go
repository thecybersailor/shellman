package localapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"shellman/cli/internal/projectstate"
)

func (s *Server) registerRunRoutes() {
	s.mux.HandleFunc("/api/v1/runs/", s.handleRunActions)
}

type runLookupDiagnostics struct {
	PaneTarget           string
	CandidateTotal       int
	CandidateRunning     int
	CandidateLive        int
	CandidateLiveRunning int
	CandidateTaskMatch   int
	CandidateServerMatch int
	CandidateServerDiff  int
	Samples              []map[string]any
}

type paneTaskLookupDiagnostics struct {
	PaneTarget              string
	ProjectsScanned         int
	BindingsScanned         int
	BindingsWithPaneRef     int
	CandidateTotal          int
	CandidateWithTaskRecord int
	Samples                 []map[string]any
	PaneSamples             []map[string]any
}

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request, taskID string) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	projectID, store, _, err := s.findTask(taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "TASK_NOT_FOUND", err.Error())
		return
	}
	runID := newRunID()
	if err := store.InsertRun(projectstate.RunRecord{
		RunID:     runID,
		TaskID:    taskID,
		RunStatus: projectstate.RunStatusRunning,
	}); err != nil {
		respondError(w, http.StatusInternalServerError, "RUN_CREATE_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"project_id": projectID,
		"task_id":    taskID,
		"run_id":     runID,
		"status":     "running",
	})
}

func (s *Server) handleRunActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/runs/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
		return
	}
	runID := strings.TrimSpace(parts[0])

	if len(parts) == 1 && r.Method == http.MethodGet {
		respondOK(w, map[string]any{
			"run_id": runID,
		})
		return
	}
	if len(parts) == 2 && parts[1] == "events" && r.Method == http.MethodGet {
		respondOK(w, map[string]any{
			"run_id": runID,
			"items":  []any{},
		})
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost {
		switch parts[1] {
		case "bind-pane":
			s.handleRunBindPane(w, r, runID)
			return
		case "resume":
			s.handleRunResume(w, r, runID)
			return
		}
	}
	respondError(w, http.StatusNotFound, "NOT_FOUND", "route not found")
}

func (s *Server) handleRunBindPane(w http.ResponseWriter, r *http.Request, runID string) {
	var req struct {
		PaneID     string `json:"pane_id"`
		PaneTarget string `json:"pane_target"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	_, store, run, err := s.findRun(runID)
	if err != nil {
		respondError(w, http.StatusNotFound, "RUN_NOT_FOUND", err.Error())
		return
	}
	paneTarget := strings.TrimSpace(req.PaneTarget)
	if paneTarget == "" {
		paneTarget = strings.TrimSpace(req.PaneID)
	}
	if paneTarget == "" {
		paneTarget = strings.TrimSpace(r.Header.Get("X-Shellman-Active-Pane-Target"))
	}
	if paneTarget == "" {
		panes, loadErr := store.LoadPanes()
		if loadErr == nil {
			if binding, ok := panes[run.TaskID]; ok {
				if strings.TrimSpace(binding.PaneTarget) != "" {
					paneTarget = strings.TrimSpace(binding.PaneTarget)
				} else if strings.TrimSpace(binding.PaneID) != "" {
					paneTarget = strings.TrimSpace(binding.PaneID)
				}
			}
		}
	}
	if paneTarget == "" {
		respondError(w, http.StatusBadRequest, "INVALID_PANE_TARGET", "pane_target is required")
		return
	}
	paneID := strings.TrimSpace(req.PaneID)
	if paneID == "" {
		paneID = paneTarget
	}
	if err := store.UpsertRunBinding(projectstate.RunBinding{
		RunID:            runID,
		ServerInstanceID: detectServerInstanceID(),
		PaneID:           paneID,
		PaneTarget:       paneTarget,
		BindingStatus:    projectstate.BindingStatusLive,
	}); err != nil {
		respondError(w, http.StatusInternalServerError, "RUN_BIND_FAILED", err.Error())
		return
	}
	if err := store.SetRunStatus(runID, projectstate.RunStatusRunning); err != nil {
		respondError(w, http.StatusInternalServerError, "RUN_BIND_FAILED", err.Error())
		return
	}
	respondOK(w, map[string]any{
		"run_id":      runID,
		"status":      "running",
		"pane_id":     paneID,
		"pane_target": paneTarget,
	})
}

func (s *Server) handleRunResume(w http.ResponseWriter, r *http.Request, runID string) {
	_, store, _, err := s.findRun(runID)
	if err != nil {
		respondError(w, http.StatusNotFound, "RUN_NOT_FOUND", err.Error())
		return
	}
	invalidated, err := s.invalidateBindingIfServerChanged(store, runID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "RUN_RESUME_FAILED", err.Error())
		return
	}
	if invalidated {
		respondOK(w, map[string]any{
			"run_id": runID,
			"status": projectstate.RunStatusNeedsRebind,
		})
		return
	}
	respondOK(w, map[string]any{
		"run_id": runID,
		"status": projectstate.RunStatusRunning,
	})
}

func newRunID() string {
	return "r_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func (s *Server) findRun(runID string) (string, *projectstate.Store, projectstate.RunRecord, error) {
	projects, err := s.deps.ProjectsStore.ListProjects()
	if err != nil {
		return "", nil, projectstate.RunRecord{}, err
	}
	for _, p := range projects {
		store := projectstate.NewStore(p.RepoRoot)
		run, err := store.GetRun(runID)
		if err == nil {
			return p.ProjectID, store, run, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", nil, projectstate.RunRecord{}, err
		}
	}
	return "", nil, projectstate.RunRecord{}, errors.New("run not found")
}

func (s *Server) findTaskByPaneTarget(paneTarget string) (string, *projectstate.Store, projectstate.TaskIndexEntry, bool, paneTaskLookupDiagnostics, error) {
	projects, err := s.deps.ProjectsStore.ListProjects()
	if err != nil {
		return "", nil, projectstate.TaskIndexEntry{}, false, paneTaskLookupDiagnostics{}, err
	}
	diag := paneTaskLookupDiagnostics{
		PaneTarget:  paneTarget,
		Samples:     make([]map[string]any, 0, 8),
		PaneSamples: make([]map[string]any, 0, 16),
	}
	var bestProjectID string
	var bestStore *projectstate.Store
	var bestEntry projectstate.TaskIndexEntry
	bestFound := false

	for _, p := range projects {
		store := projectstate.NewStore(p.RepoRoot)
		diag.ProjectsScanned++

		panes, err := store.LoadPanes()
		if err != nil {
			return "", nil, projectstate.TaskIndexEntry{}, false, diag, err
		}

		for taskID, binding := range panes {
			diag.BindingsScanned++
			if strings.TrimSpace(binding.PaneTarget) != "" || strings.TrimSpace(binding.PaneID) != "" || strings.TrimSpace(binding.PaneUUID) != "" {
				diag.BindingsWithPaneRef++
			}
			if len(diag.PaneSamples) < 16 {
				diag.PaneSamples = append(diag.PaneSamples, map[string]any{
					"project_id":  p.ProjectID,
					"task_id":     taskID,
					"pane_target": strings.TrimSpace(binding.PaneTarget),
					"pane_id":     strings.TrimSpace(binding.PaneID),
					"pane_uuid":   strings.TrimSpace(binding.PaneUUID),
				})
			}
			if !paneRefMatchesBinding(paneTarget, binding) {
				continue
			}
			diag.CandidateTotal++
			entry, hasTaskRecord, err := findTaskEntryInProject(store, p.ProjectID, taskID)
			if err != nil {
				return "", nil, projectstate.TaskIndexEntry{}, false, diag, err
			}
			if hasTaskRecord {
				diag.CandidateWithTaskRecord++
			}
			if len(diag.Samples) < 8 {
				sample := map[string]any{
					"project_id":         p.ProjectID,
					"task_id":            taskID,
					"pane_target":        strings.TrimSpace(binding.PaneTarget),
					"pane_id":            strings.TrimSpace(binding.PaneID),
					"pane_uuid":          strings.TrimSpace(binding.PaneUUID),
					"task_record":        hasTaskRecord,
					"task_status":        "",
					"task_last_modified": int64(0),
				}
				if hasTaskRecord {
					sample["task_status"] = strings.TrimSpace(entry.Status)
					sample["task_last_modified"] = entry.LastModified
				}
				diag.Samples = append(diag.Samples, sample)
			}
			if !hasTaskRecord {
				continue
			}
			if !bestFound ||
				entry.LastModified > bestEntry.LastModified ||
				(entry.LastModified == bestEntry.LastModified && strings.TrimSpace(taskID) < strings.TrimSpace(bestEntry.TaskID)) {
				bestFound = true
				bestProjectID = p.ProjectID
				bestStore = store
				bestEntry = entry
			}
		}
	}

	if !bestFound {
		return "", nil, projectstate.TaskIndexEntry{}, false, diag, nil
	}
	return bestProjectID, bestStore, bestEntry, true, diag, nil
}

func (s *Server) findLiveRunningRunByPaneTargetForTask(store *projectstate.Store, paneTarget, taskID string) (projectstate.RunRecord, bool, runLookupDiagnostics, error) {
	diag := runLookupDiagnostics{
		PaneTarget: paneTarget,
		Samples:    make([]map[string]any, 0, 8),
	}
	if store == nil {
		return projectstate.RunRecord{}, false, diag, nil
	}

	candidates, err := store.ListRunCandidatesByPaneTarget(paneTarget, 8)
	if err != nil {
		return projectstate.RunRecord{}, false, diag, err
	}
	currentServerInstanceID := detectServerInstanceID()
	for _, item := range candidates {
		diag.CandidateTotal++
		if strings.TrimSpace(item.TaskID) == strings.TrimSpace(taskID) {
			diag.CandidateTaskMatch++
		}
		if item.RunStatus == projectstate.RunStatusRunning {
			diag.CandidateRunning++
		}
		if item.BindingStatus == projectstate.BindingStatusLive {
			diag.CandidateLive++
		}
		if item.RunStatus == projectstate.RunStatusRunning && item.BindingStatus == projectstate.BindingStatusLive {
			diag.CandidateLiveRunning++
		}
		if strings.TrimSpace(item.ServerInstanceID) != "" && strings.TrimSpace(currentServerInstanceID) != "" {
			if strings.TrimSpace(item.ServerInstanceID) == strings.TrimSpace(currentServerInstanceID) {
				diag.CandidateServerMatch++
			} else {
				diag.CandidateServerDiff++
			}
		}
		if len(diag.Samples) < 8 {
			diag.Samples = append(diag.Samples, map[string]any{
				"run_id":             item.RunID,
				"task_id":            item.TaskID,
				"run_status":         item.RunStatus,
				"binding_status":     item.BindingStatus,
				"pane_target":        item.PaneTarget,
				"pane_id":            item.PaneID,
				"server_instance_id": item.ServerInstanceID,
				"stale_reason":       item.StaleReason,
			})
		}
		if strings.TrimSpace(item.TaskID) != strings.TrimSpace(taskID) {
			continue
		}
		if item.RunStatus != projectstate.RunStatusRunning || item.BindingStatus != projectstate.BindingStatusLive {
			continue
		}
		run, err := store.GetRun(item.RunID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return projectstate.RunRecord{}, false, diag, err
		}
		return run, true, diag, nil
	}
	return projectstate.RunRecord{}, false, diag, nil
}

func (s *Server) invalidateBindingIfServerChanged(store *projectstate.Store, runID string) (bool, error) {
	binding, ok, err := store.GetLiveBindingByRunID(runID)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	currentServer := detectServerInstanceID()
	if strings.TrimSpace(currentServer) == "" || binding.ServerInstanceID == currentServer {
		return false, nil
	}
	if err := store.MarkBindingsStaleByServer(binding.ServerInstanceID, "tmux_restarted"); err != nil {
		return false, err
	}
	if err := store.SetRunStatus(runID, projectstate.RunStatusNeedsRebind); err != nil {
		return false, err
	}
	if err := store.AppendRunEvent(runID, "tmux_restarted", map[string]any{
		"expected_server_instance_id": binding.ServerInstanceID,
		"current_server_instance_id":  currentServer,
	}); err != nil {
		return false, err
	}
	return true, nil
}
