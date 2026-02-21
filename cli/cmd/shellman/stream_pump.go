package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"shellman/cli/internal/bridge"
	"shellman/cli/internal/protocol"
	"shellman/cli/internal/streamdiff"
	"shellman/cli/internal/turn"
)

const maxTermFrameDataBytes = 24 * 1024

func runStreamPump(
	ctx context.Context,
	wsClient *turn.WSClient,
	tmuxService bridge.TmuxService,
	target *activeTarget,
	interval time.Duration,
	logger *slog.Logger,
) {
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	lastTarget := ""
	lastTargetVersion := uint64(0)
	lastSnapshot := ""
	lastCursorX := 0
	lastCursorY := 0
	hasLastCursor := false
	pendingSwitchTarget := ""
	pendingSwitchSince := time.Time{}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			currentTarget, currentTargetVersion := target.GetWithVersion()
			if currentTarget == "" {
				continue
			}

			if currentTarget != lastTarget || currentTargetVersion != lastTargetVersion {
				usedHistory := true
				snapshot, err := tmuxService.CaptureHistory(currentTarget, streamHistoryLines)
				if err != nil {
					logger.Warn("capture history failed", "target", currentTarget, "err", err)
					if isPaneTargetMissingError(err) {
						sendPaneEndedEvent(ctx, wsClient, currentTarget, err.Error(), logger)
						target.ClearIf(currentTarget)
						continue
					}
					usedHistory = false
					snapshot, err = tmuxService.CapturePane(currentTarget)
					if err != nil {
						logger.Warn("capture pane failed", "target", currentTarget, "err", err)
						if isPaneTargetMissingError(err) {
							sendPaneEndedEvent(ctx, wsClient, currentTarget, err.Error(), logger)
							target.ClearIf(currentTarget)
						}
						continue
					}
				} else if snapshot == "" {
					usedHistory = false
					snapshot, err = tmuxService.CapturePane(currentTarget)
					if err != nil {
						logger.Warn("capture pane failed", "target", currentTarget, "err", err)
						if isPaneTargetMissingError(err) {
							sendPaneEndedEvent(ctx, wsClient, currentTarget, err.Error(), logger)
							target.ClearIf(currentTarget)
						}
						continue
					}
				}
				snapshot = normalizeTermSnapshot(snapshot)
				snapshotLines := debugLineCount(snapshot)
				if !usedHistory && snapshotLines <= 1 {
					if pendingSwitchTarget != currentTarget {
						pendingSwitchTarget = currentTarget
						pendingSwitchSince = time.Now()
					}
					if time.Since(pendingSwitchSince) < 350*time.Millisecond {
						if traceStreamEnabled {
							logger.Debug("stream target switch wait stable", "target", currentTarget, "target_version", currentTargetVersion, "snapshot_lines", snapshotLines, "waited_ms", time.Since(pendingSwitchSince).Milliseconds())
						}
						continue
					}
				} else {
					pendingSwitchTarget = ""
					pendingSwitchSince = time.Time{}
				}

				cursorX, cursorY, cursorErr := tmuxService.CursorPosition(currentTarget)
				if cursorErr != nil {
					logger.Warn("read cursor position failed", "target", currentTarget, "err", cursorErr)
				}
				if traceStreamEnabled {
					logger.Debug("stream target switch", "from", lastTarget, "to", currentTarget, "from_version", lastTargetVersion, "to_version", currentTargetVersion, "send", "reset(snapshot)", "snapshot_len", len(snapshot), "cursor_ok", cursorErr == nil)
					logger.Debug("stream target switch detail", "target", currentTarget, "target_version", currentTargetVersion, "snapshot_lines", debugLineCount(snapshot), "cursor_x", cursorX, "cursor_y", cursorY, "snapshot_preview", debugPreview(snapshot, 240))
				}
				sendTermFrame(ctx, wsClient, currentTarget, "reset", snapshot, cursorX, cursorY, cursorErr == nil, logger)
				lastSnapshot = snapshot
				if traceStreamEnabled {
					logger.Debug("stream baseline", "target", currentTarget, "baseline_len", len(lastSnapshot), "baseline_lines", debugLineCount(lastSnapshot), "baseline_preview", debugPreview(lastSnapshot, 240))
				}
				lastTarget = currentTarget
				lastTargetVersion = currentTargetVersion
				if cursorErr == nil {
					lastCursorX = cursorX
					lastCursorY = cursorY
					hasLastCursor = true
				} else {
					hasLastCursor = false
				}
				continue
			}
			pendingSwitchTarget = ""
			pendingSwitchSince = time.Time{}

			snapshot, err := tmuxService.CapturePane(currentTarget)
			if err != nil {
				logger.Warn("capture pane failed", "target", currentTarget, "err", err)
				if isPaneTargetMissingError(err) {
					sendPaneEndedEvent(ctx, wsClient, currentTarget, err.Error(), logger)
					target.ClearIf(currentTarget)
				}
				continue
			}
			snapshot = normalizeTermSnapshot(snapshot)
			cursorX, cursorY, cursorErr := tmuxService.CursorPosition(currentTarget)
			if cursorErr != nil {
				logger.Warn("read cursor position failed", "target", currentTarget, "err", cursorErr)
			}
			cursorChanged := cursorErr == nil && (!hasLastCursor || cursorX != lastCursorX || cursorY != lastCursorY)
			snapshotChanged := snapshot != lastSnapshot
			if !snapshotChanged && !cursorChanged {
				continue
			}

			delta := streamdiff.DecideDelta(lastSnapshot, snapshot, snapshotChanged)
			mode := delta.Mode
			data := delta.Data
			sendTermFrame(ctx, wsClient, currentTarget, mode, data, cursorX, cursorY, cursorErr == nil, logger)
			lastSnapshot = snapshot
			lastTargetVersion = currentTargetVersion
			if cursorErr == nil {
				lastCursorX = cursorX
				lastCursorY = cursorY
				hasLastCursor = true
			}
		}
	}
}

func isPaneTargetMissingError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "can't find window") || strings.Contains(msg, "can't find pane")
}

func sendPaneEndedEvent(ctx context.Context, wsClient *turn.WSClient, target, reason string, logger *slog.Logger) {
	if target == "" {
		return
	}
	msg := protocol.Message{
		ID:   fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Type: "event",
		Op:   "pane.ended",
		Payload: protocol.MustRaw(map[string]any{
			"target": target,
			"reason": reason,
		}),
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		logger.Error("marshal pane.ended failed", "target", target, "err", err)
		return
	}
	if err := wsClient.Send(ctx, string(raw)); err != nil {
		logger.Error("send pane.ended failed", "target", target, "err", err)
	}
}

func sendTermFrame(
	ctx context.Context,
	wsClient *turn.WSClient,
	target string,
	mode string,
	data string,
	cursorX int,
	cursorY int,
	hasCursor bool,
	logger *slog.Logger,
) {
	frames := chunkTermData(mode, data)
	for _, frame := range frames {
		payload := map[string]any{
			"target": target,
			"mode":   frame.Mode,
			"data":   frame.Data,
		}
		if hasCursor {
			payload["cursor"] = map[string]int{
				"x": cursorX,
				"y": cursorY,
			}
		}

		msg := protocol.Message{
			ID:      fmt.Sprintf("evt_%d", time.Now().UnixNano()),
			Type:    "event",
			Op:      "term.output",
			Payload: protocol.MustRaw(payload),
		}
		raw, err := json.Marshal(msg)
		if err != nil {
			logger.Error("marshal term.output failed", "target", target, "err", err)
			return
		}
		if err := wsClient.Send(ctx, string(raw)); err != nil {
			logger.Error("send term.output failed", "target", target, "err", err)
			return
		}
	}
}

func debugEscapeText(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "\r", "\\r")
	text = strings.ReplaceAll(text, "\n", "\\n")
	text = strings.ReplaceAll(text, "\t", "\\t")
	text = strings.ReplaceAll(text, "\x1b", "\\u001b")
	return text
}

func debugPreview(text string, max int) string {
	if max <= 0 {
		max = 240
	}
	escaped := debugEscapeText(text)
	if len(escaped) <= max {
		return escaped
	}
	if max < 16 {
		return escaped[:max]
	}
	head := max / 2
	tail := max - head
	return fmt.Sprintf("%s...(truncated:%d)...%s", escaped[:head], len(escaped), escaped[len(escaped)-tail:])
}

func debugLineCount(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func commonPrefixLen(a, b string) int {
	max := len(a)
	if len(b) < max {
		max = len(b)
	}
	for i := 0; i < max; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return max
}

type termFrame struct {
	Mode string
	Data string
}

func chunkTermData(mode, data string) []termFrame {
	if len(data) <= maxTermFrameDataBytes {
		return []termFrame{{Mode: mode, Data: data}}
	}
	frames := make([]termFrame, 0, len(data)/maxTermFrameDataBytes+1)
	nextMode := mode
	rest := data
	for len(rest) > 0 {
		size := maxTermFrameDataBytes
		if len(rest) < size {
			size = len(rest)
		}
		for size > 0 && !utf8.ValidString(rest[:size]) {
			size--
		}
		if size == 0 {
			size = 1
		}
		frames = append(frames, termFrame{
			Mode: nextMode,
			Data: rest[:size],
		})
		rest = rest[size:]
		nextMode = "append"
	}
	return frames
}
