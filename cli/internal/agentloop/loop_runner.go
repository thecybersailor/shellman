package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type ResponsesAPI interface {
	CreateResponse(ctx context.Context, req CreateResponseRequest) (*CreateResponseResult, error)
}

type ResponsesStreamAPI interface {
	CreateResponseStream(ctx context.Context, req CreateResponseRequest, onTextDelta func(string)) (*CreateResponseResult, error)
}

type LoopRunnerOptions struct {
	MaxIterations int
}

type LoopRunner struct {
	client  ResponsesAPI
	tools   *ToolRegistry
	options LoopRunnerOptions
}

func NewLoopRunner(client ResponsesAPI, tools *ToolRegistry, options LoopRunnerOptions) *LoopRunner {
	if options.MaxIterations <= 0 {
		options.MaxIterations = 8
	}
	return &LoopRunner{client: client, tools: tools, options: options}
}

func (r *LoopRunner) Run(ctx context.Context, userPrompt string) (string, error) {
	return r.run(ctx, userPrompt, nil, nil)
}

func (r *LoopRunner) RunStream(ctx context.Context, userPrompt string, onTextDelta func(string)) (string, error) {
	return r.run(ctx, userPrompt, onTextDelta, nil)
}

func (r *LoopRunner) RunStreamWithTools(
	ctx context.Context,
	userPrompt string,
	onTextDelta func(string),
	onToolEvent func(map[string]any),
) (string, error) {
	return r.run(ctx, userPrompt, onTextDelta, onToolEvent)
}

func (r *LoopRunner) run(
	ctx context.Context,
	userPrompt string,
	onTextDelta func(string),
	onToolEvent func(map[string]any),
) (string, error) {
	if r == nil || r.client == nil {
		return "", errors.New("loop runner client is required")
	}
	storeEnabled := resolveResponsesStoreFromContext(ctx)
	// Always use full-context roundtrip mode: re-send the entire conversation
	// history (user message + function_call items + function_call_output items)
	// in every request instead of relying on previous_response_id.
	//
	// The previous_response_id approach requires the provider to persist the
	// response server-side. When routed through a proxy (e.g. OpenRouter, Azure)
	// with store=false — or even store=true with a provider that doesn't honour
	// it — the referenced response is gone and the API returns:
	//   "No tool call found for function call output with call_id <id>"
	//
	// Full-context mode is slightly larger per request but works with every
	// provider and avoids the 400 error entirely.
	fullContextRoundtrip := true
	userPrompt = strings.TrimSpace(userPrompt)
	userPromptForUserMessage := userPrompt
	historyInputItems := make([]map[string]any, 0, 8)
	if systemContextText, ok := extractPromptSystemContextJSON(userPrompt); ok {
		historyInputItems = append(historyInputItems, buildSystemMessageInputItem(systemContextText))
		if stripped, ok := stripPromptSystemContextSection(userPrompt); ok {
			userPromptForUserMessage = stripped
		}
	}
	historyInputItems = append(historyInputItems, buildUserMessageInputItem(userPromptForUserMessage))
	req := CreateResponseRequest{Store: boolPtr(storeEnabled)}
	if fullContextRoundtrip {
		req.Input = cloneResponseInputItems(historyInputItems)
	} else {
		req.Input = userPrompt
	}
	lastResponseTrace := ""
	initialPromptCurrentCommand, hasInitialPromptCurrentCommand := extractPromptCurrentCommand(userPrompt)
	previousRoundMode := ""
	forceRoundModeHintNextIteration := false
	stalePromptCurrentCommandDetected := false
	stalePromptCurrentCommandReasons := map[string]struct{}{}
	promptCurrentCommandCleared := false

	for i := 0; i < r.options.MaxIterations; i++ {
		allowedTools, allowlistConfigured := allowedToolNameSetFromContext(ctx)
		if r.tools != nil {
			req.Tools = r.resolveToolSpecs(allowedTools, allowlistConfigured)
		}
		currentRoundMode := resolveRoundModeFromAllowedTools(allowedTools, allowlistConfigured)
		modeChanged := previousRoundMode != "" && currentRoundMode != "" && currentRoundMode != previousRoundMode
		if !promptCurrentCommandCleared && hasInitialPromptCurrentCommand && (modeChanged || stalePromptCurrentCommandDetected) {
			clearReasons := make([]string, 0, 4)
			if modeChanged {
				clearReasons = append(clearReasons, "mode_changed")
			}
			clearReasons = append(clearReasons, sortedReasonKeys(stalePromptCurrentCommandReasons)...)
			if updatedInput, changed := clearCurrentCommandFromResponseInput(req.Input); changed {
				req.Input = updatedInput
				if fullContextRoundtrip {
					if updatedItems, ok := updatedInput.([]map[string]any); ok {
						historyInputItems = cloneResponseInputItems(updatedItems)
					}
				}
				promptCurrentCommandCleared = true
				forceRoundModeHintNextIteration = true
				stalePromptCurrentCommandDetected = false
				stalePromptCurrentCommandReasons = map[string]struct{}{}
				if onToolEvent != nil {
					onToolEvent(map[string]any{
						"type":                  "agent-debug",
						"stage":                 "responses.prompt.current_command.cleared",
						"iteration":             i + 1,
						"clear_reasons":         clearReasons,
						"previous_round_mode":   previousRoundMode,
						"current_round_mode":    currentRoundMode,
						"initial_current_cmd":   initialPromptCurrentCommand,
						"history_items_updated": true,
					})
				}
			}
		}
		callReq := req
		injectRoundModeHint := forceRoundModeHintNextIteration || modeChanged
		callReq.Input = withRoundModeHintInputWhen(req.Input, allowedTools, allowlistConfigured, injectRoundModeHint)
		forceRoundModeHintNextIteration = false
		reqSummary := summarizeCreateResponseRequest(callReq)
		if onToolEvent != nil {
			onToolEvent(map[string]any{
				"type":              "agent-debug",
				"stage":             "responses.request",
				"iteration":         i + 1,
				"request":           reqSummary,
				"previous_response": lastResponseTrace,
				"roundtrip_mode":    roundtripModeName(fullContextRoundtrip),
			})
		}
		callCtx := ctx
		if onToolEvent != nil {
			iteration := i + 1
			callCtx = WithResponsesDebugHooks(ctx, ResponsesDebugHooks{
				OnRequestRaw: func(raw string) {
					onToolEvent(map[string]any{
						"type":        "agent-debug",
						"stage":       "responses.request.raw",
						"iteration":   iteration,
						"request_raw": raw,
					})
				},
				OnResponseRaw: func(raw string) {
					onToolEvent(map[string]any{
						"type":         "agent-debug",
						"stage":        "responses.response.raw",
						"iteration":    iteration,
						"response_raw": raw,
					})
				},
				OnStreamEventRaw: func(raw string) {
					onToolEvent(map[string]any{
						"type":      "agent-debug",
						"stage":     "responses.stream.event.raw",
						"iteration": iteration,
						"event_raw": raw,
					})
				},
			})
		}
		var (
			res *CreateResponseResult
			err error
		)
		if onTextDelta != nil {
			if streamClient, ok := r.client.(ResponsesStreamAPI); ok {
				res, err = streamClient.CreateResponseStream(callCtx, callReq, onTextDelta)
			} else {
				res, err = r.client.CreateResponse(callCtx, callReq)
				if err == nil && strings.TrimSpace(res.FinalText) != "" {
					onTextDelta(res.FinalText)
				}
			}
		} else {
			res, err = r.client.CreateResponse(callCtx, callReq)
		}
		if err != nil {
			base := fmt.Sprintf("responses request failed iteration=%d %s", i+1, reqSummary)
			if strings.TrimSpace(lastResponseTrace) != "" {
				base += " prev_response_trace=" + lastResponseTrace
			}
			return "", fmt.Errorf("%s: %w", base, err)
		}
		currentTrace := summarizeEventTrace(res.EventTrace)
		if onToolEvent != nil {
			onToolEvent(map[string]any{
				"type":               "agent-debug",
				"stage":              "responses.response",
				"iteration":          i + 1,
				"response_id":        strings.TrimSpace(res.ID),
				"tool_calls":         len(res.ToolCalls),
				"tool_calls_summary": summarizeToolCalls(res.ToolCalls),
				"final_text_len":     len(strings.TrimSpace(res.FinalText)),
				"event_trace":        currentTrace,
				"event_count":        len(res.EventTrace),
			})
		}
		lastResponseTrace = currentTrace
		if res.HasFinalText() {
			return res.FinalText, nil
		}
		if len(res.ToolCalls) == 0 {
			return "", fmt.Errorf(
				"responses api returned no output_text and no tool_calls iteration=%d response_id=%q %s response_trace=%s",
				i+1,
				strings.TrimSpace(res.ID),
				reqSummary,
				currentTrace,
			)
		}
		if !fullContextRoundtrip && strings.TrimSpace(res.ID) == "" {
			return "", fmt.Errorf(
				"responses stream missing response id for tool roundtrip iteration=%d tool_calls=%s %s response_trace=%s",
				i+1,
				summarizeToolCalls(res.ToolCalls),
				reqSummary,
				currentTrace,
			)
		}
		outputs := make([]map[string]any, 0, len(res.ToolCalls))
		fullContextRoundtripItems := make([]map[string]any, 0, len(res.ToolCalls)*2)
		for _, call := range res.ToolCalls {
			callID := strings.TrimSpace(call.CallID)
			callResponseID := strings.TrimSpace(call.ResponseID)
			if callID == "" {
				return "", fmt.Errorf(
					"responses tool call missing call_id iteration=%d tool=%s id=%s response_id=%q %s",
					i+1,
					strings.TrimSpace(call.Name),
					strings.TrimSpace(call.ID),
					strings.TrimSpace(res.ID),
					reqSummary,
				)
			}
			if !fullContextRoundtrip && callResponseID != "" && callResponseID != strings.TrimSpace(res.ID) {
				return "", fmt.Errorf(
					"responses tool call response_id mismatch iteration=%d response_id=%q tool_response_id=%q call_id=%s tool=%s %s",
					i+1,
					strings.TrimSpace(res.ID),
					callResponseID,
					callID,
					strings.TrimSpace(call.Name),
					reqSummary,
				)
			}
			if onToolEvent != nil {
				onToolEvent(map[string]any{
					"type":          "dynamic-tool",
					"call_id":       callID,
					"response_id":   strings.TrimSpace(res.ID),
					"tool_name":     strings.TrimSpace(call.Name),
					"state":         "input-available",
					"input":         rawJSONToAny(call.Arguments),
					"input_raw_len": len(strings.TrimSpace(string(call.Arguments))),
					"input_preview": clipForLog(strings.TrimSpace(string(call.Arguments)), 800),
				})
			}
			var out string
			if r.tools == nil {
				toolErr := NewToolError("TOOL_REGISTRY_UNAVAILABLE", "确认工具注册表已初始化并注入到 LoopRunner")
				out = mustMarshalToolError(toolErr)
			} else {
				if allowlistConfigured {
					if _, ok := allowedTools[strings.TrimSpace(call.Name)]; !ok {
						toolErr := NewToolError(
							"TOOL_NOT_ENABLED_IN_MODE",
							fmt.Sprintf("工具 %q 不在当前 allowed_tools 中；切换模式或调整 allowlist 后重试", strings.TrimSpace(call.Name)),
						)
						errOut := mustMarshalToolError(toolErr)
						if onToolEvent != nil {
							onToolEvent(map[string]any{
								"type":        "dynamic-tool",
								"call_id":     callID,
								"response_id": strings.TrimSpace(res.ID),
								"tool_name":   strings.TrimSpace(call.Name),
								"state":       "output-error",
								"error_text":  toolErr.Message,
								"output_len":  len(strings.TrimSpace(errOut)),
								"output":      stringToMaybeJSONAny(errOut),
							})
						}
						out = errOut
						forceRoundModeHintNextIteration = true
					} else {
						toolOut, err := r.tools.Execute(ctx, call.Name, call.Arguments, call.CallID)
						if err != nil {
							errOut := mustMarshalToolError(err)
							if onToolEvent != nil {
								onToolEvent(map[string]any{
									"type":        "dynamic-tool",
									"call_id":     callID,
									"response_id": strings.TrimSpace(res.ID),
									"tool_name":   strings.TrimSpace(call.Name),
									"state":       "output-error",
									"error_text":  err.Message,
									"output_len":  len(strings.TrimSpace(errOut)),
									"output":      stringToMaybeJSONAny(errOut),
								})
							}
							out = errOut
						} else {
							out = toolOut
							if onToolEvent != nil {
								onToolEvent(map[string]any{
									"type":        "dynamic-tool",
									"call_id":     callID,
									"response_id": strings.TrimSpace(res.ID),
									"tool_name":   strings.TrimSpace(call.Name),
									"state":       "output-available",
									"output":      stringToMaybeJSONAny(toolOut),
									"output_len":  len(strings.TrimSpace(toolOut)),
								})
							}
						}
					}
				} else {
					toolOut, err := r.tools.Execute(ctx, call.Name, call.Arguments, call.CallID)
					if err != nil {
						errOut := mustMarshalToolError(err)
						if onToolEvent != nil {
							onToolEvent(map[string]any{
								"type":        "dynamic-tool",
								"call_id":     callID,
								"response_id": strings.TrimSpace(res.ID),
								"tool_name":   strings.TrimSpace(call.Name),
								"state":       "output-error",
								"error_text":  err.Message,
								"output_len":  len(strings.TrimSpace(errOut)),
								"output":      stringToMaybeJSONAny(errOut),
							})
						}
						out = errOut
					} else {
						out = toolOut
						if onToolEvent != nil {
							onToolEvent(map[string]any{
								"type":        "dynamic-tool",
								"call_id":     callID,
								"response_id": strings.TrimSpace(res.ID),
								"tool_name":   strings.TrimSpace(call.Name),
								"state":       "output-available",
								"output":      stringToMaybeJSONAny(toolOut),
								"output_len":  len(strings.TrimSpace(toolOut)),
							})
						}
					}
				}
			}
			if outputCommand, hashChanged, ok := detectPostTerminalStateFromToolOutput(out); ok {
				if hashChanged {
					stalePromptCurrentCommandDetected = true
					stalePromptCurrentCommandReasons["hash_changed"] = struct{}{}
				}
				if hasInitialPromptCurrentCommand && strings.TrimSpace(outputCommand) != "" &&
					!strings.EqualFold(strings.TrimSpace(outputCommand), strings.TrimSpace(initialPromptCurrentCommand)) {
					stalePromptCurrentCommandDetected = true
					stalePromptCurrentCommandReasons["current_command_changed"] = struct{}{}
				}
				if stalePromptCurrentCommandDetected {
					forceRoundModeHintNextIteration = true
				}
			}
			if onToolEvent != nil {
				onToolEvent(map[string]any{
					"type":            "agent-debug",
					"stage":           "responses.roundtrip.item",
					"iteration":       i + 1,
					"response_id":     strings.TrimSpace(res.ID),
					"call_id":         callID,
					"tool_name":       strings.TrimSpace(call.Name),
					"function_output": clipForLog(strings.TrimSpace(out), 800),
					"output_len":      len(strings.TrimSpace(out)),
				})
			}
			replayCall := buildReplayFunctionCallInputItem(call)
			outputItem := map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  out,
			}
			outputs = append(outputs, outputItem)
			fullContextRoundtripItems = append(fullContextRoundtripItems, replayCall, outputItem)
		}
		if onToolEvent != nil {
			onToolEvent(map[string]any{
				"type":                 "agent-debug",
				"stage":                "responses.roundtrip.prepare",
				"iteration":            i + 1,
				"previous_response_id": strings.TrimSpace(res.ID),
				"roundtrip_mode":       roundtripModeName(fullContextRoundtrip),
				"items_count":          len(outputs),
				"items_summary":        summarizeResponseInput(outputs),
			})
		}
		nextReq := CreateResponseRequest{
			Store: boolPtr(storeEnabled),
		}
		if fullContextRoundtrip {
			historyInputItems = append(historyInputItems, fullContextRoundtripItems...)
			nextReq.Input = cloneResponseInputItems(historyInputItems)
		} else {
			nextReq.Input = outputs
			nextReq.PreviousResponseID = res.ID
		}
		req = nextReq
		previousRoundMode = currentRoundMode
	}
	return "", fmt.Errorf("responses loop exceeded max iterations: %d", r.options.MaxIterations)
}

func withRoundModeHintInputWhen(
	input any,
	allowedTools map[string]struct{},
	allowlistConfigured bool,
	enable bool,
) any {
	if !enable {
		return input
	}
	items, ok := input.([]map[string]any)
	if !ok {
		return input
	}
	hint := buildRoundModeHintInputItem(allowedTools, allowlistConfigured)
	if hint == nil {
		return input
	}
	out := make([]map[string]any, 0, len(items)+1)
	out = append(out, hint)
	out = append(out, items...)
	return out
}

func resolveRoundModeFromAllowedTools(allowedTools map[string]struct{}, allowlistConfigured bool) string {
	if !allowlistConfigured {
		return "unconstrained"
	}
	_, hasExecCommand := allowedTools["exec_command"]
	_, hasTaskInputPrompt := allowedTools["task.input_prompt"]
	switch {
	case hasTaskInputPrompt && !hasExecCommand:
		return "ai_agent"
	case hasExecCommand && !hasTaskInputPrompt:
		return "shell"
	case hasExecCommand && hasTaskInputPrompt:
		return "mixed"
	default:
		return "default"
	}
}

func clearCurrentCommandFromResponseInput(input any) (any, bool) {
	items, ok := input.([]map[string]any)
	if !ok {
		return input, false
	}
	updated, changed := clearCurrentCommandInInputItems(items)
	if !changed {
		return input, false
	}
	return updated, true
}

func clearCurrentCommandInInputItems(items []map[string]any) ([]map[string]any, bool) {
	if len(items) == 0 {
		return items, false
	}
	out := make([]map[string]any, len(items))
	copy(out, items)
	changed := false
	for i, item := range items {
		if strings.TrimSpace(fmt.Sprint(item["type"])) != "message" || strings.TrimSpace(fmt.Sprint(item["role"])) != "user" {
			continue
		}
		rawContent, ok := item["content"]
		if !ok {
			continue
		}
		parts, ok := normalizeMessageContentParts(rawContent)
		if !ok || len(parts) == 0 {
			continue
		}
		nextParts := make([]map[string]any, len(parts))
		copy(nextParts, parts)
		partChanged := false
		for idx, part := range parts {
			if strings.TrimSpace(fmt.Sprint(part["type"])) != "input_text" {
				continue
			}
			rawText, _ := part["text"].(string)
			nextText, textChanged := clearCurrentCommandInPromptText(rawText)
			if !textChanged {
				continue
			}
			updatedPart := cloneMap(part)
			updatedPart["text"] = nextText
			nextParts[idx] = updatedPart
			partChanged = true
		}
		if !partChanged {
			continue
		}
		updatedItem := cloneMap(item)
		updatedItem["content"] = nextParts
		out[i] = updatedItem
		changed = true
	}
	if !changed {
		return items, false
	}
	return out, true
}

func normalizeMessageContentParts(raw any) ([]map[string]any, bool) {
	switch v := raw.(type) {
	case []map[string]any:
		out := make([]map[string]any, len(v))
		copy(out, v)
		return out, true
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			part, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			out = append(out, part)
		}
		return out, true
	default:
		return nil, false
	}
}

func extractPromptCurrentCommand(prompt string) (string, bool) {
	decoded, _, _, ok := extractPromptTaskContextJSON(prompt)
	if !ok {
		return "", false
	}
	screen, ok := decoded["terminal_screen_state"].(map[string]any)
	if !ok {
		return "", false
	}
	raw := strings.TrimSpace(fmt.Sprint(screen["current_command"]))
	if raw == "" || raw == "<nil>" {
		return "", false
	}
	return raw, true
}

func clearCurrentCommandInPromptText(prompt string) (string, bool) {
	decoded, start, end, ok := extractPromptTaskContextJSON(prompt)
	if !ok {
		return prompt, false
	}
	screen, ok := decoded["terminal_screen_state"].(map[string]any)
	if !ok {
		return prompt, false
	}
	if _, exists := screen["current_command"]; !exists {
		return prompt, false
	}
	screen["current_command"] = ""
	nextRaw, err := json.Marshal(decoded)
	if err != nil {
		return prompt, false
	}
	return prompt[:start] + string(nextRaw) + prompt[end:], true
}

func extractPromptTaskContextJSON(prompt string) (map[string]any, int, int, bool) {
	start, end, ok := extractPromptJSONObjectRange(prompt, "terminal_screen_state_json:")
	if !ok {
		return nil, 0, 0, false
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(prompt[start:end]), &decoded); err != nil {
		return nil, 0, 0, false
	}
	return decoded, start, end, true
}

func extractPromptSystemContextJSON(prompt string) (string, bool) {
	start, end, _, _, ok := extractPromptSystemAndEventSectionRanges(prompt)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(prompt[start:end]), true
}

func stripPromptSystemContextSection(prompt string) (string, bool) {
	_, _, systemSectionStart, eventSectionStart, ok := extractPromptSystemAndEventSectionRanges(prompt)
	if !ok {
		return prompt, false
	}
	next := prompt[:systemSectionStart] + prompt[eventSectionStart:]
	return strings.TrimSpace(next), true
}

func extractPromptSystemAndEventSectionRanges(prompt string) (systemJSONStart, systemJSONEnd, systemSectionStart, eventSectionStart int, ok bool) {
	const (
		systemMarker       = "\n\nsystem_context_json:"
		eventMarker        = "\n\nevent_context_json:"
		conversationMarker = "\n\nconversation_history:"
	)
	convIdx := strings.Index(prompt, conversationMarker)
	if convIdx < 0 {
		return 0, 0, 0, 0, false
	}
	eventIdx := strings.LastIndex(prompt[:convIdx], eventMarker)
	if eventIdx < 0 {
		return 0, 0, 0, 0, false
	}
	systemIdx := strings.LastIndex(prompt[:eventIdx], systemMarker)
	if systemIdx < 0 {
		return 0, 0, 0, 0, false
	}
	start := systemIdx + len(systemMarker)
	for start < len(prompt) {
		ch := prompt[start]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			start++
			continue
		}
		break
	}
	if start >= len(prompt) || prompt[start] != '{' {
		return 0, 0, 0, 0, false
	}
	end, found := findJSONObjectEnd(prompt, start)
	if !found || end > eventIdx {
		return 0, 0, 0, 0, false
	}
	return start, end, systemIdx, eventIdx, true
}

func extractPromptJSONObjectRange(prompt string, marker string) (int, int, bool) {
	idx := strings.Index(prompt, marker)
	if idx < 0 {
		return 0, 0, false
	}
	start := idx + len(marker)
	for start < len(prompt) {
		ch := prompt[start]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			start++
			continue
		}
		break
	}
	if start >= len(prompt) || prompt[start] != '{' {
		return 0, 0, false
	}
	end, ok := findJSONObjectEnd(prompt, start)
	if !ok {
		return 0, 0, false
	}
	return start, end, true
}

func findJSONObjectEnd(input string, start int) (int, bool) {
	if start < 0 || start >= len(input) || input[start] != '{' {
		return 0, false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(input); i++ {
		ch := input[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
	}
	return 0, false
}

func detectPostTerminalStateFromToolOutput(output string) (string, bool, bool) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "", false, false
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return "", false, false
	}
	screen := nestedMap(decoded, "data", "post_terminal_screen_state")
	if screen == nil {
		screen = nestedMap(decoded, "post_terminal_screen_state")
	}
	if screen == nil {
		return "", false, false
	}
	currentCommand := strings.TrimSpace(fmt.Sprint(screen["current_command"]))
	if currentCommand == "<nil>" {
		currentCommand = ""
	}
	return currentCommand, boolFromAny(screen["hash_changed"]), true
}

func nestedMap(root map[string]any, keys ...string) map[string]any {
	cur := root
	for i, key := range keys {
		raw, ok := cur[key]
		if !ok {
			return nil
		}
		next, ok := raw.(map[string]any)
		if !ok {
			return nil
		}
		if i == len(keys)-1 {
			return next
		}
		cur = next
	}
	return nil
}

func boolFromAny(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(strings.TrimSpace(x), "true")
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
	}
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func sortedReasonKeys(reasons map[string]struct{}) []string {
	if len(reasons) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(reasons))
	for key := range reasons {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func buildRoundModeHintInputItem(allowedTools map[string]struct{}, allowlistConfigured bool) map[string]any {
	mode := "default"
	if allowlistConfigured {
		if _, ok := allowedTools["task.input_prompt"]; ok {
			mode = "ai_agent"
		} else if _, ok := allowedTools["exec_command"]; ok {
			mode = "shell"
		}
	}
	names := make([]string, 0, len(allowedTools))
	for name := range allowedTools {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	allowlistState := "resolver_disabled"
	if allowlistConfigured {
		allowlistState = "resolver_enabled"
	}
	text := strings.TrimSpace(strings.Join([]string{
		"ROUND_MODE_HINT",
		fmt.Sprintf("allowlist=%s", allowlistState),
		fmt.Sprintf("mode=%s", mode),
		fmt.Sprintf("allowed_tools=%s", strings.Join(names, ",")),
		"Authoritative mode for this round is derived from allowed_tools; ignore stale terminal snapshots from earlier messages.",
		"Never call tools that are not listed in allowed_tools for this round.",
	}, "\n"))
	return map[string]any{
		"type": "message",
		"role": "user",
		"content": []map[string]any{
			{
				"type": "input_text",
				"text": text,
			},
		},
	}
}

func (r *LoopRunner) resolveToolSpecs(allowedTools map[string]struct{}, allowlistConfigured bool) []ResponseToolSpec {
	if r == nil || r.tools == nil {
		return nil
	}
	if !allowlistConfigured {
		return r.tools.Specs()
	}
	if len(allowedTools) == 0 {
		return []ResponseToolSpec{}
	}
	names := make([]string, 0, len(allowedTools))
	for name := range allowedTools {
		names = append(names, name)
	}
	return r.tools.SpecsByNames(names)
}

func allowedToolNameSetFromContext(ctx context.Context) (map[string]struct{}, bool) {
	names, ok := AllowedToolNamesFromContext(ctx)
	if !ok {
		return nil, false
	}
	out := map[string]struct{}{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out, true
}

func rawJSONToAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw)
	}
	return out
}

func stringToMaybeJSONAny(text string) any {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	var out any
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return text
	}
	return out
}

func summarizeEventTrace(trace []string) string {
	if len(trace) == 0 {
		return ""
	}
	joined := strings.Join(trace, " | ")
	joined = strings.TrimSpace(joined)
	if len(joined) > 2000 {
		return joined[:2000] + "...(truncated)"
	}
	return joined
}

func clipForLog(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "...(truncated)"
}

func summarizeCreateResponseRequest(req CreateResponseRequest) string {
	storeSummary := "<unset>"
	if req.Store != nil {
		storeSummary = fmt.Sprintf("%t", *req.Store)
	}
	parts := []string{
		fmt.Sprintf("stream=%t", req.Stream),
		fmt.Sprintf("store=%s", storeSummary),
		fmt.Sprintf("previous_response_id=%q", strings.TrimSpace(req.PreviousResponseID)),
		fmt.Sprintf("tools=%d", len(req.Tools)),
		fmt.Sprintf("input=%s", summarizeResponseInput(req.Input)),
	}
	return strings.Join(parts, " ")
}

func resolveResponsesStoreFromContext(ctx context.Context) bool {
	scope, ok := TaskScopeFromContext(ctx)
	if !ok {
		return false
	}
	return scope.ResponsesStore
}

func boolPtr(v bool) *bool {
	return &v
}

func roundtripModeName(fullContext bool) string {
	if fullContext {
		return "full_context"
	}
	return "previous_response_id"
}

func cloneResponseInputItems(in []map[string]any) []map[string]any {
	if len(in) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(in))
	for _, item := range in {
		out = append(out, item)
	}
	return out
}

func buildUserMessageInputItem(text string) map[string]any {
	return map[string]any{
		"type": "message",
		"role": "user",
		"content": []map[string]any{
			{
				"type": "input_text",
				"text": strings.TrimSpace(text),
			},
		},
	}
}

func buildSystemMessageInputItem(text string) map[string]any {
	return map[string]any{
		"type": "message",
		"role": "system",
		"content": []map[string]any{
			{
				"type": "input_text",
				"text": strings.TrimSpace(text),
			},
		},
	}
}

func buildReplayFunctionCallInputItem(call ToolCall) map[string]any {
	callID := strings.TrimSpace(call.CallID)
	itemID := strings.TrimSpace(call.ID)
	if itemID == "" {
		itemID = callID
	}
	arguments := strings.TrimSpace(string(call.Arguments))
	if arguments == "" {
		arguments = "{}"
	}
	return map[string]any{
		"type":      "function_call",
		"id":        itemID,
		"call_id":   callID,
		"name":      sanitizeFunctionCallNameForInput(call.Name),
		"arguments": arguments,
	}
}

func sanitizeFunctionCallNameForInput(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "tool_call"
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			b.WriteRune(ch)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "tool_call"
	}
	return out
}

func summarizeResponseInput(input any) string {
	switch v := input.(type) {
	case string:
		return fmt.Sprintf("text(len=%d)", len(strings.TrimSpace(v)))
	case []map[string]any:
		items := make([]map[string]any, 0, len(v))
		items = append(items, v...)
		return summarizeResponseInputItems(items)
	case []any:
		items := make([]map[string]any, 0, len(v))
		for _, item := range v {
			asMap, ok := item.(map[string]any)
			if !ok {
				return fmt.Sprintf("items=%d(type_mismatch=%T)", len(v), item)
			}
			items = append(items, asMap)
		}
		return summarizeResponseInputItems(items)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("type=%T(marshal_error=%v)", input, err)
		}
		trimmed := strings.TrimSpace(string(raw))
		if len(trimmed) > 160 {
			trimmed = trimmed[:160] + "...(truncated)"
		}
		return fmt.Sprintf("type=%T json=%s", input, trimmed)
	}
}

func summarizeResponseInputItems(items []map[string]any) string {
	if len(items) == 0 {
		return "items=0"
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		itemType := strings.TrimSpace(fmt.Sprint(item["type"]))
		callID := strings.TrimSpace(fmt.Sprint(item["call_id"]))
		outputLen := len(strings.TrimSpace(fmt.Sprint(item["output"])))
		token := itemType
		if token == "" {
			token = "<empty_type>"
		}
		if callID != "" {
			token += fmt.Sprintf("(call_id=%s)", callID)
		}
		if itemType == "function_call_output" {
			token += fmt.Sprintf("(output_len=%d)", outputLen)
		}
		out = append(out, token)
	}
	return fmt.Sprintf("items=%d[%s]", len(items), strings.Join(out, ", "))
}

func summarizeToolCalls(calls []ToolCall) string {
	if len(calls) == 0 {
		return "<none>"
	}
	out := make([]string, 0, len(calls))
	for _, call := range calls {
		out = append(out, fmt.Sprintf(
			"%s(call_id=%s,id=%s,response_id=%s,args_len=%d)",
			strings.TrimSpace(call.Name),
			strings.TrimSpace(call.CallID),
			strings.TrimSpace(call.ID),
			strings.TrimSpace(call.ResponseID),
			len(strings.TrimSpace(string(call.Arguments))),
		))
	}
	return strings.Join(out, ", ")
}
