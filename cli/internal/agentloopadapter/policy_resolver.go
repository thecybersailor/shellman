package agentloopadapter

import (
	"context"
	"strings"

	core "github.com/flaboy/agentloop/core"
)

type PolicyResolver struct {
	taskAllowed []string
	pmAllowed   []string
}

func NewPolicyResolver(taskAllowed, pmAllowed []string) *PolicyResolver {
	return &PolicyResolver{
		taskAllowed: normalizeToolNames(taskAllowed),
		pmAllowed:   normalizeToolNames(pmAllowed),
	}
}

func (r *PolicyResolver) Resolve(_ context.Context, req core.PolicyRequest[State]) (core.ToolPolicy, error) {
	state := normalizeState(req.State)
	allowed := r.taskAllowed
	if state.Mode == ModePM {
		allowed = r.pmAllowed
	}
	return core.ToolPolicy{
		AllowedToolNames: append([]string{}, allowed...),
		Mode:             string(state.Mode),
		PolicyVersion:    "shellman-v1",
	}, nil
}

func normalizeToolNames(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
