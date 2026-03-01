package agentloopadapter

import (
	"context"

	"github.com/flaboy/agentloop"
)

type ToolError = agentloop.ToolError
type ResponseToolSpec = agentloop.ResponseToolSpec
type AllowedToolNamesResolver = agentloop.AllowedToolNamesResolver

func NewToolError(code, message string) *ToolError {
	return agentloop.NewToolError(code, message)
}

func WithAllowedToolNames(ctx context.Context, toolNames []string) context.Context {
	return agentloop.WithAllowedToolNames(ctx, toolNames)
}

func WithAllowedToolNamesResolver(ctx context.Context, resolver AllowedToolNamesResolver) context.Context {
	return agentloop.WithAllowedToolNamesResolver(ctx, resolver)
}

func AllowedToolNamesFromContext(ctx context.Context) ([]string, bool) {
	return agentloop.AllowedToolNamesFromContext(ctx)
}
