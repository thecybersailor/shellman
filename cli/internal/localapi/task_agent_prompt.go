package localapi

import (
	"encoding/json"
	"fmt"
	"strings"
)

type TaskAgentTTYContext struct {
	CurrentCommand string
	OutputTail     string
	Cwd            string
}

type TaskAgentParentContext struct {
	Name          string
	Description   string
	Flag          string
	StatusMessage string
}

type TaskAgentChildContext struct {
	TaskID        string
	Name          string
	Description   string
	Flag          string
	StatusMessage string
	ReportMessage string
}

type TaskAgentAutoProgressPromptInput struct {
	TaskID            string
	RunID             string
	Name              string
	Description       string
	Summary           string
	PrevFlag          string
	PrevStatusMessage string
	TTY               TaskAgentTTYContext
	ParentTask        *TaskAgentParentContext
	ChildTasks        []TaskAgentChildContext
}

func buildTaskAgentAutoProgressPrompt(input TaskAgentAutoProgressPromptInput) string {
	input.TaskID = strings.TrimSpace(input.TaskID)
	input.RunID = strings.TrimSpace(input.RunID)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Summary = strings.TrimSpace(input.Summary)
	input.PrevFlag = strings.TrimSpace(input.PrevFlag)
	input.PrevStatusMessage = strings.TrimSpace(input.PrevStatusMessage)
	if input.Summary == "" {
		input.Summary = "tty_output detected pane idle and stable output"
	}

	var b strings.Builder
	b.WriteString("TTY_OUTPUT_EVENT\n")
	b.WriteString("task_id: ")
	b.WriteString(input.TaskID)
	b.WriteString("\n")
	if input.RunID != "" {
		b.WriteString("run_id: ")
		b.WriteString(input.RunID)
		b.WriteString("\n")
	}
	b.WriteString("name: ")
	b.WriteString(input.Name)
	b.WriteString("\n")
	b.WriteString("description: ")
	b.WriteString(input.Description)
	b.WriteString("\n")
	b.WriteString("summary: ")
	b.WriteString(input.Summary)
	b.WriteString("\n\n")
	b.WriteString("context_json:\n")
	b.WriteString(mustBuildTaskContextJSON(input.PrevFlag, input.PrevStatusMessage, input.TTY, input.ParentTask, input.ChildTasks))
	b.WriteString("\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- respond with short action-oriented summary after tool calls.\n")
	b.WriteString(fmt.Sprintf("- Focus on this task: %s.\n", input.TaskID))
	return strings.TrimSpace(b.String())
}

func buildTaskAgentUserPrompt(userInput string, prevFlag string, prevStatusMessage string, tty TaskAgentTTYContext, parent *TaskAgentParentContext, children []TaskAgentChildContext) string {
	userInput = strings.TrimSpace(userInput)
	var b strings.Builder
	b.WriteString("USER_INPUT_EVENT\n")
	b.WriteString("user_input:\n")
	b.WriteString(userInput)
	b.WriteString("\n\n")
	b.WriteString("context_json:\n")
	b.WriteString(mustBuildTaskContextJSON(prevFlag, prevStatusMessage, tty, parent, children))
	b.WriteString("\n")
	return strings.TrimSpace(b.String())
}

func mustBuildTaskContextJSON(prevFlag string, prevStatus string, tty TaskAgentTTYContext, parent *TaskAgentParentContext, children []TaskAgentChildContext) string {
	prevFlag = strings.TrimSpace(prevFlag)
	prevStatus = strings.TrimSpace(prevStatus)
	tty.CurrentCommand = strings.TrimSpace(tty.CurrentCommand)
	tty.OutputTail = strings.TrimSpace(tty.OutputTail)
	tty.Cwd = strings.TrimSpace(tty.Cwd)
	parentObj := map[string]any{
		"name":           "",
		"description":    "",
		"flag":           "",
		"status_message": "",
	}
	if parent != nil {
		parentObj["name"] = strings.TrimSpace(parent.Name)
		parentObj["description"] = strings.TrimSpace(parent.Description)
		parentObj["flag"] = strings.TrimSpace(parent.Flag)
		parentObj["status_message"] = strings.TrimSpace(parent.StatusMessage)
	}
	childList := make([]map[string]any, 0, len(children))
	for _, child := range children {
		childList = append(childList, map[string]any{
			"task_id":        strings.TrimSpace(child.TaskID),
			"name":           strings.TrimSpace(child.Name),
			"description":    strings.TrimSpace(child.Description),
			"flag":           strings.TrimSpace(child.Flag),
			"status_message": strings.TrimSpace(child.StatusMessage),
			"report_message": strings.TrimSpace(child.ReportMessage),
		})
	}
	raw, _ := json.Marshal(map[string]any{
		"task_context": map[string]any{
			"prev_flag":           prevFlag,
			"prev_status_message": prevStatus,
			"tty": map[string]any{
				"current_command": tty.CurrentCommand,
				"output_tail":     tty.OutputTail,
				"cwd":             tty.Cwd,
			},
		},
		"parent_task": parentObj,
		"child_tasks": childList,
	})
	return string(raw)
}

func buildTaskAgentAutoProgressDisplayContent(taskID, summary, runID string) string {
	taskID = strings.TrimSpace(taskID)
	summary = strings.TrimSpace(summary)
	runID = strings.TrimSpace(runID)
	if summary == "" {
		summary = "auto-complete: pane idle and output stable"
	}
	meta := map[string]any{
		"display_type": "runtime",
		"source":       "tty_output",
		"event":        "tty_output",
		"task_id":      taskID,
	}
	if runID != "" {
		meta["run_id"] = runID
	}
	raw, err := json.Marshal(map[string]any{
		"text": summary,
		"meta": meta,
	})
	if err != nil {
		return summary
	}
	return string(raw)
}
