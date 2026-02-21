package agentloop

import (
	"context"
	"encoding/json"
	"strings"
)

type TaskCurrentSetFlagTool struct {
	Exec func(ctx context.Context, taskID, flag, statusMessage string) (string, *ToolError)
}

func (t *TaskCurrentSetFlagTool) Name() string { return "task.current.set_flag" }

func (t *TaskCurrentSetFlagTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Set current task flag and status message.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"flag": map[string]any{
					"type": "string",
					"enum": []string{"success", "notify", "error"},
				},
				"status_message": map[string]any{"type": "string"},
			},
			"required": []string{"flag", "status_message"},
		},
	}
}

func (t *TaskCurrentSetFlagTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("TASK_CURRENT_SET_FLAG_EXEC_UNAVAILABLE", "确认 task.current.set_flag 工具已注入可执行回调")
	}
	scope, ok := TaskScopeFromContext(ctx)
	if !ok || strings.TrimSpace(scope.TaskID) == "" {
		return "", NewToolError("TASK_CONTEXT_MISSING", "确认当前调用已绑定 task scope")
	}
	req := struct {
		Flag          string `json:"flag"`
		StatusMessage string `json:"status_message"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "检查 task.current.set_flag 参数 JSON: 需要 flag 与 status_message")
	}
	flag := strings.TrimSpace(req.Flag)
	if flag != "success" && flag != "notify" && flag != "error" {
		return "", NewToolError("INVALID_FLAG_KEY", "flag 只能是 success、notify、error")
	}
	statusMessage := strings.TrimSpace(req.StatusMessage)
	if statusMessage == "" {
		return "", NewToolError("INVALID_STATUS_MESSAGE", "提供非空 status_message")
	}
	out, err := t.Exec(ctx, strings.TrimSpace(scope.TaskID), flag, statusMessage)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) != "" {
		return out, nil
	}
	raw, _ := json.Marshal(map[string]any{
		"ok":             true,
		"task_id":        strings.TrimSpace(scope.TaskID),
		"flag":           flag,
		"status_message": statusMessage,
	})
	return string(raw), nil
}
