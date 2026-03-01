package agentloopadapter

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
		Parameters: ResponseToolParameters{
			Type: "object",
			Properties: []ResponseToolProperty{
				{Name: "flag", Schema: ResponseToolSchema{Type: "string", Enum: []string{"success", "notify", "error"}}},
				{Name: "status_message", Schema: ResponseToolSchema{Type: "string"}},
			},
			Required: []string{"flag", "status_message"},
		},
	}
}

func (t *TaskCurrentSetFlagTool) Execute(ctx context.Context, _ struct{}, input string, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("TASK_CURRENT_SET_FLAG_EXEC_UNAVAILABLE", "Ensure task.current.set_flag exec callback is injected")
	}
	scope, ok := TaskScopeFromContext(ctx)
	if !ok || strings.TrimSpace(scope.TaskID) == "" {
		return "", NewToolError("TASK_CONTEXT_MISSING", "Ensure current invocation is bound to task scope")
	}
	req := struct {
		Flag          string `json:"flag"`
		StatusMessage string `json:"status_message"`
	}{}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "Check task.current.set_flag JSON: require flag and status_message")
	}
	flag := strings.TrimSpace(req.Flag)
	if flag != "success" && flag != "notify" && flag != "error" {
		return "", NewToolError("INVALID_FLAG_KEY", "flag must be one of success, notify, error")
	}
	statusMessage := strings.TrimSpace(req.StatusMessage)
	if statusMessage == "" {
		return "", NewToolError("INVALID_STATUS_MESSAGE", "Provide non-empty status_message")
	}
	out, err := t.Exec(ctx, strings.TrimSpace(scope.TaskID), flag, statusMessage)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(out) != "" {
		return out, nil
	}
	raw, _ := json.Marshal(struct {
		Ok            bool   `json:"ok"`
		TaskID        string `json:"task_id"`
		Flag          string `json:"flag"`
		StatusMessage string `json:"status_message"`
	}{
		Ok:            true,
		TaskID:        strings.TrimSpace(scope.TaskID),
		Flag:          flag,
		StatusMessage: statusMessage,
	})
	return string(raw), nil
}
