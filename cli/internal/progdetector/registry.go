package progdetector

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

type Registry struct {
	mu    sync.RWMutex
	byID  map[string]Detector
	order []string
}

func NewRegistry() *Registry {
	return &Registry{
		byID:  map[string]Detector{},
		order: []string{},
	}
}

var ProgramDetectorRegistry = NewRegistry()

func (r *Registry) Register(detector Detector) error {
	if r == nil {
		return errors.New("registry is nil")
	}
	if detector == nil {
		return errors.New("detector is nil")
	}
	id := strings.TrimSpace(detector.ProgramID())
	if id == "" {
		return errors.New("program id is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byID[id]; exists {
		return fmt.Errorf("detector %q already registered", id)
	}
	r.byID[id] = detector
	r.order = append(r.order, id)
	return nil
}

func (r *Registry) MustRegister(detector Detector) {
	if err := r.Register(detector); err != nil {
		panic(err)
	}
}

func (r *Registry) Get(programID string) (Detector, bool) {
	if r == nil {
		return nil, false
	}
	id := strings.TrimSpace(programID)
	if id == "" {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	detector, ok := r.byID[id]
	return detector, ok
}

func (r *Registry) DetectByCurrentCommand(currentCommand string) (Detector, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, id := range r.order {
		detector := r.byID[id]
		if detector != nil && detector.MatchCurrentCommand(currentCommand) {
			return detector, true
		}
	}
	return nil, false
}

func (r *Registry) List() []Detector {
	if r == nil {
		return []Detector{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Detector, 0, len(r.order))
	for _, id := range r.order {
		if detector := r.byID[id]; detector != nil {
			out = append(out, detector)
		}
	}
	return out
}
