package localapi

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"shellman/cli/internal/global"
	"shellman/cli/internal/projectstate"
)

type taskStreamWithToolsRunner struct{}

func (r *taskStreamWithToolsRunner) Run(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (r *taskStreamWithToolsRunner) RunStreamWithTools(
	_ context.Context,
	_ string,
	onTextDelta func(string),
	onToolEvent func(map[string]any),
) (string, error) {
	if onTextDelta != nil {
		onTextDelta("stream answer")
	}
	if onToolEvent != nil {
		onToolEvent(map[string]any{
			"type":          "tool_input",
			"call_id":       "task_call_1",
			"response_id":   "task_resp_1",
			"tool_name":     "exec_command",
			"state":         "input-available",
			"input":         "{\"cmd\":\"pwd\"}",
			"input_preview": "pwd",
			"input_raw_len": 13,
		})
		onToolEvent(map[string]any{
			"type":        "tool_output",
			"call_id":     "task_call_1",
			"response_id": "task_resp_1",
			"tool_name":   "exec_command",
			"state":       "output-available",
			"output":      "{\"ok\":true}",
			"output_len":  11,
		})
	}
	return "stream answer", nil
}

func TestTaskAgentActor_PersistsMergedToolEventsInAssistantMessage(t *testing.T) {
	repo := t.TempDir()
	projects := &memProjectsStore{
		projects: []global.ActiveProject{{ProjectID: "p1", RepoRoot: filepath.Clean(repo)}},
	}
	runner := &taskStreamWithToolsRunner{}
	srv := NewServer(Deps{ConfigStore: &staticConfigStore{}, ProjectsStore: projects, AgentLoopRunner: runner})
	taskID, err := srv.createTask("p1", "", "root")
	if err != nil {
		t.Fatalf("createTask failed: %v", err)
	}
	store := projectstate.NewStore(repo)

	if err := srv.sendTaskAgentLoop(context.Background(), TaskAgentLoopEvent{
		TaskID:         taskID,
		ProjectID:      "p1",
		Source:         "user_input",
		DisplayContent: "hello",
		AgentPrompt:    "hello",
	}); err != nil {
		t.Fatalf("sendTaskAgentLoop failed: %v", err)
	}

	waitUntil(t, 3*time.Second, func() bool {
		items, listErr := store.ListTaskMessages(taskID, 10)
		if listErr != nil || len(items) < 2 {
			return false
		}
		last := items[len(items)-1]
		if last.Role != "assistant" || last.Status != projectstate.StatusCompleted {
			return false
		}
		var payload struct {
			Text  string           `json:"text"`
			Tools []map[string]any `json:"tools"`
		}
		if err := json.Unmarshal([]byte(last.Content), &payload); err != nil {
			return false
		}
		if len(payload.Tools) != 1 {
			return false
		}
		tool := payload.Tools[0]
		return tool["tool_name"] == "exec_command" &&
			tool["state"] == "output-available" &&
			tool["input"] == "{\"cmd\":\"pwd\"}" &&
			tool["output"] == "{\"ok\":true}"
	})
}
