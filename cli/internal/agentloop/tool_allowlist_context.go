package agentloop

import (
	"context"
	"strings"
)

type allowedToolNamesContextKey struct{}
type allowedToolNamesResolverContextKey struct{}
type AllowedToolNamesResolver func() []string

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
		clean := make([]string, 0, len(names))
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
			clean = append(clean, name)
		}
		return clean, true
	}
	v := ctx.Value(allowedToolNamesContextKey{})
	names, ok := v.([]string)
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
