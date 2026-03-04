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

func runStatusPump(
	ctx context.Context,
	wsClient *turn.WSClient,
	tmuxService bridge.TmuxService,
	httpExec gatewayHTTPExecutor,
	interval time.Duration,
	inputTracker *inputActivityTracker,
	logger *slog.Logger,
	extras ...any,
) {
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	_ = wsClient
	_ = tmuxService
	_ = httpExec
	_ = interval
	_ = inputTracker
	_ = extras
	logger.Info("status pump disabled: runtime uses pane/registry event stream")
	<-ctx.Done()
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
