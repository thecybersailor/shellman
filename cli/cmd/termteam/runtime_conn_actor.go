package main

import (
	"encoding/json"
	"strings"
	"sync"

	"termteam/cli/internal/protocol"
)

type ConnActor struct {
	id string

	mu       sync.RWMutex
	selected string
	outbound chan protocol.Message
}

func NewConnActor(connID string) *ConnActor {
	return &ConnActor{
		id:       strings.TrimSpace(connID),
		outbound: make(chan protocol.Message, 128),
	}
}

func (c *ConnActor) ID() string {
	if c == nil {
		return ""
	}
	return c.id
}

func (c *ConnActor) Select(target string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.selected = strings.TrimSpace(target)
}

func (c *ConnActor) Selected() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.selected
}

func (c *ConnActor) Outbound() chan<- protocol.Message {
	if c == nil {
		return nil
	}
	return c.outbound
}

func (c *ConnActor) OutboundRead() <-chan protocol.Message {
	if c == nil {
		return nil
	}
	return c.outbound
}

func (c *ConnActor) Enqueue(msg protocol.Message) bool {
	if c == nil {
		return false
	}
	select {
	case c.outbound <- msg:
		return true
	default:
		if isAppendTermOutput(msg) {
			return false
		}

		// Keep reset/system events best-effort when queue is full.
		select {
		case <-c.outbound:
		default:
		}
		select {
		case c.outbound <- msg:
			return true
		default:
			return false
		}
	}
}

func (c *ConnActor) PendingMessages() int {
	if c == nil {
		return 0
	}
	return len(c.outbound)
}

func decodeTermOutputMeta(msg protocol.Message) (target string, mode string, dataLen int) {
	if msg.Op != "term.output" {
		return "", "", 0
	}
	var payload struct {
		Target string `json:"target"`
		Mode   string `json:"mode"`
		Data   string `json:"data"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return "", "", 0
	}
	return payload.Target, payload.Mode, len(payload.Data)
}
