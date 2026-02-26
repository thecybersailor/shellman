package agentloop

import (
	"strings"
	"testing"
)

func TestResponsesClient_toSDKRequest_RejectsSystemNotFirst(t *testing.T) {
	client := NewResponsesClient(OpenAIConfig{Model: "gpt-5-mini"}, nil)
	_, err := client.toSDKRequest(CreateResponseRequest{
		Input: []map[string]any{
			{
				"type":    "message",
				"role":    "user",
				"content": []map[string]any{{"type": "input_text", "text": "u"}},
			},
			{
				"type":    "message",
				"role":    "system",
				"content": []map[string]any{{"type": "input_text", "text": "s"}},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "system message must be first") {
		t.Fatalf("expected invariant error, got %v", err)
	}
}
