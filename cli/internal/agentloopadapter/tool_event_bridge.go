package agentloopadapter

import (
	"strings"

	"github.com/flaboy/agentloop"
)

func ToLegacyToolEvent(event agentloop.LoopEvent) (map[string]any, bool) {
	switch e := event.(type) {
	case agentloop.ToolInputEvent:
		return map[string]any{
			"type":          "tool_input",
			"call_id":       strings.TrimSpace(e.CallID),
			"response_id":   strings.TrimSpace(e.ResponseID),
			"tool_name":     strings.TrimSpace(e.ToolName),
			"state":         "input-available",
			"input":         strings.TrimSpace(e.Input),
			"input_preview": strings.TrimSpace(e.InputPreview),
			"input_len":     e.InputRawLen,
		}, true
	case agentloop.ToolOutputEvent:
		return map[string]any{
			"type":        "tool_output",
			"call_id":     strings.TrimSpace(e.CallID),
			"response_id": strings.TrimSpace(e.ResponseID),
			"tool_name":   strings.TrimSpace(e.ToolName),
			"state":       strings.TrimSpace(e.State),
			"output":      strings.TrimSpace(e.Output),
			"output_len":  e.OutputLen,
			"error_text":  strings.TrimSpace(e.ErrorString),
		}, true
	default:
		return nil, false
	}
}
