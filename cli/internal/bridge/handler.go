package bridge

import (
	"encoding/json"

	"shellman/cli/internal/protocol"
)

type TmuxService interface {
	ListSessions() ([]string, error)
	PaneExists(target string) (bool, error)
	SelectPane(target string) error
	SendInput(target, text string) error
	Resize(target string, cols, rows int) error
	CapturePane(target string) (string, error)
	CaptureHistory(target string, lines int) (string, error)
	StartPipePane(target, shellCmd string) error
	StopPipePane(target string) error
	CursorPosition(target string) (int, int, error)
	CreateSiblingPane(target string) (string, error)
	CreateChildPane(target string) (string, error)
}

type Handler struct {
	tmux     TmuxService
	httpExec func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error)
}

func NewHandler(t TmuxService) *Handler {
	return &Handler{tmux: t}
}

func (h *Handler) SetHTTPExecutor(exec func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error)) {
	h.httpExec = exec
}

func (h *Handler) Handle(msg protocol.Message) protocol.Message {
	resp := protocol.Message{ID: msg.ID, Type: "res", Op: msg.Op}

	switch msg.Op {
	case "tmux.list":
		sessions, err := h.tmux.ListSessions()
		if err != nil {
			resp.Error = &protocol.ErrPayload{Code: "TMUX_ERROR", Message: err.Error()}
			return resp
		}
		resp.Payload = protocol.MustRaw(map[string]any{"sessions": sessions})
		return resp
	case "tmux.select_pane":
		var payload struct {
			Target string `json:"target"`
			Cols   int    `json:"cols"`
			Rows   int    `json:"rows"`
		}
		_ = json.Unmarshal(msg.Payload, &payload)
		exists, err := h.tmux.PaneExists(payload.Target)
		if err != nil {
			resp.Error = &protocol.ErrPayload{Code: "TMUX_ERROR", Message: err.Error()}
			return resp
		}
		if !exists {
			resp.Error = &protocol.ErrPayload{Code: "TMUX_PANE_NOT_FOUND", Message: "pane target not found"}
			return resp
		}
		if payload.Cols >= 2 && payload.Rows >= 2 {
			if err := h.tmux.Resize(payload.Target, payload.Cols, payload.Rows); err != nil {
				resp.Error = &protocol.ErrPayload{Code: "TMUX_ERROR", Message: err.Error()}
				return resp
			}
		}
		if err := h.tmux.SelectPane(payload.Target); err != nil {
			recheckExists, recheckErr := h.tmux.PaneExists(payload.Target)
			if recheckErr == nil && !recheckExists {
				resp.Error = &protocol.ErrPayload{Code: "TMUX_PANE_NOT_FOUND", Message: "pane target not found"}
				return resp
			}
			resp.Error = &protocol.ErrPayload{Code: "TMUX_ERROR", Message: err.Error()}
			return resp
		}
		resp.Payload = protocol.MustRaw(map[string]any{})
		return resp
	case "term.input":
		var payload struct {
			Target string `json:"target"`
			Text   string `json:"text"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			resp.Error = &protocol.ErrPayload{Code: "BAD_PAYLOAD", Message: err.Error()}
			return resp
		}
		if err := h.tmux.SendInput(payload.Target, payload.Text); err != nil {
			resp.Error = &protocol.ErrPayload{Code: "TMUX_ERROR", Message: err.Error()}
			return resp
		}
		resp.Payload = protocol.MustRaw(map[string]any{})
		return resp
	case "term.resize":
		var payload struct {
			Target string `json:"target"`
			Cols   int    `json:"cols"`
			Rows   int    `json:"rows"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			resp.Error = &protocol.ErrPayload{Code: "BAD_PAYLOAD", Message: err.Error()}
			return resp
		}
		if err := h.tmux.Resize(payload.Target, payload.Cols, payload.Rows); err != nil {
			resp.Error = &protocol.ErrPayload{Code: "TMUX_ERROR", Message: err.Error()}
			return resp
		}
		resp.Payload = protocol.MustRaw(map[string]any{})
		return resp
	case "tmux.create_sibling_pane":
		var payload struct {
			Target string `json:"target"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			resp.Error = &protocol.ErrPayload{Code: "BAD_PAYLOAD", Message: err.Error()}
			return resp
		}
		pane, err := h.tmux.CreateSiblingPane(payload.Target)
		if err != nil {
			resp.Error = &protocol.ErrPayload{Code: "TMUX_ERROR", Message: err.Error()}
			return resp
		}
		resp.Payload = protocol.MustRaw(map[string]any{"pane_id": pane})
		return resp
	case "tmux.create_child_pane":
		var payload struct {
			Target string `json:"target"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			resp.Error = &protocol.ErrPayload{Code: "BAD_PAYLOAD", Message: err.Error()}
			return resp
		}
		pane, err := h.tmux.CreateChildPane(payload.Target)
		if err != nil {
			resp.Error = &protocol.ErrPayload{Code: "TMUX_ERROR", Message: err.Error()}
			return resp
		}
		resp.Payload = protocol.MustRaw(map[string]any{"pane_id": pane})
		return resp
	case "gateway.http":
		if h.httpExec == nil {
			resp.Error = &protocol.ErrPayload{Code: "GATEWAY_UNAVAILABLE", Message: "http executor unavailable"}
			return resp
		}
		var payload struct {
			Method  string            `json:"method"`
			Path    string            `json:"path"`
			Headers map[string]string `json:"headers"`
			Body    string            `json:"body"`
		}
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			resp.Error = &protocol.ErrPayload{Code: "BAD_PAYLOAD", Message: err.Error()}
			return resp
		}
		status, headers, body, err := h.httpExec(payload.Method, payload.Path, payload.Headers, payload.Body)
		if err != nil {
			resp.Error = &protocol.ErrPayload{Code: "GATEWAY_EXEC_FAILED", Message: err.Error()}
			return resp
		}
		resp.Payload = protocol.MustRaw(map[string]any{
			"status":  status,
			"headers": headers,
			"body":    body,
		})
		return resp
	default:
		resp.Error = &protocol.ErrPayload{Code: "UNKNOWN_OP", Message: "unsupported op"}
		return resp
	}
}
