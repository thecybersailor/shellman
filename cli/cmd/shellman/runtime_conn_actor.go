package main

import (
	"encoding/json"
	"strings"
	"sync"

	"shellman/cli/internal/protocol"
)

type ConnActor struct {
	id string

	mu         sync.RWMutex
	selected   string
	watchOrder []string
	watchSet   map[string]struct{}
	outbound   chan protocol.Message
}

func NewConnActor(connID string) *ConnActor {
	return &ConnActor{
		id:       strings.TrimSpace(connID),
		watchSet: map[string]struct{}{},
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

func (c *ConnActor) SelectAndWatch(target string, limit int) (evicted string) {
	if c == nil {
		return ""
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if limit <= 0 {
		limit = 1
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.selected = target
	if c.watchSet == nil {
		c.watchSet = map[string]struct{}{}
	}
	if _, exists := c.watchSet[target]; exists {
		for i, item := range c.watchOrder {
			if item == target {
				c.watchOrder = append(c.watchOrder[:i], c.watchOrder[i+1:]...)
				break
			}
		}
	}
	c.watchOrder = append(c.watchOrder, target)
	c.watchSet[target] = struct{}{}

	for len(c.watchOrder) > limit {
		dropped := c.watchOrder[0]
		c.watchOrder = c.watchOrder[1:]
		delete(c.watchSet, dropped)
		if evicted == "" {
			evicted = dropped
		}
	}
	return evicted
}

func (c *ConnActor) WatchedTargets() []string {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.watchOrder) == 0 {
		return nil
	}
	out := make([]string, len(c.watchOrder))
	copy(out, c.watchOrder)
	return out
}

func (c *ConnActor) Selected() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.selected
}

func (c *ConnActor) Outbound() chan protocol.Message {
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
