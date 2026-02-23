package agentloop

import (
	"context"
	"encoding/json"
	"strings"
)

type PMConversationMode string

const (
	PMConversationModeDefault PMConversationMode = "default"
	PMConversationModePlan    PMConversationMode = "plan"
)

type pmConversationModeContextKey struct{}

func WithPMConversationMode(ctx context.Context, mode PMConversationMode) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(string(mode)) == "" {
		mode = PMConversationModeDefault
	}
	return context.WithValue(ctx, pmConversationModeContextKey{}, mode)
}

func pmConversationModeFromContext(ctx context.Context) PMConversationMode {
	if ctx == nil {
		return PMConversationModeDefault
	}
	v := ctx.Value(pmConversationModeContextKey{})
	mode, ok := v.(PMConversationMode)
	if !ok {
		return PMConversationModeDefault
	}
	if strings.TrimSpace(string(mode)) == "" {
		return PMConversationModeDefault
	}
	return mode
}

type PMToolsAdapterDeps struct {
	ExecCommand      func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError)
	WriteStdin       func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError)
	ApplyPatch       func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError)
	UpdatePlan       func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError)
	ViewImage        func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError)
	RequestUserInput func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError)
	MultiToolUse     func(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError)
	WebTool          func(ctx context.Context, toolName string, input json.RawMessage, callID string) (string, *ToolError)
}

type PMToolsAdapter struct {
	deps PMToolsAdapterDeps
}

func NewPMToolsAdapter(deps PMToolsAdapterDeps) *PMToolsAdapter {
	return &PMToolsAdapter{deps: deps}
}

func (a *PMToolsAdapter) Execute(ctx context.Context, toolName string, input json.RawMessage, callID string) (string, *ToolError) {
	if a == nil {
		return "", NewToolError("PM_TOOLS_ADAPTER_UNAVAILABLE", "确认 PM tools adapter 已初始化")
	}
	name := strings.TrimSpace(toolName)
	switch name {
	case "exec_command":
		if a.deps.ExecCommand == nil {
			return "", NewToolError("EXEC_COMMAND_EXEC_UNAVAILABLE", "确认 exec_command 回调已注入")
		}
		return a.deps.ExecCommand(ctx, input, callID)
	case "write_stdin":
		if a.deps.WriteStdin == nil {
			return "", NewToolError("WRITE_STDIN_EXEC_UNAVAILABLE", "确认 write_stdin 回调已注入")
		}
		return a.deps.WriteStdin(ctx, input, callID)
	case "apply_patch":
		if a.deps.ApplyPatch == nil {
			return "", NewToolError("APPLY_PATCH_EXEC_UNAVAILABLE", "确认 apply_patch 回调已注入")
		}
		return a.deps.ApplyPatch(ctx, input, callID)
	case "update_plan":
		if a.deps.UpdatePlan == nil {
			return "", NewToolError("UPDATE_PLAN_EXEC_UNAVAILABLE", "确认 update_plan 回调已注入")
		}
		return a.deps.UpdatePlan(ctx, input, callID)
	case "view_image":
		if a.deps.ViewImage == nil {
			return "", NewToolError("VIEW_IMAGE_EXEC_UNAVAILABLE", "确认 view_image 回调已注入")
		}
		return a.deps.ViewImage(ctx, input, callID)
	case "request_user_input":
		if pmConversationModeFromContext(ctx) != PMConversationModePlan {
			return "", NewToolError("REQUEST_USER_INPUT_PLAN_MODE_ONLY", "切换到 plan 模式后再调用 request_user_input")
		}
		if a.deps.RequestUserInput == nil {
			return "", NewToolError("REQUEST_USER_INPUT_EXEC_UNAVAILABLE", "确认 request_user_input 回调已注入")
		}
		return a.deps.RequestUserInput(ctx, input, callID)
	case "multi_tool_use.parallel":
		if a.deps.MultiToolUse == nil {
			return "", NewToolError("MULTI_TOOL_USE_EXEC_UNAVAILABLE", "确认 multi_tool_use.parallel 回调已注入")
		}
		return a.deps.MultiToolUse(ctx, input, callID)
	default:
		if strings.HasPrefix(name, "web.") {
			if a.deps.WebTool == nil {
				return "", NewToolError("WEB_TOOL_EXEC_UNAVAILABLE", "确认 web 工具回调已注入")
			}
			return a.deps.WebTool(ctx, name, input, callID)
		}
		return "", NewToolError("TOOL_NOT_FOUND", "确认工具名已注册并处于当前 allowed_tools 内")
	}
}
