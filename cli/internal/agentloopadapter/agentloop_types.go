package agentloopadapter

import (
	"context"

	"github.com/flaboy/agentloop"
	core "github.com/flaboy/agentloop/core"
)

type ToolError = core.ToolError
type ResponseToolSpec = core.ResponseToolSpec
type ResponseToolParameters = core.ResponseToolParameters
type ResponseToolProperty = core.ResponseToolProperty
type ResponseToolSchema = core.ResponseToolSchema
type AllowedToolNamesResolver = agentloop.AllowedToolNamesResolver

func NewToolError(code, message string) *ToolError {
	return core.NewToolError(code, message)
}

func IntPtr(v int) *int {
	return core.IntPtr(v)
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
