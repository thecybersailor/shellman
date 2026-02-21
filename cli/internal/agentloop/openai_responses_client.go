package agentloop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
)

type OpenAIConfig struct {
	BaseURL string
	Model   string
	APIKey  string
}

type CreateResponseRequest struct {
	Model              string             `json:"model"`
	Input              any                `json:"input"`
	Tools              []ResponseToolSpec `json:"tools,omitempty"`
	PreviousResponseID string             `json:"previous_response_id,omitempty"`
	Store              *bool              `json:"store,omitempty"`
	Stream             bool               `json:"stream,omitempty"`
}

type ToolCall struct {
	ID         string
	CallID     string
	ResponseID string
	Name       string
	Arguments  json.RawMessage
}

type CreateResponseResult struct {
	ID         string
	FinalText  string
	ToolCalls  []ToolCall
	EventTrace []string
}

func (r CreateResponseResult) HasFinalText() bool {
	return strings.TrimSpace(r.FinalText) != ""
}

type ResponsesClient struct {
	cfg     OpenAIConfig
	service responses.ResponseService
}

func NewResponsesClient(cfg OpenAIConfig, httpClient *http.Client) *ResponsesClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	opts := []option.RequestOption{option.WithHTTPClient(httpClient)}
	if base := strings.TrimSpace(cfg.BaseURL); base != "" {
		opts = append(opts, option.WithBaseURL(base))
	}
	if key := strings.TrimSpace(cfg.APIKey); key != "" {
		opts = append(opts, option.WithAPIKey(key))
	}
	return &ResponsesClient{
		cfg:     cfg,
		service: responses.NewResponseService(opts...),
	}
}

func (c *ResponsesClient) CreateResponse(ctx context.Context, req CreateResponseRequest) (*CreateResponseResult, error) {
	req.Stream = false
	params, err := c.toSDKRequest(req)
	if err != nil {
		return nil, err
	}
	emitResponsesRequestRaw(ctx, marshalJSONForDebug(params))
	var rawResp *http.Response
	var rawBody []byte
	_, err = c.service.New(
		ctx,
		params,
		option.WithResponseInto(&rawResp),
		option.WithResponseBodyInto(&rawBody),
	)
	if err != nil {
		return nil, c.wrapRequestError(err, req, rawResp)
	}
	if len(rawBody) == 0 {
		return nil, fmt.Errorf("responses api returned empty response request=%s", summarizeCreateResponseRequest(req))
	}
	emitResponsesResponseRaw(ctx, string(rawBody))
	return parseResponseResult(rawBody)
}

func (c *ResponsesClient) CreateResponseStream(ctx context.Context, req CreateResponseRequest, onTextDelta func(string)) (*CreateResponseResult, error) {
	req.Stream = true
	params, err := c.toSDKRequest(req)
	if err != nil {
		return nil, err
	}
	emitResponsesRequestRaw(ctx, marshalJSONForDebug(params))
	var rawResp *http.Response
	stream := c.service.NewStreaming(ctx, params, option.WithResponseInto(&rawResp))
	if stream == nil {
		return nil, fmt.Errorf("responses stream unavailable request=%s", summarizeCreateResponseRequest(req))
	}
	defer stream.Close()

	out := &CreateResponseResult{}
	eventTrace := make([]string, 0, 32)
	sawTextDelta := false
	toolCallOrder := make([]string, 0, 8)
	toolCallsByKey := map[string]*ToolCall{}
	itemToKey := map[string]string{}
	argumentByKey := map[string]string{}

	resolveToolCallKey := func(candidates ...string) string {
		normalized := make([]string, 0, len(candidates))
		for _, raw := range candidates {
			key := strings.TrimSpace(raw)
			if key == "" {
				continue
			}
			normalized = append(normalized, key)
		}
		for _, key := range normalized {
			if mapped := strings.TrimSpace(itemToKey[key]); mapped != "" {
				return mapped
			}
		}
		for _, key := range normalized {
			if _, ok := toolCallsByKey[key]; ok {
				return key
			}
		}
		if len(normalized) > 0 {
			return normalized[0]
		}
		return ""
	}
	ensureToolCall := func(key string) *ToolCall {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil
		}
		if existing, ok := toolCallsByKey[key]; ok {
			return existing
		}
		tc := &ToolCall{}
		toolCallsByKey[key] = tc
		toolCallOrder = append(toolCallOrder, key)
		return tc
	}
	rememberItemKey := func(itemID, key string) {
		itemID = strings.TrimSpace(itemID)
		key = strings.TrimSpace(key)
		if itemID == "" || key == "" {
			return
		}
		itemToKey[itemID] = key
	}
	buildToolCalls := func() []ToolCall {
		outCalls := make([]ToolCall, 0, len(toolCallOrder))
		for _, key := range toolCallOrder {
			tc := toolCallsByKey[key]
			if tc == nil {
				continue
			}
			call := *tc
			if strings.TrimSpace(call.ResponseID) == "" {
				call.ResponseID = strings.TrimSpace(out.ID)
			}
			outCalls = append(outCalls, call)
		}
		return outCalls
	}
	setArguments := func(arguments string, candidates ...string) {
		if strings.TrimSpace(arguments) == "" {
			return
		}
		key := resolveToolCallKey(candidates...)
		if key == "" {
			return
		}
		tc := ensureToolCall(key)
		if tc == nil {
			return
		}
		argumentByKey[key] = arguments
		tc.Arguments = json.RawMessage(strings.TrimSpace(argumentByKey[key]))
		for _, candidate := range candidates {
			rememberItemKey(candidate, key)
		}
	}
	appendArgumentsDelta := func(delta string, candidates ...string) {
		if strings.TrimSpace(delta) == "" {
			return
		}
		key := resolveToolCallKey(candidates...)
		if key == "" {
			return
		}
		tc := ensureToolCall(key)
		if tc == nil {
			return
		}
		argumentByKey[key] += delta
		if merged := strings.TrimSpace(argumentByKey[key]); merged != "" {
			tc.Arguments = json.RawMessage(merged)
		}
		for _, candidate := range candidates {
			rememberItemKey(candidate, key)
		}
	}
	mergeToolCall := func(call ToolCall, eventItemID, eventResponseID string) {
		key := resolveToolCallKey(call.CallID, call.ID, eventItemID)
		if key == "" {
			return
		}
		tc := ensureToolCall(key)
		if tc == nil {
			return
		}
		inID := strings.TrimSpace(call.ID)
		inCallID := strings.TrimSpace(call.CallID)
		if inID != "" && (strings.TrimSpace(tc.ID) == "" || strings.TrimSpace(tc.ID) == strings.TrimSpace(tc.CallID)) {
			tc.ID = inID
		}
		if inCallID != "" && (strings.TrimSpace(tc.CallID) == "" || strings.TrimSpace(tc.CallID) == strings.TrimSpace(tc.ID)) {
			tc.CallID = inCallID
		}
		if strings.TrimSpace(tc.Name) == "" {
			tc.Name = strings.TrimSpace(call.Name)
		}
		if strings.TrimSpace(tc.ResponseID) == "" {
			tc.ResponseID = strings.TrimSpace(call.ResponseID)
			if strings.TrimSpace(tc.ResponseID) == "" {
				tc.ResponseID = strings.TrimSpace(eventResponseID)
			}
		}
		deltaArgs := strings.TrimSpace(argumentByKey[key])
		itemArgs := strings.TrimSpace(string(call.Arguments))
		if deltaArgs != "" {
			tc.Arguments = json.RawMessage(deltaArgs)
		} else if itemArgs != "" {
			tc.Arguments = call.Arguments
		}
		rememberItemKey(call.ID, key)
		rememberItemKey(call.CallID, key)
		rememberItemKey(eventItemID, key)
	}

	processEvent := func(data string) error {
		data = strings.TrimSpace(data)
		if data == "" {
			return nil
		}
		emitResponsesStreamEventRaw(ctx, data)
		var event struct {
			Type       string          `json:"type"`
			Delta      string          `json:"delta"`
			ResponseID string          `json:"response_id"`
			ItemID     string          `json:"item_id"`
			Name       string          `json:"name"`
			Arguments  string          `json:"arguments"`
			Sequence   int             `json:"sequence_number"`
			Item       responseItem    `json:"item"`
			Response   responsePayload `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return fmt.Errorf("invalid responses stream event: %w data=%q", err, clipForLog(data, 600))
		}
		eventResponseID := strings.TrimSpace(event.Response.ID)
		if eventResponseID == "" {
			eventResponseID = strings.TrimSpace(event.ResponseID)
		}
		if eventResponseID == "" {
			eventResponseID = strings.TrimSpace(event.Item.ResponseID)
		}
		if out.ID == "" && eventResponseID != "" {
			out.ID = eventResponseID
		}
		eventTrace = appendEventTrace(eventTrace, summarizeStreamEventForTrace(event.Type, eventResponseID, event.Item, event.ItemID, event.Name, event.Arguments, event.Sequence))
		switch strings.TrimSpace(event.Type) {
		case "response.created":
		case "response.output_text.delta":
			if event.Delta == "" {
				return nil
			}
			sawTextDelta = true
			out.FinalText += event.Delta
			if onTextDelta != nil {
				onTextDelta(event.Delta)
			}
		case "response.output_item.added", "response.output_item.done":
			if call, ok := toToolCall(event.Item); ok {
				if strings.TrimSpace(call.ResponseID) == "" {
					call.ResponseID = eventResponseID
				}
				mergeToolCall(call, event.ItemID, eventResponseID)
				out.ToolCalls = buildToolCalls()
				return nil
			}
			if !sawTextDelta {
				appendMessageText(out, event.Item.Content)
			}
		case "response.function_call_arguments.delta":
			appendArgumentsDelta(event.Delta, event.ItemID, event.Item.ID, event.Item.CallID)
			out.ToolCalls = buildToolCalls()
		case "response.function_call_arguments.done":
			setArguments(event.Arguments, event.ItemID, event.Item.ID, event.Item.CallID)
			out.ToolCalls = buildToolCalls()
		case "response.completed":
			for _, item := range event.Response.Output {
				if call, ok := toToolCall(item); ok {
					if strings.TrimSpace(call.ResponseID) == "" {
						call.ResponseID = eventResponseID
					}
					mergeToolCall(call, item.ID, eventResponseID)
					continue
				}
				if !sawTextDelta {
					appendMessageText(out, item.Content)
				}
			}
			out.ToolCalls = buildToolCalls()
		}
		return nil
	}

	for stream.Next() {
		event := stream.Current()
		if err := processEvent(event.RawJSON()); err != nil {
			return nil, err
		}
	}
	if err := stream.Err(); err != nil {
		return nil, c.wrapRequestError(err, req, rawResp)
	}
	out.ToolCalls = buildToolCalls()
	out.EventTrace = append([]string(nil), eventTrace...)
	return out, nil
}

func (c *ResponsesClient) toSDKRequest(req CreateResponseRequest) (responses.ResponseNewParams, error) {
	var out responses.ResponseNewParams
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(c.cfg.Model)
	}
	if model != "" {
		out.Model = model
	}
	if req.Store != nil {
		out.Store = param.NewOpt(*req.Store)
	}
	if prev := strings.TrimSpace(req.PreviousResponseID); prev != "" {
		out.PreviousResponseID = param.NewOpt(prev)
	}
	input, err := toSDKInput(req.Input)
	if err != nil {
		return responses.ResponseNewParams{}, err
	}
	out.Input = input
	if len(req.Tools) > 0 {
		tools, err := toSDKTools(req.Tools)
		if err != nil {
			return responses.ResponseNewParams{}, err
		}
		out.Tools = tools
	}
	return out, nil
}

func toSDKInput(input any) (responses.ResponseNewParamsInputUnion, error) {
	var out responses.ResponseNewParamsInputUnion
	if input == nil {
		return out, nil
	}
	switch v := input.(type) {
	case string:
		out.OfString = param.NewOpt(v)
		return out, nil
	case []map[string]any:
		items := make(responses.ResponseInputParam, 0, len(v))
		for i, rawItem := range v {
			item, err := toSDKInputItem(rawItem)
			if err != nil {
				return responses.ResponseNewParamsInputUnion{}, fmt.Errorf("invalid response input item[%d]: %w", i, err)
			}
			items = append(items, item)
		}
		out.OfInputItemList = items
		return out, nil
	case []any:
		items := make(responses.ResponseInputParam, 0, len(v))
		for i, rawItem := range v {
			item, err := toSDKInputItem(rawItem)
			if err != nil {
				return responses.ResponseNewParamsInputUnion{}, fmt.Errorf("invalid response input item[%d]: %w", i, err)
			}
			items = append(items, item)
		}
		out.OfInputItemList = items
		return out, nil
	default:
		raw, err := json.Marshal(input)
		if err != nil {
			return responses.ResponseNewParamsInputUnion{}, fmt.Errorf("marshal response input failed: %w", err)
		}
		trimmed := strings.TrimSpace(string(raw))
		if trimmed == "" || trimmed == "null" {
			return out, nil
		}
		if strings.HasPrefix(trimmed, "[") {
			var items []responses.ResponseInputItemUnionParam
			if err := json.Unmarshal(raw, &items); err != nil {
				return responses.ResponseNewParamsInputUnion{}, fmt.Errorf("decode response input list failed: %w", err)
			}
			out.OfInputItemList = items
			return out, nil
		}
		if strings.HasPrefix(trimmed, "\"") {
			var text string
			if err := json.Unmarshal(raw, &text); err != nil {
				return responses.ResponseNewParamsInputUnion{}, fmt.Errorf("decode response input text failed: %w", err)
			}
			out.OfString = param.NewOpt(text)
			return out, nil
		}
		return responses.ResponseNewParamsInputUnion{}, fmt.Errorf("unsupported response input type=%T", input)
	}
}

func toSDKInputItem(rawItem any) (responses.ResponseInputItemUnionParam, error) {
	raw, err := json.Marshal(rawItem)
	if err != nil {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("marshal response input item failed: %w", err)
	}
	var out responses.ResponseInputItemUnionParam
	if err := json.Unmarshal(raw, &out); err != nil {
		return responses.ResponseInputItemUnionParam{}, fmt.Errorf("decode response input item failed: %w", err)
	}
	return out, nil
}

func toSDKTools(tools []ResponseToolSpec) ([]responses.ToolUnionParam, error) {
	out := make([]responses.ToolUnionParam, 0, len(tools))
	for i, spec := range tools {
		raw, err := json.Marshal(spec)
		if err != nil {
			return nil, fmt.Errorf("marshal response tool[%d] failed: %w", i, err)
		}
		var tool responses.ToolUnionParam
		if err := json.Unmarshal(raw, &tool); err != nil {
			return nil, fmt.Errorf("decode response tool[%d] failed: %w", i, err)
		}
		out = append(out, tool)
	}
	return out, nil
}

func parseResponseResult(raw []byte) (*CreateResponseResult, error) {
	var decoded responsePayload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	out := &CreateResponseResult{ID: strings.TrimSpace(decoded.ID)}
	for _, item := range decoded.Output {
		if call, ok := toToolCall(item); ok {
			if strings.TrimSpace(call.ResponseID) == "" {
				call.ResponseID = out.ID
			}
			out.ToolCalls = append(out.ToolCalls, call)
			continue
		}
		appendMessageText(out, item.Content)
	}
	return out, nil
}

func (c *ResponsesClient) wrapRequestError(err error, req CreateResponseRequest, rawResp *http.Response) error {
	var apiErr *responses.Error
	if errors.As(err, &apiErr) {
		resp := rawResp
		if resp == nil {
			resp = apiErr.Response
		}
		body := strings.TrimSpace(apiErr.RawJSON())
		if body == "" {
			body = strings.TrimSpace(err.Error())
		}
		return fmt.Errorf(
			"responses api status %d request_id=%q headers=%s request=%s response=%s",
			apiErr.StatusCode,
			responseRequestID(resp),
			summarizeResponseHeaders(resp),
			summarizeCreateResponseRequest(req),
			body,
		)
	}
	return fmt.Errorf("responses request failed request=%s: %w", summarizeCreateResponseRequest(req), err)
}

type responseContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responseItem struct {
	Type       string                `json:"type"`
	ID         string                `json:"id"`
	ResponseID string                `json:"response_id"`
	CallID     string                `json:"call_id"`
	Name       string                `json:"name"`
	Arguments  string                `json:"arguments"`
	Content    []responseContentPart `json:"content"`
}

type responsePayload struct {
	ID     string         `json:"id"`
	Output []responseItem `json:"output"`
}

func responseRequestID(resp *http.Response) string {
	if resp == nil || resp.Header == nil {
		return ""
	}
	for _, key := range []string{"x-request-id", "request-id", "openai-request-id", "x-openai-request-id"} {
		value := strings.TrimSpace(resp.Header.Get(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func summarizeResponseHeaders(resp *http.Response) string {
	if resp == nil || resp.Header == nil {
		return "{}"
	}
	keys := []string{
		"x-request-id",
		"request-id",
		"openai-request-id",
		"x-openai-request-id",
		"openrouter-request-id",
		"openrouter-model",
		"via",
		"server",
		"cf-ray",
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := strings.TrimSpace(resp.Header.Get(k))
		if v == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%q", k, v))
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func toToolCall(item responseItem) (ToolCall, bool) {
	if strings.TrimSpace(item.Type) != "function_call" {
		return ToolCall{}, false
	}
	return ToolCall{
		ID:         strings.TrimSpace(item.ID),
		CallID:     strings.TrimSpace(item.CallID),
		ResponseID: strings.TrimSpace(item.ResponseID),
		Name:       strings.TrimSpace(item.Name),
		Arguments:  json.RawMessage(item.Arguments),
	}, true
}

func appendMessageText(out *CreateResponseResult, parts []responseContentPart) {
	if out == nil {
		return
	}
	for _, content := range parts {
		if strings.TrimSpace(content.Type) != "output_text" || strings.TrimSpace(content.Text) == "" {
			continue
		}
		if out.FinalText == "" {
			out.FinalText = content.Text
		} else {
			out.FinalText += "\n" + content.Text
		}
	}
}

func appendEventTrace(trace []string, entry string) []string {
	if strings.TrimSpace(entry) == "" {
		return trace
	}
	if len(trace) >= 40 {
		trace = trace[1:]
	}
	return append(trace, entry)
}

func marshalJSONForDebug(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("{\"marshal_error\":%q}", err.Error())
	}
	return string(raw)
}

func summarizeStreamEventForTrace(
	eventType string,
	responseID string,
	item responseItem,
	itemID string,
	name string,
	arguments string,
	sequence int,
) string {
	eventType = strings.TrimSpace(eventType)
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		responseID = strings.TrimSpace(item.ResponseID)
	}
	itemType := strings.TrimSpace(item.Type)
	if itemType == "" && strings.Contains(eventType, "function_call_arguments") {
		itemType = "function_call_arguments"
	}
	callID := strings.TrimSpace(item.CallID)
	if callID == "" {
		callID = strings.TrimSpace(item.ID)
	}
	resolvedItemID := strings.TrimSpace(item.ID)
	if resolvedItemID == "" {
		resolvedItemID = strings.TrimSpace(itemID)
	}
	args := strings.TrimSpace(item.Arguments)
	if args == "" {
		args = strings.TrimSpace(arguments)
	}
	toolName := strings.TrimSpace(item.Name)
	if toolName == "" {
		toolName = strings.TrimSpace(name)
	}
	return fmt.Sprintf(
		"%s(seq=%d resp=%s item_type=%s item_id=%s call_id=%s tool=%s args_len=%d)",
		eventType,
		sequence,
		responseID,
		itemType,
		resolvedItemID,
		callID,
		toolName,
		len(args),
	)
}
