package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
)

type ToolRegistry struct {
	mu     sync.RWMutex
	byName map[string]Tool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{byName: map[string]Tool{}}
}

func (r *ToolRegistry) Register(tool Tool) error {
	if r == nil {
		return errors.New("registry is nil")
	}
	if tool == nil {
		return errors.New("tool is nil")
	}
	name := strings.TrimSpace(tool.Name())
	if name == "" {
		return errors.New("tool name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.byName[name] = tool
	return nil
}

func (r *ToolRegistry) Get(name string) (Tool, bool) {
	if r == nil {
		return nil, false
	}
	name = strings.TrimSpace(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.byName[name]
	return tool, ok
}

func (r *ToolRegistry) Specs() []ResponseToolSpec {
	if r == nil {
		return []ResponseToolSpec{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.byName))
	for name := range r.byName {
		names = append(names, name)
	}
	slices.Sort(names)
	out := make([]ResponseToolSpec, 0, len(names))
	for _, name := range names {
		out = append(out, r.byName[name].Spec())
	}
	return out
}

func (r *ToolRegistry) SpecsByNames(names []string) []ResponseToolSpec {
	if r == nil {
		return []ResponseToolSpec{}
	}
	allow := map[string]struct{}{}
	for _, item := range names {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		allow[name] = struct{}{}
	}
	if len(allow) == 0 {
		return []ResponseToolSpec{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]string, 0, len(allow))
	for name := range allow {
		if _, ok := r.byName[name]; ok {
			keys = append(keys, name)
		}
	}
	slices.Sort(keys)
	out := make([]ResponseToolSpec, 0, len(keys))
	for _, name := range keys {
		out = append(out, r.byName[name].Spec())
	}
	return out
}

func (r *ToolRegistry) Execute(ctx context.Context, name string, input json.RawMessage, callID string) (string, *ToolError) {
	tool, ok := r.Get(name)
	if !ok {
		return "", NewToolError("TOOL_NOT_FOUND", "确认工具名已注册并处于当前 allowed_tools 内")
	}
	out, err := tool.Execute(ctx, input, callID)
	if err != nil {
		return "", err
	}
	return out, nil
}
