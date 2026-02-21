package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"shellman/cli/internal/bridge"
	"shellman/cli/internal/protocol"
	"shellman/cli/internal/turn"
)

const tmuxStatusMaxMessageBytes = 24 * 1024

type sessionStatusItem struct {
	Target         string        `json:"target"`
	Title          string        `json:"title"`
	CurrentCommand string        `json:"current_command,omitempty"`
	Status         SessionStatus `json:"status"`
	UpdatedAt      int64         `json:"updated_at"`
}

type paneStatusMetadataProvider interface {
	PaneTitleAndCurrentCommand(target string) (string, string, error)
}

type paneStatusActivityProvider interface {
	PaneLastActiveAt(target string) (time.Time, error)
}

func runStatusPump(
	ctx context.Context,
	wsClient *turn.WSClient,
	tmuxService bridge.TmuxService,
	httpExec gatewayHTTPExecutor,
	interval time.Duration,
	inputTracker *inputActivityTracker,
	logger *slog.Logger,
	baselineByTargetOpt ...map[string]paneRuntimeBaseline,
) {
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	var baselineByTarget map[string]paneRuntimeBaseline
	if len(baselineByTargetOpt) > 0 {
		baselineByTarget = baselineByTargetOpt[0]
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	states := map[string]paneStatusState{}
	metaProvider, hasMetaProvider := tmuxService.(paneStatusMetadataProvider)
	activityProvider, hasActivityProvider := tmuxService.(paneStatusActivityProvider)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			targets, err := tmuxService.ListSessions()
			if err != nil {
				logger.Warn("list sessions for status failed", "err", err)
				continue
			}

			now := time.Now().UTC()
			seen := map[string]struct{}{}
			items := make([]sessionStatusItem, 0, len(targets))
			readyCount := 0
			runningCount := 0
			unknownCount := 0

			for _, target := range targets {
				seen[target] = struct{}{}
				title := sessionTitleFromTarget(target)
				currentCommand := ""
				if hasMetaProvider {
					metaTitle, metaCommand, metaErr := metaProvider.PaneTitleAndCurrentCommand(target)
					if metaErr == nil {
						if metaTitle != "" {
							title = metaTitle
						}
						currentCommand = metaCommand
					}
				}
				snapshot, capErr := tmuxService.CapturePane(target)
				if capErr != nil {
					if prevState, ok := states[target]; ok && prevState.Emitted != "" {
						logger.Warn(
							"pane status reset to unknown due to capture failure",
							"pane_target", target,
							"prev_status", prevState.Emitted,
							"err", capErr,
						)
					}
					items = append(items, sessionStatusItem{
						Target:         target,
						Title:          title,
						CurrentCommand: currentCommand,
						Status:         SessionStatusUnknown,
						UpdatedAt:      now.Unix(),
					})
					delete(states, target)
					unknownCount++
					continue
				}

				currHash := sha1Text(normalizeTermSnapshot(snapshot))
				lastInputAt := time.Time{}
				if inputTracker != nil {
					lastInputAt = inputTracker.Last(target)
				}
				prevState := states[target]
				if baselineByTarget != nil {
					if baseline, ok := baselineByTarget[target]; ok {
						prevState = applyPaneRuntimeBaseline(prevState, baseline, now)
					}
				}
				if hasActivityProvider && prevState.PrevHash == "" && prevState.LastActiveAt.IsZero() {
					lastActiveAt, activityErr := activityProvider.PaneLastActiveAt(target)
					if activityErr == nil && !lastActiveAt.IsZero() {
						if lastActiveAt.After(now) {
							lastActiveAt = now
						}
						prevState.LastActiveAt = lastActiveAt
					}
				}
				state := advancePaneStatus(
					prevState,
					currHash,
					now,
					lastInputAt,
					statusTransitionDelay,
					statusInputIgnoreWindow,
				)
				states[target] = state
				status := state.Emitted
				if status == "" {
					status = SessionStatusUnknown
				}
				if prevState.Emitted != status {
					logger.Info(
						"pane status changed",
						"pane_target", target,
						"from", prevState.Emitted,
						"to", status,
						"current_command", currentCommand,
						"prev_hash_prefix", hashPrefix(prevState.PrevHash),
						"curr_hash_prefix", hashPrefix(currHash),
						"candidate", state.Candidate,
						"candidate_since", unixSecondOrZero(state.CandidateSince),
						"transition_delay_ms", statusTransitionDelay.Milliseconds(),
						"input_ignore_window_ms", statusInputIgnoreWindow.Milliseconds(),
						"last_active_at", unixSecondOrZero(state.LastActiveAt),
						"last_active_age_ms", ageMilliseconds(now, state.LastActiveAt),
						"last_input_at", unixSecondOrZero(lastInputAt),
						"last_input_age_ms", ageMilliseconds(now, lastInputAt),
					)
				}
				if prevState.Emitted != SessionStatusReady && status == SessionStatusReady {
					logger.Info(
						"pane entered ready (status pump only)",
						"pane_target", target,
						"prev_status", prevState.Emitted,
						"status", status,
						"current_command", currentCommand,
						"prev_hash_prefix", hashPrefix(prevState.PrevHash),
						"curr_hash_prefix", hashPrefix(currHash),
						"candidate", state.Candidate,
						"candidate_since", unixSecondOrZero(state.CandidateSince),
						"transition_delay_ms", statusTransitionDelay.Milliseconds(),
						"input_ignore_window_ms", statusInputIgnoreWindow.Milliseconds(),
						"last_active_age_ms", ageMilliseconds(now, state.LastActiveAt),
						"last_input_age_ms", ageMilliseconds(now, lastInputAt),
						"panes_total_current_tick", len(targets),
						"tracked_states_total", len(states),
					)
				}
				updatedAt := state.LastActiveAt
				if updatedAt.IsZero() {
					updatedAt = now
				}
				switch status {
				case SessionStatusReady:
					readyCount++
				case SessionStatusRunning:
					runningCount++
				default:
					unknownCount++
				}

				items = append(items, sessionStatusItem{
					Target:         target,
					Title:          title,
					CurrentCommand: currentCommand,
					Status:         status,
					UpdatedAt:      updatedAt.Unix(),
				})
			}

			for target := range states {
				if _, ok := seen[target]; !ok {
					logger.Info("pane removed from status tracking", "pane_target", target)
					delete(states, target)
				}
			}
			if inputTracker != nil {
				inputTracker.DeleteMissing(seen)
			}

			msgs, err := buildTmuxStatusMessages(items, tmuxStatusMaxMessageBytes, time.Now)
			if err != nil {
				logger.Error("build tmux.status message failed", "err", err)
				continue
			}
			for _, msg := range msgs {
				raw, err := json.Marshal(msg)
				if err != nil {
					logger.Error("marshal tmux.status failed", "err", err)
					break
				}
				if err := wsClient.Send(ctx, string(raw)); err != nil {
					logger.Warn("send tmux.status failed", "err", err)
					break
				}
			}
		}
	}
}

func applyPaneRuntimeBaseline(state paneStatusState, baseline paneRuntimeBaseline, now time.Time) paneStatusState {
	if baseline.LastActiveAt > 0 && state.LastActiveAt.IsZero() {
		at := time.Unix(baseline.LastActiveAt, 0).UTC()
		if at.After(now) {
			at = now
		}
		state.LastActiveAt = at
	}
	if strings.TrimSpace(baseline.SnapshotHash) != "" && state.PrevHash == "" {
		state.PrevHash = strings.TrimSpace(baseline.SnapshotHash)
	}
	if state.Emitted == "" {
		switch baseline.RuntimeStatus {
		case SessionStatusRunning, SessionStatusReady, SessionStatusUnknown:
			state.Emitted = baseline.RuntimeStatus
			state.Candidate = baseline.RuntimeStatus
			if !state.LastActiveAt.IsZero() {
				state.CandidateSince = state.LastActiveAt
			} else {
				state.CandidateSince = now
			}
		}
	}
	return state
}

func buildTmuxStatusMessages(items []sessionStatusItem, maxBytes int, nowFn func() time.Time) ([]protocol.Message, error) {
	if nowFn == nil {
		nowFn = time.Now
	}
	if maxBytes <= 0 {
		maxBytes = tmuxStatusMaxMessageBytes
	}
	chunks, err := splitTmuxStatusItems(items, maxBytes)
	if err != nil {
		return nil, err
	}
	baseID := nowFn().UnixNano()
	msgs := make([]protocol.Message, 0, len(chunks))
	for idx, chunk := range chunks {
		msgs = append(msgs, protocol.Message{
			ID:   fmt.Sprintf("evt_status_%d_%d", baseID, idx),
			Type: "event",
			Op:   "tmux.status",
			Payload: protocol.MustRaw(map[string]any{
				"mode":        "full",
				"items":       chunk,
				"chunk_index": idx,
				"chunk_total": len(chunks),
			}),
		})
	}
	return msgs, nil
}

func splitTmuxStatusItems(items []sessionStatusItem, maxBytes int) ([][]sessionStatusItem, error) {
	if len(items) == 0 {
		return [][]sessionStatusItem{{}}, nil
	}
	chunks := make([][]sessionStatusItem, 0, 1)
	current := make([]sessionStatusItem, 0, len(items))
	for _, item := range items {
		candidate := append(current, item)
		ok, err := tmuxStatusChunkFits(candidate, maxBytes)
		if err != nil {
			return nil, err
		}
		if ok {
			current = candidate
			continue
		}
		if len(current) == 0 {
			current = []sessionStatusItem{item}
			chunks = append(chunks, current)
			current = make([]sessionStatusItem, 0, len(items))
			continue
		}
		chunks = append(chunks, current)
		current = []sessionStatusItem{item}
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks, nil
}

func tmuxStatusChunkFits(items []sessionStatusItem, maxBytes int) (bool, error) {
	probe := protocol.Message{
		ID:   "probe",
		Type: "event",
		Op:   "tmux.status",
		Payload: protocol.MustRaw(map[string]any{
			"mode":  "full",
			"items": items,
		}),
	}
	raw, err := json.Marshal(probe)
	if err != nil {
		return false, err
	}
	return len(raw) <= maxBytes, nil
}

func triggerAutoCompletionByPane(autoComplete paneAutoCompletionExecutor, paneTarget string, logger *slog.Logger) {
	triggerAutoCompletionByPaneWithObservedAt(autoComplete, paneTarget, time.Time{}, logger)
}

func triggerAutoCompletionByPaneWithObservedAt(autoComplete paneAutoCompletionExecutor, paneTarget string, observedLastActiveAt time.Time, logger *slog.Logger) {
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	if autoComplete == nil {
		logger.Warn("skip auto-complete: executor is nil", "pane_target", paneTarget)
		return
	}
	paneTarget = strings.TrimSpace(paneTarget)
	if paneTarget == "" {
		logger.Warn("skip auto-complete: empty pane target")
		return
	}
	resp, err := autoComplete(paneTarget, observedLastActiveAt)
	if err != nil {
		logger.Warn("auto-complete trigger failed", "pane_target", paneTarget, "err", err)
		return
	}
	logger.Info(
		"auto-complete response",
		"pane_target", paneTarget,
		"triggered", resp.Triggered,
		"result_status", strings.TrimSpace(resp.Status),
		"reason", strings.TrimSpace(resp.Reason),
		"run_id", strings.TrimSpace(resp.RunID),
		"task_id", strings.TrimSpace(resp.TaskID),
	)
	if strings.TrimSpace(resp.Reason) == "no-task-pane-binding" {
		logger.Error(
			"auto-complete response indicates pane has no task binding",
			"pane_target", paneTarget,
			"triggered", resp.Triggered,
			"result_status", strings.TrimSpace(resp.Status),
			"reason", strings.TrimSpace(resp.Reason),
			"run_id", strings.TrimSpace(resp.RunID),
			"task_id", strings.TrimSpace(resp.TaskID),
		)
	}
	if strings.TrimSpace(resp.Reason) == "no-live-running-run" {
		logger.Error(
			"auto-complete returned impossible no-live-running-run",
			"pane_target", paneTarget,
			"triggered", resp.Triggered,
			"result_status", strings.TrimSpace(resp.Status),
			"run_id", strings.TrimSpace(resp.RunID),
			"task_id", strings.TrimSpace(resp.TaskID),
		)
	}
}

func hashPrefix(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) <= 8 {
		return hash
	}
	return hash[:8]
}

func unixSecondOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().Unix()
}

func ageMilliseconds(now, at time.Time) int64 {
	if at.IsZero() {
		return -1
	}
	return now.Sub(at).Milliseconds()
}
