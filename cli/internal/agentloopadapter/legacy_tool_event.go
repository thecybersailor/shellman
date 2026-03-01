package agentloopadapter

import (
	"fmt"
	"strings"
)

type LegacyToolEvent struct {
	Type         string
	CallID       string
	ResponseID   string
	ToolName     string
	State        string
	Input        string
	Output       string
	ErrorText    string
	InputPreview string
	InputLen     int
	OutputLen    int
	HasInput     bool
	HasOutput    bool
	HasErrorText bool
}

func ParseLegacyToolEvent(raw map[string]any) (LegacyToolEvent, bool) {
	if len(raw) == 0 {
		return LegacyToolEvent{}, false
	}
	eventType := strings.TrimSpace(toEventString(raw["type"]))
	if eventType == "" || eventType == "agent-debug" {
		return LegacyToolEvent{}, false
	}
	out := LegacyToolEvent{
		Type:         eventType,
		CallID:       strings.TrimSpace(toEventString(raw["call_id"])),
		ResponseID:   strings.TrimSpace(toEventString(raw["response_id"])),
		ToolName:     strings.TrimSpace(toEventString(raw["tool_name"])),
		State:        strings.TrimSpace(toEventString(raw["state"])),
		InputPreview: strings.TrimSpace(toEventString(raw["input_preview"])),
		InputLen:     firstEventInt(raw["input_raw_len"], raw["input_len"]),
		OutputLen:    firstEventInt(raw["output_len"]),
	}
	if input, ok := raw["input"]; ok {
		out.HasInput = true
		out.Input = strings.TrimSpace(toEventString(input))
	}
	if output, ok := raw["output"]; ok {
		out.HasOutput = true
		out.Output = strings.TrimSpace(toEventString(output))
	}
	if errText, ok := raw["error_text"]; ok {
		out.HasErrorText = true
		out.ErrorText = strings.TrimSpace(toEventString(errText))
	}
	return out, true
}

func (e LegacyToolEvent) ToToolStatePatch() map[string]any {
	patch := map[string]any{
		"type":      strings.TrimSpace(e.Type),
		"tool_name": strings.TrimSpace(e.ToolName),
		"state":     strings.TrimSpace(e.State),
	}
	if e.HasInput {
		patch["input"] = strings.TrimSpace(e.Input)
	}
	if e.HasOutput {
		patch["output"] = strings.TrimSpace(e.Output)
	}
	if e.HasErrorText {
		patch["error_text"] = strings.TrimSpace(e.ErrorText)
	}
	return patch
}

func MergeToolStatePatch(current map[string]any, patch map[string]any) map[string]any {
	if current == nil {
		current = map[string]any{}
	}
	for k, v := range patch {
		if strings.TrimSpace(fmt.Sprint(v)) == "" {
			continue
		}
		current[k] = v
	}
	return current
}

func toEventString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func firstEventInt(values ...any) int {
	for _, value := range values {
		if n, ok := eventInt(value); ok {
			return n
		}
	}
	return 0
}

func eventInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case uint:
		return int(n), true
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return int(n), true
	case uint64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}
