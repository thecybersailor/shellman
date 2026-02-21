package localapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"shellman/cli/internal/projectstate"
)

type TaskPromptHistoryOptions struct {
	MaxMessages int
	MaxChars    int
}

type TaskPromptHistoryMeta struct {
	TotalMessages int
	Included      int
	Dropped       int
	OutputChars   int
}

func defaultTaskPromptHistoryOptions() TaskPromptHistoryOptions {
	return TaskPromptHistoryOptions{
		MaxMessages: 80,
		MaxChars:    12000,
	}
}

func buildTaskPromptHistory(msgs []projectstate.TaskMessageRecord, opts TaskPromptHistoryOptions) (string, TaskPromptHistoryMeta) {
	if opts.MaxMessages <= 0 {
		opts.MaxMessages = 80
	}
	if opts.MaxChars <= 0 {
		opts.MaxChars = 12000
	}
	lines := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			continue
		}
		content := normalizeTimelineMessageContent(role, msg.Content)
		if content == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("[%s#%d] %s", role, msg.ID, content))
	}
	meta := TaskPromptHistoryMeta{TotalMessages: len(lines)}
	if len(lines) == 0 {
		return "", meta
	}

	start := 0
	if len(lines) > opts.MaxMessages {
		start = len(lines) - opts.MaxMessages
	}
	candidate := lines[start:]

	kept := make([]string, 0, len(candidate))
	totalChars := 0
	for i := len(candidate) - 1; i >= 0; i-- {
		line := candidate[i]
		addition := len(line)
		if len(kept) > 0 {
			addition++
		}
		if totalChars+addition > opts.MaxChars {
			break
		}
		kept = append(kept, line)
		totalChars += addition
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}

	meta.Included = len(kept)
	meta.Dropped = meta.TotalMessages - meta.Included
	if meta.Dropped < 0 {
		meta.Dropped = 0
	}

	if meta.Dropped == 0 {
		out := strings.Join(kept, "\n")
		meta.OutputChars = len(out)
		return out, meta
	}

	summary := fmt.Sprintf(
		"history_summary:\n- dropped_messages: %d\n- included_messages: %d\nrecent_history:\n%s",
		meta.Dropped,
		meta.Included,
		strings.Join(kept, "\n"),
	)
	meta.OutputChars = len(summary)
	return summary, meta
}

func normalizeTimelineMessageContent(role, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if text := extractPromptTextField(raw); text != "" {
		raw = text
	}
	if role == "assistant" {
		if text := extractAssistantStructuredContent(raw); text != "" {
			raw = text
		}
	}
	raw = strings.Join(strings.Fields(raw), " ")
	if len(raw) > 360 {
		raw = raw[:360] + "...(truncated)"
	}
	return strings.TrimSpace(raw)
}

func extractPromptTextField(raw string) string {
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(body["text"]))
	if text == "" || text == "<nil>" {
		return ""
	}
	return text
}

func extractAssistantStructuredContent(raw string) string {
	var body struct {
		Text  string           `json:"text"`
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return ""
	}
	text := strings.TrimSpace(body.Text)
	if len(body.Tools) == 0 {
		return text
	}
	toolStates := make([]string, 0, len(body.Tools))
	for _, tool := range body.Tools {
		name := strings.TrimSpace(fmt.Sprint(tool["tool_name"]))
		state := strings.TrimSpace(fmt.Sprint(tool["state"]))
		if name == "" {
			continue
		}
		if state == "" || state == "<nil>" {
			toolStates = append(toolStates, name)
			continue
		}
		toolStates = append(toolStates, name+":"+state)
	}
	if len(toolStates) == 0 {
		return text
	}
	if text == "" {
		return "tools(" + strings.Join(toolStates, ", ") + ")"
	}
	return text + " tools(" + strings.Join(toolStates, ", ") + ")"
}
