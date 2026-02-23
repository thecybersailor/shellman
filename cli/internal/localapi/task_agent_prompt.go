package localapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type TaskAgentTTYContext struct {
	CurrentCommand string
	OutputTail     string
	Cwd            string
	CursorX        int
	CursorY        int
	HasCursor      bool
	CursorHint     string
	CursorSemantic string
}

type TaskAgentParentContext struct {
	Name          string
	Description   string
	TaskRole      string
	Flag          string
	StatusMessage string
}

type TaskAgentChildContext struct {
	TaskID        string
	Name          string
	Description   string
	TaskRole      string
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
	HistoryBlock      string
	PrevFlag          string
	PrevStatusMessage string
	TTY               TaskAgentTTYContext
	ParentTask        *TaskAgentParentContext
	ChildTasks        []TaskAgentChildContext
	TaskContextDocs   []taskCompletionContextDocument
	SkillIndex        []SkillIndexEntry
	SkillIndexError   string
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
	taskContextJSON := mustBuildTaskContextJSON(input.PrevFlag, input.PrevStatusMessage, input.TTY, input.ParentTask, input.ChildTasks)
	systemContextJSON := mustBuildTaskSystemContextJSON(input.TaskContextDocs, input.SkillIndex, input.SkillIndexError)
	eventContextJSON := mustBuildTaskEventContextJSON("tty_output", "", input.Summary, input.HistoryBlock, taskContextJSON)

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
	b.WriteString("system_context_json:\n")
	b.WriteString(systemContextJSON)
	b.WriteString("\n\n")
	b.WriteString("event_context_json:\n")
	b.WriteString(eventContextJSON)
	b.WriteString("\n\n")
	b.WriteString("conversation_history:\n")
	if strings.TrimSpace(input.HistoryBlock) == "" {
		b.WriteString("(none)\n\n")
	} else {
		b.WriteString(strings.TrimSpace(input.HistoryBlock))
		b.WriteString("\n\n")
	}
	b.WriteString("terminal_screen_state_json:\n")
	b.WriteString(taskContextJSON)
	b.WriteString("\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- respond with short action-oriented summary after tool calls.\n")
	b.WriteString(fmt.Sprintf("- Focus on this task: %s.\n", input.TaskID))
	return strings.TrimSpace(b.String())
}

func buildTaskAgentUserPrompt(userInput string, prevFlag string, prevStatusMessage string, tty TaskAgentTTYContext, parent *TaskAgentParentContext, children []TaskAgentChildContext, historyBlock string) string {
	return buildTaskAgentUserPromptWithContexts(userInput, prevFlag, prevStatusMessage, tty, parent, children, historyBlock, nil, nil, "")
}

func buildTaskAgentUserPromptWithContexts(
	userInput string,
	prevFlag string,
	prevStatusMessage string,
	tty TaskAgentTTYContext,
	parent *TaskAgentParentContext,
	children []TaskAgentChildContext,
	historyBlock string,
	taskContextDocs []taskCompletionContextDocument,
	skillIndex []SkillIndexEntry,
	skillIndexError string,
) string {
	userInput = strings.TrimSpace(userInput)
	taskContextJSON := mustBuildTaskContextJSON(prevFlag, prevStatusMessage, tty, parent, children)
	systemContextJSON := mustBuildTaskSystemContextJSON(taskContextDocs, skillIndex, skillIndexError)
	eventContextJSON := mustBuildTaskEventContextJSON("user_input", userInput, "", historyBlock, taskContextJSON)
	var b strings.Builder
	b.WriteString("USER_INPUT_EVENT\n")
	b.WriteString("user_input:\n")
	b.WriteString(userInput)
	b.WriteString("\n\n")
	b.WriteString("system_context_json:\n")
	b.WriteString(systemContextJSON)
	b.WriteString("\n\n")
	b.WriteString("event_context_json:\n")
	b.WriteString(eventContextJSON)
	b.WriteString("\n\n")
	b.WriteString("conversation_history:\n")
	if strings.TrimSpace(historyBlock) == "" {
		b.WriteString("(none)\n\n")
	} else {
		b.WriteString(strings.TrimSpace(historyBlock))
		b.WriteString("\n\n")
	}
	b.WriteString("terminal_screen_state_json:\n")
	b.WriteString(taskContextJSON)
	b.WriteString("\n")
	return strings.TrimSpace(b.String())
}

func mustBuildTaskContextJSON(prevFlag string, prevStatus string, tty TaskAgentTTYContext, parent *TaskAgentParentContext, children []TaskAgentChildContext) string {
	raw, _ := json.Marshal(buildTaskContextObject(prevFlag, prevStatus, tty, parent, children))
	return string(raw)
}

func buildTaskContextObject(prevFlag string, prevStatus string, tty TaskAgentTTYContext, parent *TaskAgentParentContext, children []TaskAgentChildContext) map[string]any {
	prevFlag = strings.TrimSpace(prevFlag)
	prevStatus = strings.TrimSpace(prevStatus)
	tty.CurrentCommand = strings.TrimSpace(tty.CurrentCommand)
	tty.OutputTail = strings.TrimSpace(tty.OutputTail)
	tty.Cwd = strings.TrimSpace(tty.Cwd)
	tty.CursorHint = strings.TrimSpace(tty.CursorHint)
	tty.CursorSemantic = strings.TrimSpace(tty.CursorSemantic)
	if tty.CursorHint == "" {
		tty.CursorHint = inferCursorHint(tty)
	}
	if tty.CursorSemantic == "" {
		tty.CursorSemantic = inferCursorSemantic(tty)
	}
	parentObj := map[string]any{
		"name":           "",
		"description":    "",
		"task_role":      "",
		"flag":           "",
		"status_message": "",
	}
	if parent != nil {
		parentObj["name"] = strings.TrimSpace(parent.Name)
		parentObj["description"] = strings.TrimSpace(parent.Description)
		parentObj["task_role"] = strings.TrimSpace(parent.TaskRole)
		parentObj["flag"] = strings.TrimSpace(parent.Flag)
		parentObj["status_message"] = strings.TrimSpace(parent.StatusMessage)
	}
	childList := make([]map[string]any, 0, len(children))
	for _, child := range children {
		childList = append(childList, map[string]any{
			"task_id":        strings.TrimSpace(child.TaskID),
			"name":           strings.TrimSpace(child.Name),
			"description":    strings.TrimSpace(child.Description),
			"task_role":      strings.TrimSpace(child.TaskRole),
			"flag":           strings.TrimSpace(child.Flag),
			"status_message": strings.TrimSpace(child.StatusMessage),
			"report_message": strings.TrimSpace(child.ReportMessage),
		})
	}
	return map[string]any{
		"task_context": map[string]any{
			"prev_flag":           prevFlag,
			"prev_status_message": prevStatus,
		},
		"terminal_screen_state": map[string]any{
			"current_command": tty.CurrentCommand,
			"viewport_text":   tty.OutputTail,
			"cwd":             tty.Cwd,
			"cursor": map[string]any{
				"row":     tty.CursorY,
				"col":     tty.CursorX,
				"visible": tty.HasCursor,
			},
			"cursor_hint":     tty.CursorHint,
			"cursor_semantic": tty.CursorSemantic,
		},
		"parent_task": parentObj,
		"child_tasks": childList,
	}
}

func mustBuildTaskSystemContextJSON(taskContextDocs []taskCompletionContextDocument, skillIndex []SkillIndexEntry, skillIndexError string) string {
	skills := cloneSkillEntries(skillIndex)
	sort.Slice(skills, func(i, j int) bool {
		if skills[i].Name == skills[j].Name {
			return skills[i].Path < skills[j].Path
		}
		return skills[i].Name < skills[j].Name
	})
	docs := make([]taskCompletionContextDocument, 0, len(taskContextDocs))
	for _, doc := range taskContextDocs {
		docs = append(docs, taskCompletionContextDocument{
			Path:    strings.TrimSpace(doc.Path),
			Content: strings.TrimSpace(doc.Content),
		})
	}
	raw, _ := json.Marshal(map[string]any{
		"contract_version": "v2",
		"instructions": map[string]any{
			"skill_body_loading_policy": "inject_index_only_read_body_on_demand",
		},
		"task_completion_context_docs": docs,
		"skills_index":                skills,
		"skills_index_error":          strings.TrimSpace(skillIndexError),
	})
	return string(raw)
}

func mustBuildTaskEventContextJSON(eventType, userInput, summary, historyBlock, taskContextJSON string) string {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		eventType = "user_input"
	}
	event := map[string]any{
		"event_type":           eventType,
		"user_input":           strings.TrimSpace(userInput),
		"summary":              strings.TrimSpace(summary),
		"conversation_history": strings.TrimSpace(historyBlock),
	}
	var taskContext map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(taskContextJSON)), &taskContext); err == nil {
		event["task_context"] = taskContext
	}
	raw, _ := json.Marshal(event)
	return string(raw)
}

func inferCursorHint(tty TaskAgentTTYContext) string {
	if !tty.HasCursor {
		return "cursor_unavailable"
	}
	tail := strings.TrimSpace(tty.OutputTail)
	if strings.HasSuffix(tail, "$") || strings.HasSuffix(tail, "#") || strings.HasSuffix(tail, ">") {
		return "cursor_at_shell_prompt_ready_for_input"
	}
	return "cursor_on_terminal_screen"
}

func inferCursorSemantic(tty TaskAgentTTYContext) string {
	if !tty.HasCursor {
		return "cursor_unavailable"
	}
	tail := strings.TrimSpace(tty.OutputTail)
	if strings.HasSuffix(tail, "$") || strings.HasSuffix(tail, "#") || strings.HasSuffix(tail, ">") {
		return "shell_prompt_ready"
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(tty.CurrentCommand)), "bash") ||
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(tty.CurrentCommand)), "zsh") {
		return "command_typing"
	}
	return "terminal_program"
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
