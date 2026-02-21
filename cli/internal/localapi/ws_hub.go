package localapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
	"shellman/cli/internal/protocol"
)

type WSHub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
	seq     atomic.Uint64
}

func NewWSHub() *WSHub {
	return &WSHub{clients: map[*websocket.Conn]struct{}{}}
}

func (h *WSHub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, conn)
		h.mu.Unlock()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()

	ctx := r.Context()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

func (h *WSHub) Publish(topic, projectID, taskID string, payload map[string]any) {
	outPayload := map[string]any{}
	if projectID != "" {
		outPayload["project_id"] = projectID
	}
	if taskID != "" {
		outPayload["task_id"] = taskID
	}
	for k, v := range payload {
		outPayload[k] = v
	}

	evt := protocol.Message{
		ID:      fmt.Sprintf("evt_%d", h.seq.Add(1)),
		Type:    "event",
		Op:      topic,
		Payload: protocol.MustRaw(outPayload),
	}
	msg, err := json.Marshal(evt)
	if err != nil {
		return
	}

	h.mu.RLock()
	clients := make([]*websocket.Conn, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_ = c.Write(ctx, websocket.MessageText, msg)
		cancel()
	}
}
