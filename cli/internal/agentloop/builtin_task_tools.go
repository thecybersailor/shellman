package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

func TaskActionToolContractNames() []string {
	return []string{
		"task.current.set_flag",
		"write_stdin",
		"exec_command",
		"readfile",
		"task.input_prompt",
		"task.child.get_context",
		"task.child.get_tty_output",
		"task.child.spawn",
		"task.child.send_message",
		"task.parent.report",
	}
}

type WriteStdinTool struct {
	Exec func(ctx context.Context, taskID, input string, timeoutMs int) (string, error)
}

func (t *WriteStdinTool) Name() string { return "write_stdin" }

func (t *WriteStdinTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Send exact raw bytes to current task TTY stdin without appending newline.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input": map[string]any{"type": "string"},
				"timeout_ms": map[string]any{
					"type":    "integer",
					"minimum": 100,
					"maximum": 15000,
				},
			},
			"required": []string{"input"},
		},
	}
}

func (t *WriteStdinTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, error) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", errors.New("WRITE_STDIN_EXEC_UNAVAILABLE")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Input     string `json:"input"`
		TimeoutMs int    `json:"timeout_ms"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", err
	}
	if req.Input == "" {
		return "", errors.New("INVALID_INPUT")
	}
	if shouldRejectShellCommandWithoutSubmit(ctx, req.Input) {
		return "", errors.New("SHELL_WRITE_STDIN_COMMAND_MISSING_SUBMIT: input looks like a complete shell command; append \"\\r\" to execute, or use exec_command")
	}
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 1800
	}
	if timeoutMs < 100 || timeoutMs > 15000 {
		return "", errors.New("INVALID_TIMEOUT_MS")
	}
	return t.Exec(ctx, taskID, req.Input, timeoutMs)
}

type ExecCommandTool struct {
	Exec func(ctx context.Context, taskID, command string, maxOutputTokens int) (string, error)
}

func (t *ExecCommandTool) Name() string { return "exec_command" }

func (t *ExecCommandTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Execute one shell command in current task and return sampled output tail.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{"type": "string"},
				"max_output_tokens": map[string]any{
					"type":    "integer",
					"minimum": 128,
					"maximum": 8000,
				},
			},
			"required": []string{"command"},
		},
	}
}

func (t *ExecCommandTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, error) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", errors.New("EXEC_COMMAND_EXEC_UNAVAILABLE")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Command         string `json:"command"`
		MaxOutputTokens int    `json:"max_output_tokens"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", err
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return "", errors.New("INVALID_COMMAND")
	}
	maxOutputTokens := req.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = 1200
	}
	if maxOutputTokens < 128 || maxOutputTokens > 8000 {
		return "", errors.New("INVALID_MAX_OUTPUT_TOKENS")
	}
	return t.Exec(ctx, taskID, command, maxOutputTokens)
}

type ReadFileTool struct {
	Exec func(ctx context.Context, taskID, path string, maxChars int) (string, error)
}

func (t *ReadFileTool) Name() string { return "readfile" }

func (t *ReadFileTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Read file content under current task repo root with optional max_chars clipping.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string"},
				"max_chars": map[string]any{
					"type":    "integer",
					"minimum": 128,
					"maximum": 200000,
				},
			},
			"required": []string{"path"},
		},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, error) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", errors.New("READFILE_EXEC_UNAVAILABLE")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Path     string `json:"path"`
		MaxChars int    `json:"max_chars"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", err
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return "", errors.New("INVALID_PATH")
	}
	maxChars := req.MaxChars
	if maxChars <= 0 {
		maxChars = 24000
	}
	if maxChars < 128 || maxChars > 200000 {
		return "", errors.New("INVALID_MAX_CHARS")
	}
	return t.Exec(ctx, taskID, path, maxChars)
}

type TaskInputPromptTool struct {
	Exec func(ctx context.Context, taskID, prompt string) (string, error)
}

func (t *TaskInputPromptTool) Name() string { return "task.input_prompt" }

func (t *TaskInputPromptTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Send one prompt message to current in-pane AI agent and submit with Enter.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt": map[string]any{"type": "string"},
			},
			"required": []string{"prompt"},
		},
	}
}

func (t *TaskInputPromptTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, error) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", errors.New("TASK_INPUT_PROMPT_EXEC_UNAVAILABLE")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Prompt string `json:"prompt"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", err
	}
	prompt := strings.Trim(req.Prompt, " \t")
	if strings.TrimSpace(prompt) == "" {
		return "", errors.New("INVALID_PROMPT")
	}
	if !strings.HasSuffix(prompt, "\r") && !strings.HasSuffix(prompt, "\n") {
		prompt += "\r"
	}
	return t.Exec(ctx, taskID, prompt)
}

type TaskChildGetContextTool struct {
	Exec func(ctx context.Context, taskID, childTaskID string) (string, error)
}

func (t *TaskChildGetContextTool) Name() string { return "task.child.get_context" }

func (t *TaskChildGetContextTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Get context of a child task under current task.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{"type": "string"},
			},
			"required": []string{"task_id"},
		},
	}
}

func (t *TaskChildGetContextTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, error) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", errors.New("TASK_CHILD_GET_CONTEXT_EXEC_UNAVAILABLE")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		TaskID string `json:"task_id"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", err
	}
	childTaskID := strings.TrimSpace(req.TaskID)
	if childTaskID == "" {
		return "", errors.New("INVALID_TASK_ID")
	}
	return t.Exec(ctx, taskID, childTaskID)
}

type TaskChildGetTTYOutputTool struct {
	Exec func(ctx context.Context, taskID, childTaskID string, offset int) (string, error)
}

func (t *TaskChildGetTTYOutputTool) Name() string { return "task.child.get_tty_output" }

func (t *TaskChildGetTTYOutputTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Get runtime tty output of a child task with optional offset.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{"type": "string"},
				"offset":  map[string]any{"type": "integer", "minimum": 0},
			},
			"required": []string{"task_id", "offset"},
		},
	}
}

func (t *TaskChildGetTTYOutputTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, error) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", errors.New("TASK_CHILD_GET_TTY_OUTPUT_EXEC_UNAVAILABLE")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		TaskID string `json:"task_id"`
		Offset int    `json:"offset"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", err
	}
	childTaskID := strings.TrimSpace(req.TaskID)
	if childTaskID == "" {
		return "", errors.New("INVALID_TASK_ID")
	}
	if req.Offset < 0 {
		return "", errors.New("INVALID_OFFSET")
	}
	return t.Exec(ctx, taskID, childTaskID, req.Offset)
}

type TaskChildSpawnTool struct {
	Exec func(ctx context.Context, taskID, command, title, description, prompt string) (string, error)
}

func (t *TaskChildSpawnTool) Name() string { return "task.child.spawn" }

func (t *TaskChildSpawnTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Spawn a child task under current task with command, title, description and prompt.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":     map[string]any{"type": "string"},
				"title":       map[string]any{"type": "string"},
				"description": map[string]any{"type": "string"},
				"prompt":      map[string]any{"type": "string"},
			},
			"required": []string{"command", "title", "description", "prompt"},
		},
	}
}

func (t *TaskChildSpawnTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, error) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", errors.New("TASK_CHILD_SPAWN_EXEC_UNAVAILABLE")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Command     string `json:"command"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", err
	}
	if strings.TrimSpace(req.Command) == "" ||
		strings.TrimSpace(req.Title) == "" ||
		strings.TrimSpace(req.Description) == "" ||
		strings.TrimSpace(req.Prompt) == "" {
		return "", errors.New("INVALID_SPAWN_INPUT")
	}
	return t.Exec(ctx, taskID, req.Command, req.Title, req.Description, req.Prompt)
}

type TaskChildSendMessageTool struct {
	Exec func(ctx context.Context, taskID, childTaskID, message string) (string, error)
}

func (t *TaskChildSendMessageTool) Name() string { return "task.child.send_message" }

func (t *TaskChildSendMessageTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Send message from parent task to one child task.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{"type": "string"},
				"message": map[string]any{"type": "string"},
			},
			"required": []string{"task_id", "message"},
		},
	}
}

func (t *TaskChildSendMessageTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, error) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", errors.New("TASK_CHILD_SEND_MESSAGE_EXEC_UNAVAILABLE")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		TaskID  string `json:"task_id"`
		Message string `json:"message"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", err
	}
	childTaskID := strings.TrimSpace(req.TaskID)
	if childTaskID == "" {
		return "", errors.New("INVALID_TASK_ID")
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return "", errors.New("INVALID_MESSAGE")
	}
	return t.Exec(ctx, taskID, childTaskID, message)
}

type TaskParentReportTool struct {
	Exec func(ctx context.Context, taskID, summary string) (string, error)
}

func (t *TaskParentReportTool) Name() string { return "task.parent.report" }

func (t *TaskParentReportTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Report summary from current task to parent task.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"summary": map[string]any{"type": "string"},
			},
			"required": []string{"summary"},
		},
	}
}

func (t *TaskParentReportTool) Execute(ctx context.Context, input json.RawMessage, callID string) (string, error) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", errors.New("TASK_PARENT_REPORT_EXEC_UNAVAILABLE")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Summary string `json:"summary"`
	}{}
	if err := json.Unmarshal(input, &req); err != nil {
		return "", err
	}
	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		return "", errors.New("INVALID_SUMMARY")
	}
	return t.Exec(ctx, taskID, summary)
}

func currentTaskIDFromContext(ctx context.Context) (string, error) {
	scope, ok := TaskScopeFromContext(ctx)
	if !ok || strings.TrimSpace(scope.TaskID) == "" {
		return "", errors.New("TASK_CONTEXT_MISSING")
	}
	return strings.TrimSpace(scope.TaskID), nil
}

func shouldRejectShellCommandWithoutSubmit(ctx context.Context, input string) bool {
	if !isLikelyShellToolMode(ctx) {
		return false
	}
	return looksLikeCompleteShellCommandWithoutSubmit(input)
}

func isLikelyShellToolMode(ctx context.Context) bool {
	names, ok := AllowedToolNamesFromContext(ctx)
	if !ok {
		return false
	}
	hasExecCommand := false
	hasTaskInputPrompt := false
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		switch name {
		case "exec_command":
			hasExecCommand = true
		case "task.input_prompt":
			hasTaskInputPrompt = true
		}
	}
	return hasExecCommand && !hasTaskInputPrompt
}

func looksLikeCompleteShellCommandWithoutSubmit(input string) bool {
	if strings.Contains(input, "\r") || strings.Contains(input, "\n") {
		return false
	}
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false
	}
	if strings.ContainsRune(trimmed, '\x1b') || strings.ContainsRune(trimmed, '\t') {
		return false
	}
	if strings.HasSuffix(trimmed, "\\") || endsWithShellOperator(trimmed) {
		return false
	}
	if !shellTextStructureLooksComplete(trimmed) {
		return false
	}
	commandToken := extractShellCommandToken(trimmed)
	if commandToken == "" {
		return false
	}
	// "/" is almost always user typing state/path, not an executable command intent.
	if commandToken == "/" {
		return false
	}
	return true
}

func endsWithShellOperator(text string) bool {
	if text == "" {
		return false
	}
	for _, suffix := range []string{"&&", "||", "|", ";", "&", "("} {
		if strings.HasSuffix(text, suffix) {
			return true
		}
	}
	return false
}

func shellTextStructureLooksComplete(text string) bool {
	var (
		inSingle bool
		inDouble bool
		escaped  bool
	)
	parens := 0
	brackets := 0
	braces := 0
	for _, ch := range text {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && !inSingle {
			escaped = true
			continue
		}
		if inSingle {
			if ch == '\'' {
				inSingle = false
			}
			continue
		}
		if inDouble {
			if ch == '"' {
				inDouble = false
			}
			continue
		}
		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '(':
			parens++
		case ')':
			parens--
		case '[':
			brackets++
		case ']':
			brackets--
		case '{':
			braces++
		case '}':
			braces--
		}
		if parens < 0 || brackets < 0 || braces < 0 {
			return false
		}
	}
	return !inSingle && !inDouble && !escaped && parens == 0 && brackets == 0 && braces == 0
}

var envAssignPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=.*$`)

func extractShellCommandToken(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	i := 0
	for i < len(fields) && envAssignPattern.MatchString(fields[i]) {
		i++
	}
	for i < len(fields) {
		token := strings.TrimSpace(fields[i])
		switch token {
		case "sudo", "command", "builtin", "time", "nohup":
			i++
			for i < len(fields) && strings.HasPrefix(strings.TrimSpace(fields[i]), "-") {
				i++
			}
		case "env":
			i++
			for i < len(fields) {
				part := strings.TrimSpace(fields[i])
				if strings.HasPrefix(part, "-") || envAssignPattern.MatchString(part) {
					i++
					continue
				}
				break
			}
		default:
			if strings.HasPrefix(token, "#") {
				return ""
			}
			return token
		}
	}
	return ""
}
