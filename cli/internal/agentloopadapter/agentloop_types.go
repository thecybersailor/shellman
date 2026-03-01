package agentloopadapter

import (
	"context"
	"strings"

	core "github.com/flaboy/agentloop/core"
)

type ToolError = core.ToolError
type ResponseToolSpec = core.ResponseToolSpec
type ResponseToolParameters = core.ResponseToolParameters
type ResponseToolProperty = core.ResponseToolProperty
type ResponseToolSchema = core.ResponseToolSchema
type AllowedToolNamesResolver func() []string
type allowedToolNamesContextKey struct{}
type allowedToolNamesResolverContextKey struct{}

func NewToolError(code, message string) *ToolError {
	return core.NewToolError(code, message)
}

func IntPtr(v int) *int {
	return core.IntPtr(v)
}

func WithAllowedToolNames(ctx context.Context, toolNames []string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	clean := make([]string, 0, len(toolNames))
	seen := map[string]struct{}{}
	for _, item := range toolNames {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		clean = append(clean, name)
	}
	return context.WithValue(ctx, allowedToolNamesContextKey{}, clean)
}

func WithAllowedToolNamesResolver(ctx context.Context, resolver AllowedToolNamesResolver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if resolver == nil {
		return ctx
	}
	return context.WithValue(ctx, allowedToolNamesResolverContextKey{}, resolver)
}

func AllowedToolNamesFromContext(ctx context.Context) ([]string, bool) {
	if ctx == nil {
		return nil, false
	}
	if resolver, ok := ctx.Value(allowedToolNamesResolverContextKey{}).(AllowedToolNamesResolver); ok && resolver != nil {
		names := resolver()
		out := make([]string, 0, len(names))
		seen := map[string]struct{}{}
		for _, item := range names {
			name := strings.TrimSpace(item)
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
		return out, true
	}
	names, ok := ctx.Value(allowedToolNamesContextKey{}).([]string)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(names))
	for _, item := range names {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out, true
}
