package main

import (
	"io"
	"log/slog"
	"strings"

	"termteam/cli/internal/global"
	"termteam/cli/internal/projectstate"
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
	projects, err := global.NewProjectsStore(configDir).ListProjects()
	if err != nil {
		logger.Warn("load pane baseline skipped: list projects failed", "err", err)
		return out
	}

	for _, p := range projects {
		repoRoot := strings.TrimSpace(p.RepoRoot)
		if repoRoot == "" {
			continue
		}
		store := projectstate.NewStore(repoRoot)
		panes, err := store.LoadPanes()
		if err != nil {
			logger.Warn("load pane baseline: load panes failed", "project_id", p.ProjectID, "repo_root", repoRoot, "err", err)
			continue
		}
		rows, err := store.ListTasksByProject(p.ProjectID)
		if err != nil {
			logger.Warn("load pane baseline: list tasks failed", "project_id", p.ProjectID, "repo_root", repoRoot, "err", err)
			continue
		}
		rowByTaskID := make(map[string]projectstate.TaskRecordRow, len(rows))
		for _, row := range rows {
			rowByTaskID[row.TaskID] = row
		}

		for taskID, binding := range panes {
			base := paneRuntimeBaseline{
				LastActiveAt:  0,
				RuntimeStatus: SessionStatusUnknown,
			}
			if row, ok := rowByTaskID[taskID]; ok && row.LastModified > 0 {
				base.LastActiveAt = row.LastModified
			}

			for _, paneID := range []string{
				strings.TrimSpace(binding.PaneTarget),
				strings.TrimSpace(binding.PaneID),
				strings.TrimSpace(binding.PaneUUID),
			} {
				if paneID == "" {
					continue
				}
				row, found, runtimeErr := store.GetPaneRuntimeByPaneID(paneID)
				if runtimeErr != nil || !found {
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
