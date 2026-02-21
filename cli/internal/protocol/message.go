package protocol

import "encoding/json"

type Message struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Op      string          `json:"op"`
	Payload json.RawMessage `json:"payload"`
	Error   *ErrPayload     `json:"error,omitempty"`
}

type ErrPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func MustRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
