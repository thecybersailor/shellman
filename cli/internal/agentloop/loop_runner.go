package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	historyInputItems := make([]map[string]any, 0, 8)
	historyInputItems = append(historyInputItems, buildUserMessageInputItem(userPrompt))
	req := CreateResponseRequest{Store: boolPtr(storeEnabled)}
	if fullContextRoundtrip {
		req.Input = cloneResponseInputItems(historyInputItems)
	} else {
		req.Input = userPrompt
	}
	lastResponseTrace := ""

	for i := 0; i < r.options.MaxIterations; i++ {
		allowedTools, allowlistConfigured := allowedToolNameSetFromContext(ctx)
		if r.tools != nil {
			req.Tools = r.resolveToolSpecs(allowedTools, allowlistConfigured)
		}
		reqSummary := summarizeCreateResponseRequest(req)
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
				res, err = streamClient.CreateResponseStream(callCtx, req, onTextDelta)
			} else {
				res, err = r.client.CreateResponse(callCtx, req)
				if err == nil && strings.TrimSpace(res.FinalText) != "" {
					onTextDelta(res.FinalText)
				}
			}
		} else {
			res, err = r.client.CreateResponse(callCtx, req)
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
		replayCalls := make([]map[string]any, 0, len(res.ToolCalls))
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
				out = `{"error":"tool registry unavailable"}`
			} else {
				if allowlistConfigured {
					if _, ok := allowedTools[strings.TrimSpace(call.Name)]; !ok {
						toolErr := fmt.Errorf("tool %q is not enabled in current mode", strings.TrimSpace(call.Name))
						errOut := fmt.Sprintf(`{"error":%q}`, toolErr.Error())
						if onToolEvent != nil {
							onToolEvent(map[string]any{
								"type":        "dynamic-tool",
								"call_id":     callID,
								"response_id": strings.TrimSpace(res.ID),
								"tool_name":   strings.TrimSpace(call.Name),
								"state":       "output-error",
								"error_text":  toolErr.Error(),
								"output_len":  len(strings.TrimSpace(errOut)),
								"output":      stringToMaybeJSONAny(errOut),
							})
						}
						out = errOut
					} else {
						toolOut, err := r.tools.Execute(ctx, call.Name, call.Arguments, call.CallID)
						if err != nil {
							errOut := fmt.Sprintf(`{"error":%q}`, err.Error())
							if onToolEvent != nil {
								onToolEvent(map[string]any{
									"type":        "dynamic-tool",
									"call_id":     callID,
									"response_id": strings.TrimSpace(res.ID),
									"tool_name":   strings.TrimSpace(call.Name),
									"state":       "output-error",
									"error_text":  err.Error(),
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
						errOut := fmt.Sprintf(`{"error":%q}`, err.Error())
						if onToolEvent != nil {
							onToolEvent(map[string]any{
								"type":        "dynamic-tool",
								"call_id":     callID,
								"response_id": strings.TrimSpace(res.ID),
								"tool_name":   strings.TrimSpace(call.Name),
								"state":       "output-error",
								"error_text":  err.Error(),
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
			replayCalls = append(replayCalls, buildReplayFunctionCallInputItem(call))
			outputs = append(outputs, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  out,
			})
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
			historyInputItems = append(historyInputItems, replayCalls...)
			historyInputItems = append(historyInputItems, outputs...)
			nextReq.Input = cloneResponseInputItems(historyInputItems)
		} else {
			nextReq.Input = outputs
			nextReq.PreviousResponseID = res.ID
		}
		req = nextReq
	}
	return "", fmt.Errorf("responses loop exceeded max iterations: %d", r.options.MaxIterations)
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

func buildReplayFunctionCallInputItem(call ToolCall) map[string]any {
	callID := strings.TrimSpace(call.CallID)
	arguments := strings.TrimSpace(string(call.Arguments))
	if arguments == "" {
		arguments = "{}"
	}
	return map[string]any{
		"type":      "function_call",
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
