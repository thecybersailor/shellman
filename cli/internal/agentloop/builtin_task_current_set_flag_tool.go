package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type TaskCurrentSetFlagTool struct {
	Exec func(ctx context.Context, taskID, flag, statusMessage string) (string, error)
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

func (t *TaskCurrentSetFlagTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, error) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", errors.New("TASK_CURRENT_SET_FLAG_EXEC_UNAVAILABLE")
	}
	scope, ok := TaskScopeFromContext(ctx)
	if !ok || strings.TrimSpace(scope.TaskID) == "" {
		return "", errors.New("TASK_CONTEXT_MISSING")
	}
	req := struct {
		Flag          string `json:"flag"`
		StatusMessage string `json:"status_message"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", err
	}
	flag := strings.TrimSpace(req.Flag)
	if flag != "success" && flag != "notify" && flag != "error" {
		return "", errors.New("INVALID_FLAG_KEY")
	}
	statusMessage := strings.TrimSpace(req.StatusMessage)
	if statusMessage == "" {
		return "", errors.New("INVALID_STATUS_MESSAGE")
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
