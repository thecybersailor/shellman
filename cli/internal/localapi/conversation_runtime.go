package localapi

import (
	"context"
	"errors"
	"strings"
	"sync"
)

const defaultConversationQueueSize = 64

type ConversationEvent struct {
	Key     string
	Payload any
}

type ConversationHandler func(context.Context, ConversationEvent) error

type conversationActor struct {
	key           string
	queue         chan ConversationEvent
	handler       ConversationHandler
	mu            sync.Mutex
	inflight      context.CancelFunc
}

type ConversationRuntime struct {
	mu        sync.Mutex
	actors    map[string]*conversationActor
	handler   ConversationHandler
	queueSize int
}

func NewConversationRuntime(handler ConversationHandler) *ConversationRuntime {
	return &ConversationRuntime{
		actors:    map[string]*conversationActor{},
		handler:   handler,
		queueSize: defaultConversationQueueSize,
	}
}

func (r *ConversationRuntime) Enqueue(ctx context.Context, evt ConversationEvent) error {
	if r == nil {
		return errors.New("conversation runtime is unavailable")
	}
	key := strings.TrimSpace(evt.Key)
	if key == "" {
		return errors.New("conversation key is required")
	}
	evt.Key = key
	actor := r.getOrCreateActor(key)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case actor.queue <- evt:
		return nil
	}
}

func (r *ConversationRuntime) Cancel(key string) {
	if r == nil {
		return
	}
	actor := r.getActor(strings.TrimSpace(key))
	if actor == nil {
		return
	}
	actor.mu.Lock()
	cancel := actor.inflight
	actor.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *ConversationRuntime) QueueLen(key string) int {
	if r == nil {
		return 0
	}
	actor := r.getActor(strings.TrimSpace(key))
	if actor == nil {
		return 0
	}
	return len(actor.queue)
}

func (r *ConversationRuntime) getActor(key string) *conversationActor {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.actors[key]
}

func (r *ConversationRuntime) getOrCreateActor(key string) *conversationActor {
	r.mu.Lock()
	defer r.mu.Unlock()
	if actor, ok := r.actors[key]; ok {
		return actor
	}
	actor := &conversationActor{
		key:     key,
		queue:   make(chan ConversationEvent, r.queueSize),
		handler: r.handler,
	}
	actor.start()
	r.actors[key] = actor
	return actor
}

func (a *conversationActor) start() {
	go func() {
		for evt := range a.queue {
			if a.handler == nil {
				continue
			}
			runCtx, cancel := context.WithCancel(context.Background())
			a.mu.Lock()
			a.inflight = cancel
			a.mu.Unlock()
			_ = a.handler(runCtx, evt)
			cancel()
			a.mu.Lock()
			a.inflight = nil
			a.mu.Unlock()
		}
	}()
}
