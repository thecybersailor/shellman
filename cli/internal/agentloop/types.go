package agentloop

import (
	"context"
	"encoding/json"
)

type ResponseToolSpec struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type Tool interface {
	Name() string
	Spec() ResponseToolSpec
	Execute(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError)
}
