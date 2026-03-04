package localapi

import (
	"strings"

	"shellman/cli/internal/projectstate"
)

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

func paneRefMatchesBinding(paneHint string, binding projectstate.PaneBinding) bool {
	paneHint = strings.TrimSpace(paneHint)
	if paneHint == "" {
		return false
	}
	return paneHint == strings.TrimSpace(binding.PaneTarget) ||
		paneHint == strings.TrimSpace(binding.PaneID) ||
		paneHint == strings.TrimSpace(binding.PaneUUID)
}
