package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"

	dbmodel "shellman/cli/internal/db"
	"shellman/cli/internal/global"
	"shellman/cli/internal/projectstate"
)

type paneRuntimeBaseline struct {
	LastActiveAt  int64
	RuntimeStatus SessionStatus
	SnapshotHash  string
}

// loadPaneRuntimeBaselineFromDB builds pane_target -> runtime baseline from persisted runtime/task index.
func loadPaneRuntimeBaselineFromDB(logger *slog.Logger) map[string]paneRuntimeBaseline {
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	out := map[string]paneRuntimeBaseline{}

	configDir, err := global.DefaultConfigDir()
	if err != nil {
		logger.Warn("load pane baseline skipped: resolve config dir failed", "err", err)
		return out
	}
	projects, err := listProjectsForPaneBaseline(configDir, logger)
	if err != nil {
		logger.Warn("load pane baseline skipped: list projects failed", "err", err)
		return out
	}
	runtimeIndex, runtimeErr := loadPaneRuntimeBaselineIndex(logger)
	if runtimeErr != nil {
		logger.Warn("load pane baseline: build pane_runtime index failed", "err", runtimeErr)
	}
	paneBindingsByRepo, paneBindingErr := loadPaneBindingsByRepo(logger)
	if paneBindingErr != nil {
		logger.Warn("load pane baseline: build pane bindings index failed", "err", paneBindingErr)
	}
	taskLastModifiedIndex, taskLastModifiedErr := loadTaskLastModifiedIndex(logger)
	if taskLastModifiedErr != nil {
		logger.Warn("load pane baseline: build task last_modified index failed", "err", taskLastModifiedErr)
	}

	for _, p := range projects {
		repoRoot := strings.TrimSpace(p.RepoRoot)
		if repoRoot == "" {
			continue
		}
		panes := paneBindingsByRepo[repoRoot]
		if len(panes) == 0 {
			continue
		}

		for taskID, binding := range panes {
			base := paneRuntimeBaseline{
				LastActiveAt:  0,
				RuntimeStatus: SessionStatusUnknown,
			}
			if lastModified := taskLastModifiedIndex[taskLastModifiedKey(repoRoot, p.ProjectID, taskID)]; lastModified > 0 {
				base.LastActiveAt = lastModified
			}

			for _, paneID := range []string{
				strings.TrimSpace(binding.PaneTarget),
				strings.TrimSpace(binding.PaneID),
				strings.TrimSpace(binding.PaneUUID),
			} {
				if paneID == "" {
					continue
				}
				row, found := runtimeIndex[paneID]
				if !found {
					continue
				}
				if row.UpdatedAt > base.LastActiveAt {
					base.LastActiveAt = row.UpdatedAt
				}
				if status := normalizeBaselineRuntimeStatus(row.RuntimeStatus); status != SessionStatusUnknown {
					base.RuntimeStatus = status
				}
				hash := strings.TrimSpace(row.SnapshotHash)
				if hash != "" {
					base.SnapshotHash = hash
				}
			}

			mergePaneBaseline(out, strings.TrimSpace(binding.PaneTarget), base)
			mergePaneBaseline(out, strings.TrimSpace(binding.PaneID), base)
		}
	}

	logger.Info("pane baseline loaded from db", "targets_total", len(out))
	return out
}

func loadPaneRuntimeBaselineIndex(logger *slog.Logger) (map[string]projectstate.PaneRuntimeRecord, error) {
	out := map[string]projectstate.PaneRuntimeRecord{}
	db, err := projectstate.GlobalDB()
	if err != nil {
		return out, nil
	}
	rows, err := db.Query(`
SELECT pane_id, pane_target, runtime_status, snapshot_hash, updated_at
FROM pane_runtime
`)
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()

	put := func(key string, row projectstate.PaneRuntimeRecord) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		prev, ok := out[key]
		if !ok || row.UpdatedAt >= prev.UpdatedAt {
			out[key] = row
		}
	}

	for rows.Next() {
		var row projectstate.PaneRuntimeRecord
		if scanErr := rows.Scan(&row.PaneID, &row.PaneTarget, &row.RuntimeStatus, &row.SnapshotHash, &row.UpdatedAt); scanErr != nil {
			return out, scanErr
		}
		put(row.PaneID, row)
		put(row.PaneTarget, row)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if logger != nil {
		logger.Debug("load pane baseline: pane_runtime index ready", "keys_total", len(out))
	}
	return out, nil
}

func loadPaneBindingsByRepo(logger *slog.Logger) (map[string]projectstate.PanesIndex, error) {
	out := map[string]projectstate.PanesIndex{}
	db, err := projectstate.GlobalDB()
	if err != nil {
		return out, nil
	}
	rows, err := db.Query(`
SELECT repo_root, state_json
FROM legacy_state
WHERE state_key = 'panes_json'
`)
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var repoRoot string
		var stateJSON string
		if scanErr := rows.Scan(&repoRoot, &stateJSON); scanErr != nil {
			return out, scanErr
		}
		repoRoot = strings.TrimSpace(repoRoot)
		if repoRoot == "" || strings.TrimSpace(stateJSON) == "" {
			continue
		}
		var panes projectstate.PanesIndex
		if unmarshalErr := json.Unmarshal([]byte(stateJSON), &panes); unmarshalErr != nil {
			if logger != nil {
				logger.Warn("load pane baseline: decode panes_json failed", "repo_root", repoRoot, "err", unmarshalErr)
			}
			continue
		}
		if panes == nil {
			panes = projectstate.PanesIndex{}
		}
		out[repoRoot] = panes
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if logger != nil {
		logger.Debug("load pane baseline: pane bindings index ready", "repos_total", len(out))
	}
	return out, nil
}

func taskLastModifiedKey(repoRoot, projectID, taskID string) string {
	return strings.TrimSpace(repoRoot) + "\x00" + strings.TrimSpace(projectID) + "\x00" + strings.TrimSpace(taskID)
}

func loadTaskLastModifiedIndex(logger *slog.Logger) (map[string]int64, error) {
	out := map[string]int64{}
	db, err := projectstate.GlobalDB()
	if err != nil {
		return out, nil
	}
	rows, err := db.Query(`
SELECT repo_root, project_id, task_id, last_modified
FROM tasks
WHERE archived = false
`)
	if err != nil {
		return out, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var repoRoot string
		var projectID string
		var taskID string
		var lastModified int64
		if scanErr := rows.Scan(&repoRoot, &projectID, &taskID, &lastModified); scanErr != nil {
			return out, scanErr
		}
		if strings.TrimSpace(repoRoot) == "" || strings.TrimSpace(projectID) == "" || strings.TrimSpace(taskID) == "" {
			continue
		}
		key := taskLastModifiedKey(repoRoot, projectID, taskID)
		out[key] = lastModified
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if logger != nil {
		logger.Debug("load pane baseline: task last_modified index ready", "rows_total", len(out))
	}
	return out, nil
}

func listProjectsForPaneBaseline(configDir string, logger *slog.Logger) ([]global.ActiveProject, error) {
	if gdb, err := projectstate.GlobalDBGORM(); err == nil && gdb != nil {
		rows := []dbmodel.Project{}
		queryErr := gdb.Order("sort_order ASC").Order("updated_at DESC").Find(&rows).Error
		if queryErr == nil {
			out := make([]global.ActiveProject, 0, len(rows))
			for _, row := range rows {
				out = append(out, global.ActiveProject{
					ProjectID: strings.TrimSpace(row.ProjectID),
					RepoRoot:  strings.TrimSpace(row.RepoRoot),
				})
			}
			return out, nil
		}
		if logger != nil {
			logger.Warn("load pane baseline: list projects via global db failed; fallback to projects store", "err", queryErr)
		}
	}
	return global.NewProjectsStore(configDir).ListProjects()
}

func normalizeBaselineRuntimeStatus(raw string) SessionStatus {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(SessionStatusRunning):
		return SessionStatusRunning
	case string(SessionStatusReady):
		return SessionStatusReady
	default:
		return SessionStatusUnknown
	}
}

func mergePaneBaseline(dst map[string]paneRuntimeBaseline, target string, candidate paneRuntimeBaseline) {
	if target == "" {
		return
	}
	if candidate.LastActiveAt <= 0 && candidate.RuntimeStatus == SessionStatusUnknown && strings.TrimSpace(candidate.SnapshotHash) == "" {
		return
	}
	prev, ok := dst[target]
	if !ok {
		dst[target] = candidate
		return
	}
	if candidate.LastActiveAt > prev.LastActiveAt {
		dst[target] = candidate
		return
	}
	if candidate.LastActiveAt < prev.LastActiveAt {
		return
	}
	merged := prev
	if merged.RuntimeStatus == SessionStatusUnknown && candidate.RuntimeStatus != SessionStatusUnknown {
		merged.RuntimeStatus = candidate.RuntimeStatus
	}
	if strings.TrimSpace(merged.SnapshotHash) == "" && strings.TrimSpace(candidate.SnapshotHash) != "" {
		merged.SnapshotHash = strings.TrimSpace(candidate.SnapshotHash)
	}
	if merged.LastActiveAt <= 0 && candidate.LastActiveAt > 0 {
		merged.LastActiveAt = candidate.LastActiveAt
	}
	dst[target] = merged
}
