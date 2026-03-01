package agentloopadapter

import (
	"context"
	"encoding/json"
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
	Exec func(ctx context.Context, taskID, input string, timeoutMs int) (string, *ToolError)
}

func (t *WriteStdinTool) Name() string { return "write_stdin" }

func (t *WriteStdinTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Send exact raw bytes to current task TTY stdin without appending newline.",
		Parameters: ResponseToolParameters{
			Type: "object",
			Properties: []ResponseToolProperty{
				{Name: "input", Schema: ResponseToolSchema{Type: "string"}},
				{Name: "timeout_ms", Schema: ResponseToolSchema{Type: "integer", Minimum: IntPtr(100), Maximum: IntPtr(15000)}},
			},
			Required: []string{"input"},
		},
	}
}

func (t *WriteStdinTool) Execute(ctx context.Context, _ struct{}, input string, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("WRITE_STDIN_EXEC_UNAVAILABLE", "Ensure write_stdin exec callback is injected")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Input     string `json:"input"`
		TimeoutMs int    `json:"timeout_ms"`
	}{}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "Check write_stdin JSON: require input string, optional timeout_ms")
	}
	if req.Input == "" {
		return "", NewToolError("INVALID_INPUT", "Provide non-empty input string")
	}
	if shouldRejectShellCommandWithoutSubmit(ctx, req.Input) {
		return "", NewToolError("SHELL_WRITE_STDIN_COMMAND_MISSING_SUBMIT", "Input looks like a complete shell command; append \\r to submit, or use exec_command")
	}
	timeoutMs := req.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 1800
	}
	if timeoutMs < 100 || timeoutMs > 15000 {
		return "", NewToolError("INVALID_TIMEOUT_MS", "timeout_ms must be within 100..15000")
	}
	return t.Exec(ctx, taskID, req.Input, timeoutMs)
}

type ExecCommandTool struct {
	Exec func(ctx context.Context, taskID, command string, maxOutputTokens int) (string, *ToolError)
}

func (t *ExecCommandTool) Name() string { return "exec_command" }

func (t *ExecCommandTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Execute one shell command in current task and return sampled output tail.",
		Parameters: ResponseToolParameters{
			Type: "object",
			Properties: []ResponseToolProperty{
				{Name: "command", Schema: ResponseToolSchema{Type: "string"}},
				{Name: "max_output_tokens", Schema: ResponseToolSchema{Type: "integer", Minimum: IntPtr(128), Maximum: IntPtr(8000)}},
			},
			Required: []string{"command"},
		},
	}
}

func (t *ExecCommandTool) Execute(ctx context.Context, _ struct{}, input string, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("EXEC_COMMAND_EXEC_UNAVAILABLE", "Ensure exec_command exec callback is injected")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Command         string `json:"command"`
		MaxOutputTokens int    `json:"max_output_tokens"`
	}{}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "Check exec_command JSON: require command, optional max_output_tokens")
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return "", NewToolError("INVALID_COMMAND", "Provide non-empty command")
	}
	maxOutputTokens := req.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = 1200
	}
	if maxOutputTokens < 128 || maxOutputTokens > 8000 {
		return "", NewToolError("INVALID_MAX_OUTPUT_TOKENS", "max_output_tokens must be within 128..8000")
	}
	return t.Exec(ctx, taskID, command, maxOutputTokens)
}

type ReadFileTool struct {
	Exec func(ctx context.Context, taskID, path string, maxChars int) (string, *ToolError)
}

func (t *ReadFileTool) Name() string { return "readfile" }

func (t *ReadFileTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Read file content under current task repo root with optional max_chars clipping.",
		Parameters: ResponseToolParameters{
			Type: "object",
			Properties: []ResponseToolProperty{
				{Name: "path", Schema: ResponseToolSchema{Type: "string"}},
				{Name: "max_chars", Schema: ResponseToolSchema{Type: "integer", Minimum: IntPtr(128), Maximum: IntPtr(200000)}},
			},
			Required: []string{"path"},
		},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, _ struct{}, input string, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("READFILE_EXEC_UNAVAILABLE", "Ensure readfile exec callback is injected")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Path     string `json:"path"`
		MaxChars int    `json:"max_chars"`
	}{}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "Check readfile JSON: require path, optional max_chars")
	}
	path := strings.TrimSpace(req.Path)
	if path == "" {
		return "", NewToolError("INVALID_PATH", "Provide non-empty path")
	}
	maxChars := req.MaxChars
	if maxChars <= 0 {
		maxChars = 24000
	}
	if maxChars < 128 || maxChars > 200000 {
		return "", NewToolError("INVALID_MAX_CHARS", "max_chars must be within 128..200000")
	}
	return t.Exec(ctx, taskID, path, maxChars)
}

type TaskInputPromptTool struct {
	Exec func(ctx context.Context, taskID, prompt string) (string, *ToolError)
}

func (t *TaskInputPromptTool) Name() string { return "task.input_prompt" }

func (t *TaskInputPromptTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Send one prompt message to current in-pane AI agent and submit with Enter.",
		Parameters: ResponseToolParameters{
			Type: "object",
			Properties: []ResponseToolProperty{
				{Name: "prompt", Schema: ResponseToolSchema{Type: "string"}},
			},
			Required: []string{"prompt"},
		},
	}
}

func (t *TaskInputPromptTool) Execute(ctx context.Context, _ struct{}, input string, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("TASK_INPUT_PROMPT_EXEC_UNAVAILABLE", "Ensure task.input_prompt exec callback is injected")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Prompt string `json:"prompt"`
	}{}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "Check task.input_prompt JSON: require prompt")
	}
	prompt := strings.Trim(req.Prompt, " \t")
	if strings.TrimSpace(prompt) == "" {
		return "", NewToolError("INVALID_PROMPT", "Provide non-empty prompt")
	}
	if !strings.HasSuffix(prompt, "\r") && !strings.HasSuffix(prompt, "\n") {
		prompt += "\r"
	}
	return t.Exec(ctx, taskID, prompt)
}

type TaskChildGetContextTool struct {
	Exec func(ctx context.Context, taskID, childTaskID string) (string, *ToolError)
}

func (t *TaskChildGetContextTool) Name() string { return "task.child.get_context" }

func (t *TaskChildGetContextTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Get context of a child task under current task.",
		Parameters: ResponseToolParameters{
			Type: "object",
			Properties: []ResponseToolProperty{
				{Name: "task_id", Schema: ResponseToolSchema{Type: "string"}},
			},
			Required: []string{"task_id"},
		},
	}
}

func (t *TaskChildGetContextTool) Execute(ctx context.Context, _ struct{}, input string, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("TASK_CHILD_GET_CONTEXT_EXEC_UNAVAILABLE", "Ensure task.child.get_context exec callback is injected")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		TaskID string `json:"task_id"`
	}{}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "Check task.child.get_context JSON: require task_id")
	}
	childTaskID := strings.TrimSpace(req.TaskID)
	if childTaskID == "" {
		return "", NewToolError("INVALID_TASK_ID", "Provide non-empty task_id")
	}
	return t.Exec(ctx, taskID, childTaskID)
}

type TaskChildGetTTYOutputTool struct {
	Exec func(ctx context.Context, taskID, childTaskID string, offset int) (string, *ToolError)
}

func (t *TaskChildGetTTYOutputTool) Name() string { return "task.child.get_tty_output" }

func (t *TaskChildGetTTYOutputTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Get runtime tty output of a child task with optional offset.",
		Parameters: ResponseToolParameters{
			Type: "object",
			Properties: []ResponseToolProperty{
				{Name: "task_id", Schema: ResponseToolSchema{Type: "string"}},
				{Name: "offset", Schema: ResponseToolSchema{Type: "integer", Minimum: IntPtr(0)}},
			},
			Required: []string{"task_id", "offset"},
		},
	}
}

func (t *TaskChildGetTTYOutputTool) Execute(ctx context.Context, _ struct{}, input string, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("TASK_CHILD_GET_TTY_OUTPUT_EXEC_UNAVAILABLE", "Ensure task.child.get_tty_output exec callback is injected")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		TaskID string `json:"task_id"`
		Offset int    `json:"offset"`
	}{}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "Check task.child.get_tty_output JSON: require task_id and offset")
	}
	childTaskID := strings.TrimSpace(req.TaskID)
	if childTaskID == "" {
		return "", NewToolError("INVALID_TASK_ID", "Provide non-empty task_id")
	}
	if req.Offset < 0 {
		return "", NewToolError("INVALID_OFFSET", "offset must be >= 0")
	}
	return t.Exec(ctx, taskID, childTaskID, req.Offset)
}

type TaskChildSpawnTool struct {
	Exec func(ctx context.Context, taskID, command, title, description, prompt, taskRole string) (string, *ToolError)
}

func (t *TaskChildSpawnTool) Name() string { return "task.child.spawn" }

func (t *TaskChildSpawnTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Spawn a child task under current task with command, title, description and prompt.",
		Parameters: ResponseToolParameters{
			Type: "object",
			Properties: []ResponseToolProperty{
				{Name: "command", Schema: ResponseToolSchema{Type: "string"}},
				{Name: "title", Schema: ResponseToolSchema{Type: "string"}},
				{Name: "description", Schema: ResponseToolSchema{Type: "string"}},
				{Name: "prompt", Schema: ResponseToolSchema{Type: "string"}},
				{Name: "task_role", Schema: ResponseToolSchema{Type: "string", Enum: []string{"planner", "executor"}}},
			},
			Required: []string{"command", "title", "description", "prompt", "task_role"},
		},
	}
}

func (t *TaskChildSpawnTool) Execute(ctx context.Context, _ struct{}, input string, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("TASK_CHILD_SPAWN_EXEC_UNAVAILABLE", "Ensure task.child.spawn exec callback is injected")
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
		TaskRole    string `json:"task_role"`
	}{}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "Check task.child.spawn JSON: require command/title/description/prompt/task_role")
	}
	if strings.TrimSpace(req.Command) == "" ||
		strings.TrimSpace(req.Title) == "" ||
		strings.TrimSpace(req.Description) == "" ||
		strings.TrimSpace(req.Prompt) == "" ||
		strings.TrimSpace(req.TaskRole) == "" {
		return "", NewToolError("INVALID_SPAWN_INPUT", "command, title, description, prompt, task_role must all be non-empty")
	}
	taskRole := strings.ToLower(strings.TrimSpace(req.TaskRole))
	if taskRole != "planner" && taskRole != "executor" {
		return "", NewToolError("INVALID_SPAWN_INPUT", "task_role must be planner or executor")
	}
	return t.Exec(ctx, taskID, req.Command, req.Title, req.Description, req.Prompt, taskRole)
}

type TaskChildSendMessageTool struct {
	Exec func(ctx context.Context, taskID, childTaskID, message string) (string, *ToolError)
}

func (t *TaskChildSendMessageTool) Name() string { return "task.child.send_message" }

func (t *TaskChildSendMessageTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Send message from parent task to one child task.",
		Parameters: ResponseToolParameters{
			Type: "object",
			Properties: []ResponseToolProperty{
				{Name: "task_id", Schema: ResponseToolSchema{Type: "string"}},
				{Name: "message", Schema: ResponseToolSchema{Type: "string"}},
			},
			Required: []string{"task_id", "message"},
		},
	}
}

func (t *TaskChildSendMessageTool) Execute(ctx context.Context, _ struct{}, input string, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("TASK_CHILD_SEND_MESSAGE_EXEC_UNAVAILABLE", "Ensure task.child.send_message exec callback is injected")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		TaskID  string `json:"task_id"`
		Message string `json:"message"`
	}{}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "Check task.child.send_message JSON: require task_id and message")
	}
	childTaskID := strings.TrimSpace(req.TaskID)
	if childTaskID == "" {
		return "", NewToolError("INVALID_TASK_ID", "Provide non-empty task_id")
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return "", NewToolError("INVALID_MESSAGE", "Provide non-empty message")
	}
	return t.Exec(ctx, taskID, childTaskID, message)
}

type TaskParentReportTool struct {
	Exec func(ctx context.Context, taskID, summary string) (string, *ToolError)
}

func (t *TaskParentReportTool) Name() string { return "task.parent.report" }

func (t *TaskParentReportTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{
		Type:        "function",
		Name:        t.Name(),
		Description: "Report summary from current task to parent task.",
		Parameters: ResponseToolParameters{
			Type: "object",
			Properties: []ResponseToolProperty{
				{Name: "summary", Schema: ResponseToolSchema{Type: "string"}},
			},
			Required: []string{"summary"},
		},
	}
}

func (t *TaskParentReportTool) Execute(ctx context.Context, _ struct{}, input string, callID string) (string, *ToolError) {
	_ = callID
	if t == nil || t.Exec == nil {
		return "", NewToolError("TASK_PARENT_REPORT_EXEC_UNAVAILABLE", "Ensure task.parent.report exec callback is injected")
	}
	taskID, err := currentTaskIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	req := struct {
		Summary string `json:"summary"`
	}{}
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		return "", NewToolError("INVALID_JSON_INPUT", "Check task.parent.report JSON: require summary")
	}
	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		return "", NewToolError("INVALID_SUMMARY", "Provide non-empty summary")
	}
	return t.Exec(ctx, taskID, summary)
}

func currentTaskIDFromContext(ctx context.Context) (string, *ToolError) {
	scope, ok := TaskScopeFromContext(ctx)
	if !ok || strings.TrimSpace(scope.TaskID) == "" {
		return "", NewToolError("TASK_CONTEXT_MISSING", "Ensure current invocation is bound to task scope")
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
