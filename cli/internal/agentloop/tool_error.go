package agentloop

import "encoding/json"

type ToolError struct {
	Message string `json:"error"`
	Suggest string `json:"suggest"`
}

func (e *ToolError) Error() string {
	if e == nil {
		return "UNKNOWN_ERROR"
	}
	if e.Message == "" {
		return "UNKNOWN_ERROR"
	}
	return e.Message
}

func NewToolError(message, suggest string) *ToolError {
	if suggest == "" {
		suggest = "NO_SUGGESTION"
	}
	return &ToolError{Message: message, Suggest: suggest}
}

func mustMarshalToolError(err *ToolError) string {
	if err == nil {
		err = NewToolError("UNKNOWN_ERROR", "NO_SUGGESTION")
	}
	raw, _ := json.Marshal(err)
	return string(raw)
}
