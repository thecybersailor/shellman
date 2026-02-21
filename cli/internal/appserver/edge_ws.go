package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"shellman/cli/internal/protocol"
)

const edgeWSReadLimitBytes int64 = 1 << 20 // 1 MiB

type peerConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

type edgeSession struct {
	agent        *peerConn
	clients      map[string]*peerConn
	connByClient map[*peerConn]string
	nextConnSeq  uint64
}

type EdgeWSHub struct {
	mu       sync.Mutex
	sessions map[string]*edgeSession
}

func NewEdgeWSHub() *EdgeWSHub {
	return &EdgeWSHub{sessions: map[string]*edgeSession{}}
}

func (h *EdgeWSHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	role, turnID, ok := parseEdgePath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(edgeWSReadLimitBytes)
	peer := &peerConn{conn: conn}
	h.attach(turnID, role, peer)
	defer h.detach(turnID, role, peer)

	for {
		msgType, data, err := conn.Read(r.Context())
		if err != nil {
			if websocket.CloseStatus(err) != -1 || errors.Is(err, context.Canceled) {
				return
			}
			return
		}
		if role == "client" {
			target, outbound, ok := h.wrapClientOutbound(turnID, peer, data)
			if !ok {
				continue
			}
			h.writePeer(target, msgType, outbound)
			continue
		}

		targets, outbound := h.routeAgentOutbound(turnID, data)
		for _, target := range targets {
			h.writePeer(target, msgType, outbound)
		}
	}
}

func parseEdgePath(path string) (role, turnID string, ok bool) {
	if strings.HasPrefix(path, "/ws/agent/") {
		id := strings.TrimPrefix(path, "/ws/agent/")
		if id != "" && !strings.Contains(id, "/") {
			return "agent", id, true
		}
	}
	if strings.HasPrefix(path, "/ws/client/") {
		id := strings.TrimPrefix(path, "/ws/client/")
		if id != "" && !strings.Contains(id, "/") {
			return "client", id, true
		}
	}
	return "", "", false
}

func (h *EdgeWSHub) attach(turnID, role string, conn *peerConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sessions[turnID]
	if s == nil {
		s = &edgeSession{
			clients:      map[string]*peerConn{},
			connByClient: map[*peerConn]string{},
		}
		h.sessions[turnID] = s
	}
	if role == "agent" {
		s.agent = conn
	} else {
		s.nextConnSeq++
		connID := fmt.Sprintf("conn_%d", s.nextConnSeq)
		s.clients[connID] = conn
		s.connByClient[conn] = connID
	}
}

func (h *EdgeWSHub) detach(turnID, role string, conn *peerConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sessions[turnID]
	if s == nil {
		return
	}
	if role == "agent" && s.agent == conn {
		s.agent = nil
	}
	if role == "client" {
		connID := s.connByClient[conn]
		delete(s.connByClient, conn)
		if connID != "" {
			delete(s.clients, connID)
		}
	}
	if s.agent == nil && len(s.clients) == 0 {
		delete(h.sessions, turnID)
	}
}

func (h *EdgeWSHub) wrapClientOutbound(turnID string, conn *peerConn, data []byte) (*peerConn, []byte, bool) {
	h.mu.Lock()
	s := h.sessions[turnID]
	if s == nil || s.agent == nil {
		h.mu.Unlock()
		return nil, nil, false
	}
	connID := s.connByClient[conn]
	agent := s.agent
	h.mu.Unlock()

	if connID == "" {
		return nil, nil, false
	}
	outbound, err := protocol.WrapMuxEnvelope(connID, data)
	if err != nil {
		return nil, nil, false
	}
	return agent, outbound, true
}

func (h *EdgeWSHub) routeAgentOutbound(turnID string, data []byte) ([]*peerConn, []byte) {
	connID, inner, err := protocol.UnwrapMuxEnvelope(data)
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sessions[turnID]
	if s == nil || len(s.clients) == 0 {
		return nil, nil
	}

	if err != nil {
		targets := make([]*peerConn, 0, len(s.clients))
		for _, c := range s.clients {
			targets = append(targets, c)
		}
		return targets, data
	}

	target := s.clients[connID]
	if target == nil {
		return nil, nil
	}
	return []*peerConn{target}, inner
}

func (h *EdgeWSHub) writePeer(target *peerConn, msgType websocket.MessageType, data []byte) {
	if target == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	target.writeMu.Lock()
	_ = target.conn.Write(ctx, msgType, data)
	target.writeMu.Unlock()
	cancel()
}

func (h *EdgeWSHub) PublishClientEvent(turnID, topic, projectID, taskID string, payload map[string]any) {
	if h == nil {
		return
	}
	turnID = strings.TrimSpace(turnID)
	topic = strings.TrimSpace(topic)
	if turnID == "" || topic == "" {
		return
	}
	outPayload := map[string]any{}
	if strings.TrimSpace(projectID) != "" {
		outPayload["project_id"] = strings.TrimSpace(projectID)
	}
	if strings.TrimSpace(taskID) != "" {
		outPayload["task_id"] = strings.TrimSpace(taskID)
	}
	for k, v := range payload {
		outPayload[k] = v
	}
	raw, err := json.Marshal(protocol.Message{
		ID:      fmt.Sprintf("evt_%d", time.Now().UTC().UnixNano()),
		Type:    "event",
		Op:      topic,
		Payload: protocol.MustRaw(outPayload),
	})
	if err != nil {
		return
	}

	h.mu.Lock()
	s := h.sessions[turnID]
	if s == nil || len(s.clients) == 0 {
		h.mu.Unlock()
		return
	}
	targets := make([]*peerConn, 0, len(s.clients))
	for _, c := range s.clients {
		targets = append(targets, c)
	}
	h.mu.Unlock()

	for _, target := range targets {
		h.writePeer(target, websocket.MessageText, raw)
	}
}
