package agentloop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeNamedTool struct {
	name string
}

func (f fakeNamedTool) Name() string { return f.name }
func (f fakeNamedTool) Spec() ResponseToolSpec {
	return ResponseToolSpec{Type: "function", Name: f.name}
}
func (f fakeNamedTool) Execute(context.Context, json.RawMessage, string) (string, *ToolError) {
	return `{"ok":true}`, nil
}

func registerTestSetFlagTool(t *testing.T, registry *ToolRegistry) {
	t.Helper()
	if err := registry.Register(fakeNamedTool{name: "task.current.set_flag"}); err != nil {
		t.Fatalf("register tool failed: %v", err)
	}
}

func TestLoopRunner_UsesResponsesAPIAndToolRoundtrip(t *testing.T) {
	callCount := 0
	requestBodies := make([]map[string]any, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses path, got %s", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		requestBodies = append(requestBodies, req)
		callCount++
		if callCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_1",
				"output": []map[string]any{
					{
						"type":      "function_call",
						"id":        "fc_1",
						"call_id":   "call_1",
						"name":      "task.current.set_flag",
						"arguments": `{"flag":"success","status_message":"ok"}`,
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "resp_2",
			"output": []map[string]any{
				{
					"type": "message",
					"content": []map[string]any{
						{"type": "output_text", "text": "SHELLMAN_E2E_OK"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	registry := NewToolRegistry()
	registerTestSetFlagTool(t, registry)
	runner := NewLoopRunner(client, registry, LoopRunnerOptions{MaxIterations: 4})

	out, err := runner.Run(context.Background(), "Reply exactly: SHELLMAN_E2E_OK")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if strings.TrimSpace(out) != "SHELLMAN_E2E_OK" {
		t.Fatalf("unexpected final output: %q", out)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 responses calls, got %d", callCount)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected 2 request bodies, got %d", len(requestBodies))
	}

	firstTools, ok := requestBodies[0]["tools"].([]any)
	if !ok || len(firstTools) == 0 {
		t.Fatalf("expected first request carries tools, got %#v", requestBodies[0]["tools"])
	}
	secondInput, ok := requestBodies[1]["input"].([]any)
	if !ok || len(secondInput) < 3 {
		t.Fatalf("expected second request input has full context items, got %#v", requestBodies[1]["input"])
	}
	lastItem, ok := secondInput[len(secondInput)-1].(map[string]any)
	if !ok {
		t.Fatalf("expected last item map, got %#v", secondInput[len(secondInput)-1])
	}
	if got := strings.TrimSpace(anyToString(lastItem["type"])); got != "function_call_output" {
		t.Fatalf("expected last item type=function_call_output, got %q", got)
	}
	if got := strings.TrimSpace(anyToString(lastItem["call_id"])); got != "call_1" {
		t.Fatalf("expected call_id=call_1, got %q", got)
	}
	if got := strings.TrimSpace(anyToString(requestBodies[1]["previous_response_id"])); got != "" {
		t.Fatalf("expected no previous_response_id in full-context mode, got %q", got)
	}
}

func TestLoopRunner_FullContextReplay_InterleavesCallAndOutputWithCallIDAndID(t *testing.T) {
	callCount := 0
	requestBodies := make([]map[string]any, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		requestBodies = append(requestBodies, req)
		callCount++
		if callCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_1",
				"output": []map[string]any{
					{
						"type":      "function_call",
						"id":        "fc_a",
						"call_id":   "call_a",
						"name":      "task.current.set_flag",
						"arguments": `{"flag":"success","status_message":"ok"}`,
					},
					{
						"type":      "function_call",
						"id":        "fc_b",
						"call_id":   "call_b",
						"name":      "write_stdin",
						"arguments": `{"input":"pwd"}`,
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "resp_2",
			"output": []map[string]any{
				{
					"type": "message",
					"content": []map[string]any{
						{"type": "output_text", "text": "DONE"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	registry := NewToolRegistry()
	registerTestSetFlagTool(t, registry)
	if err := registry.Register(fakeNamedTool{name: "write_stdin"}); err != nil {
		t.Fatalf("register write_stdin failed: %v", err)
	}
	runner := NewLoopRunner(client, registry, LoopRunnerOptions{MaxIterations: 4})

	out, err := runner.Run(context.Background(), "use tools")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if strings.TrimSpace(out) != "DONE" {
		t.Fatalf("unexpected final output: %q", out)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected 2 request bodies, got %d", len(requestBodies))
	}

	secondInput, ok := requestBodies[1]["input"].([]any)
	if !ok || len(secondInput) != 5 {
		t.Fatalf("expected full context with 5 input items, got %#v", requestBodies[1]["input"])
	}
	type itemCheck struct {
		index    int
		wantType string
		callID   string
		id       string
	}
	checks := []itemCheck{
		{index: 1, wantType: "function_call", callID: "call_a", id: "fc_a"},
		{index: 2, wantType: "function_call_output", callID: "call_a"},
		{index: 3, wantType: "function_call", callID: "call_b", id: "fc_b"},
		{index: 4, wantType: "function_call_output", callID: "call_b"},
	}
	for _, check := range checks {
		item, ok := secondInput[check.index].(map[string]any)
		if !ok {
			t.Fatalf("expected input[%d] is object, got %#v", check.index, secondInput[check.index])
		}
		if got := strings.TrimSpace(anyToString(item["type"])); got != check.wantType {
			t.Fatalf("expected input[%d].type=%q, got %q", check.index, check.wantType, got)
		}
		if got := strings.TrimSpace(anyToString(item["call_id"])); got != check.callID {
			t.Fatalf("expected input[%d].call_id=%q, got %q", check.index, check.callID, got)
		}
		if check.id != "" {
			if got := strings.TrimSpace(anyToString(item["id"])); got != check.id {
				t.Fatalf("expected input[%d].id=%q, got %q", check.index, check.id, got)
			}
		}
	}
}

func anyToString(v any) string {
	s, _ := v.(string)
	return s
}

func TestLoopRunner_RunStream_EmitsTextDeltas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		stream, _ := req["stream"].(bool)
		if !stream {
			t.Fatalf("expected stream=true")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"SHELLMAN \"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"STREAM\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_stream\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	runner := NewLoopRunner(client, nil, LoopRunnerOptions{MaxIterations: 2})

	var chunks []string
	out, err := runner.RunStream(context.Background(), "Reply exactly: SHELLMAN STREAM", func(delta string) {
		chunks = append(chunks, delta)
	})
	if err != nil {
		t.Fatalf("RunStream failed: %v", err)
	}
	if strings.TrimSpace(out) != "SHELLMAN STREAM" {
		t.Fatalf("unexpected stream output: %q", out)
	}
	if got := strings.Join(chunks, ""); got != "SHELLMAN STREAM" {
		t.Fatalf("unexpected streamed chunks: %q", got)
	}
}

func TestLoopRunner_RunStreamWithTools_EmitsToolEvents(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_tools_1",
				"output": []map[string]any{
					{
						"type":      "function_call",
						"id":        "fc_1",
						"call_id":   "call_1",
						"name":      "task.current.set_flag",
						"arguments": `{"flag":"success","status_message":"ok"}`,
					},
				},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "resp_tools_2",
			"output": []map[string]any{
				{
					"type": "message",
					"content": []map[string]any{
						{"type": "output_text", "text": "DONE"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	registry := NewToolRegistry()
	registerTestSetFlagTool(t, registry)
	runner := NewLoopRunner(client, registry, LoopRunnerOptions{MaxIterations: 4})

	var events []map[string]any
	out, err := runner.RunStreamWithTools(context.Background(), "use tool", nil, func(evt map[string]any) {
		events = append(events, evt)
	})
	if err != nil {
		t.Fatalf("RunStreamWithTools failed: %v", err)
	}
	if strings.TrimSpace(out) != "DONE" {
		t.Fatalf("unexpected output: %q", out)
	}
	toolEvents := make([]map[string]any, 0, len(events))
	for _, evt := range events {
		if strings.TrimSpace(anyToString(evt["type"])) != "dynamic-tool" {
			continue
		}
		toolEvents = append(toolEvents, evt)
	}
	if len(toolEvents) < 2 {
		t.Fatalf("expected at least 2 dynamic tool events, got %d", len(toolEvents))
	}
	if got := strings.TrimSpace(anyToString(toolEvents[0]["state"])); got != "input-available" {
		t.Fatalf("expected first event input-available, got %q", got)
	}
	if got := strings.TrimSpace(anyToString(toolEvents[len(toolEvents)-1]["state"])); got != "output-available" {
		t.Fatalf("expected last event output-available, got %q", got)
	}
}

func TestLoopRunner_RunStreamWithTools_UsesStreamResponseIDForToolRoundtrip(t *testing.T) {
	callCount := 0
	requestBodies := make([]map[string]any, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses path, got %s", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		requestBodies = append(requestBodies, req)
		callCount++

		w.Header().Set("Content-Type", "text/event-stream")
		switch callCount {
		case 1:
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"response_id\":\"resp_stream_1\",\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"task.current.set_flag\",\"arguments\":\"{\\\"flag\\\":\\\"success\\\",\\\"status_message\\\":\\\"ok\\\"}\"}}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		case 2:
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"DONE\"}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_stream_2\"}}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			t.Fatalf("unexpected responses call count: %d", callCount)
		}
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	registry := NewToolRegistry()
	registerTestSetFlagTool(t, registry)
	runner := NewLoopRunner(client, registry, LoopRunnerOptions{MaxIterations: 4})

	out, err := runner.RunStreamWithTools(context.Background(), "use tool", func(string) {}, nil)
	if err != nil {
		t.Fatalf("RunStreamWithTools failed: %v", err)
	}
	if strings.TrimSpace(out) != "DONE" {
		t.Fatalf("unexpected output: %q", out)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 responses calls, got %d", callCount)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected 2 request bodies, got %d", len(requestBodies))
	}
	if got := strings.TrimSpace(anyToString(requestBodies[1]["previous_response_id"])); got != "" {
		t.Fatalf("expected no previous_response_id in full-context mode, got %q", got)
	}
	secondInput, ok := requestBodies[1]["input"].([]any)
	if !ok || len(secondInput) < 3 {
		t.Fatalf("expected second request input has full context items, got %#v", requestBodies[1]["input"])
	}
	inputItem, ok := secondInput[len(secondInput)-1].(map[string]any)
	if !ok {
		t.Fatalf("expected function_call_output object, got %#v", secondInput[len(secondInput)-1])
	}
	if got := strings.TrimSpace(anyToString(inputItem["call_id"])); got != "call_1" {
		t.Fatalf("expected call_id=call_1, got %q", got)
	}
}

func TestLoopRunner_RunStreamWithTools_UsesNestedResponseIDForToolRoundtrip(t *testing.T) {
	callCount := 0
	requestBodies := make([]map[string]any, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		requestBodies = append(requestBodies, req)
		callCount++

		w.Header().Set("Content-Type", "text/event-stream")
		switch callCount {
		case 1:
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"response\":{\"id\":\"resp_stream_nested_1\"},\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_nested_1\",\"name\":\"task.current.set_flag\",\"arguments\":\"{\\\"flag\\\":\\\"success\\\",\\\"status_message\\\":\\\"ok\\\"}\"}}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		case 2:
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"DONE\"}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_stream_nested_2\"}}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			t.Fatalf("unexpected responses call count: %d", callCount)
		}
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	registry := NewToolRegistry()
	registerTestSetFlagTool(t, registry)
	runner := NewLoopRunner(client, registry, LoopRunnerOptions{MaxIterations: 4})

	out, err := runner.RunStreamWithTools(context.Background(), "use tool", func(string) {}, nil)
	if err != nil {
		t.Fatalf("RunStreamWithTools failed: %v", err)
	}
	if strings.TrimSpace(out) != "DONE" {
		t.Fatalf("unexpected output: %q", out)
	}
	if got := strings.TrimSpace(anyToString(requestBodies[1]["previous_response_id"])); got != "" {
		t.Fatalf("expected no previous_response_id in full-context mode, got %q", got)
	}
}

func TestLoopRunner_RunStreamWithTools_SucceedsWhenStreamResponseIDMissingInFullContextMode(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		callCount++
		w.Header().Set("Content-Type", "text/event-stream")
		switch callCount {
		case 1:
			if _, ok := req["input"].([]any); !ok {
				t.Fatalf("expected first request input list in full-context mode, got %#v", req["input"])
			}
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"id\":\"fc_1\",\"call_id\":\"call_1\",\"name\":\"task.current.set_flag\",\"arguments\":\"{\\\"flag\\\":\\\"success\\\",\\\"status_message\\\":\\\"ok\\\"}\"}}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		case 2:
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"DONE\"}\n\n")
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\"}}\n\n")
			_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		default:
			t.Fatalf("unexpected call count: %d", callCount)
		}
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	registry := NewToolRegistry()
	registerTestSetFlagTool(t, registry)
	runner := NewLoopRunner(client, registry, LoopRunnerOptions{MaxIterations: 4})

	out, err := runner.RunStreamWithTools(context.Background(), "use tool", func(string) {}, nil)
	if err != nil {
		t.Fatalf("RunStreamWithTools failed: %v", err)
	}
	if strings.TrimSpace(out) != "DONE" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestLoopRunner_StoreEnabled_DoesNotUsePreviousResponseIDAndBuildsFullContext(t *testing.T) {
	callCount := 0
	requestBodies := make([]map[string]any, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		requestBodies = append(requestBodies, req)
		callCount++
		switch callCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_1",
				"output": []map[string]any{
					{
						"type":      "function_call",
						"id":        "fc_1",
						"call_id":   "call_1",
						"name":      "task.current.set_flag",
						"arguments": `{"flag":"success","status_message":"ok"}`,
					},
				},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_2",
				"output": []map[string]any{
					{
						"type": "message",
						"content": []map[string]any{
							{"type": "output_text", "text": "DONE"},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected call count: %d", callCount)
		}
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	registry := NewToolRegistry()
	registerTestSetFlagTool(t, registry)
	runner := NewLoopRunner(client, registry, LoopRunnerOptions{MaxIterations: 4})

	ctx := WithTaskScope(context.Background(), TaskScope{
		TaskID:              "t1",
		ProjectID:           "p1",
		Source:              "user",
		ResponsesStore:      true,
		DisableStoreContext: false,
	})
	out, err := runner.Run(ctx, "use tool")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if strings.TrimSpace(out) != "DONE" {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(requestBodies))
	}
	if got := strings.TrimSpace(anyToString(requestBodies[1]["previous_response_id"])); got != "" {
		t.Fatalf("expected no previous_response_id when store enabled, got %q", got)
	}
	secondInput, ok := requestBodies[1]["input"].([]any)
	if !ok || len(secondInput) < 3 {
		t.Fatalf("expected full context input items, got %#v", requestBodies[1]["input"])
	}
}

func TestLoopRunner_RefreshesAllowedToolsEachIteration(t *testing.T) {
	callCount := 0
	requestBodies := make([]map[string]any, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		requestBodies = append(requestBodies, req)
		callCount++
		switch callCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_1",
				"output": []map[string]any{
					{
						"type":      "function_call",
						"id":        "fc_1",
						"call_id":   "call_1",
						"name":      "exec_command",
						"arguments": `{"command":"codex","max_output_tokens":4000}`,
					},
				},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_2",
				"output": []map[string]any{
					{
						"type": "message",
						"content": []map[string]any{
							{"type": "output_text", "text": "DONE"},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected call count: %d", callCount)
		}
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	registry := NewToolRegistry()
	if err := registry.Register(fakeNamedTool{name: "exec_command"}); err != nil {
		t.Fatalf("register exec_command failed: %v", err)
	}
	if err := registry.Register(fakeNamedTool{name: "task.input_prompt"}); err != nil {
		t.Fatalf("register task.input_prompt failed: %v", err)
	}
	runner := NewLoopRunner(client, registry, LoopRunnerOptions{MaxIterations: 4})

	resolverCalls := 0
	ctx := WithAllowedToolNamesResolver(context.Background(), func() []string {
		resolverCalls++
		if resolverCalls == 1 {
			return []string{"exec_command"}
		}
		return []string{"task.input_prompt", "write_stdin"}
	})
	out, err := runner.Run(ctx, "use tool")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if strings.TrimSpace(out) != "DONE" {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected 2 request bodies, got %d", len(requestBodies))
	}
	firstTools := requestToolNames(requestBodies[0])
	secondTools := requestToolNames(requestBodies[1])
	if len(firstTools) != 1 || firstTools[0] != "exec_command" {
		t.Fatalf("unexpected first request tools: %#v", firstTools)
	}
	if !containsString(secondTools, "task.input_prompt") {
		t.Fatalf("second request should include task.input_prompt, got %#v", secondTools)
	}
	if containsString(secondTools, "exec_command") {
		t.Fatalf("second request should not include exec_command, got %#v", secondTools)
	}
	if resolverCalls < 2 {
		t.Fatalf("resolver should be called at least twice, got %d", resolverCalls)
	}
}

func TestLoopRunner_ExplicitEmptyAllowlist_DisablesAllTools(t *testing.T) {
	callCount := 0
	requestBodies := make([]map[string]any, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		requestBodies = append(requestBodies, req)
		callCount++
		switch callCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_1",
				"output": []map[string]any{
					{
						"type":      "function_call",
						"id":        "fc_1",
						"call_id":   "call_1",
						"name":      "exec_command",
						"arguments": `{"command":"pwd"}`,
					},
				},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_2",
				"output": []map[string]any{
					{
						"type": "message",
						"content": []map[string]any{
							{"type": "output_text", "text": "DONE"},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected call count: %d", callCount)
		}
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	registry := NewToolRegistry()
	if err := registry.Register(fakeNamedTool{name: "exec_command"}); err != nil {
		t.Fatalf("register exec_command failed: %v", err)
	}
	runner := NewLoopRunner(client, registry, LoopRunnerOptions{MaxIterations: 4})

	ctx := WithAllowedToolNames(context.Background(), []string{})
	out, err := runner.Run(ctx, "use tool")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if strings.TrimSpace(out) != "DONE" {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected 2 request bodies, got %d", len(requestBodies))
	}
	firstTools := requestToolNames(requestBodies[0])
	if len(firstTools) != 0 {
		t.Fatalf("expected no tools in first request, got %#v", firstTools)
	}
	secondInput, ok := requestBodies[1]["input"].([]any)
	if !ok || len(secondInput) < 3 {
		t.Fatalf("expected full context input items, got %#v", requestBodies[1]["input"])
	}
	lastItem, ok := secondInput[len(secondInput)-1].(map[string]any)
	if !ok {
		t.Fatalf("expected last input item map, got %#v", secondInput[len(secondInput)-1])
	}
	rawOutput := strings.TrimSpace(anyToString(lastItem["output"]))
	var outputJSON map[string]any
	if err := json.Unmarshal([]byte(rawOutput), &outputJSON); err != nil {
		t.Fatalf("expected json output, got %q err=%v", rawOutput, err)
	}
	if strings.TrimSpace(anyToString(outputJSON["error"])) == "" {
		t.Fatalf("missing error field in output: %q", rawOutput)
	}
	if strings.TrimSpace(anyToString(outputJSON["suggest"])) == "" {
		t.Fatalf("missing suggest field in output: %q", rawOutput)
	}
}

func requestToolNames(req map[string]any) []string {
	rawTools, ok := req["tools"].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(rawTools))
	for _, item := range rawTools {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(anyToString(tool["name"]))
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

func containsString(items []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func TestClearCurrentCommandInPromptText_ClearsOnlyCurrentCommand(t *testing.T) {
	raw := "USER_INPUT_EVENT\nterminal_screen_state_json:\n" +
		`{"terminal_screen_state":{"current_command":"zsh","cwd":"/tmp/repo","viewport_text":"$ "}}` +
		"\n"
	got, changed := clearCurrentCommandInPromptText(raw)
	if !changed {
		t.Fatalf("expected prompt changed, got %q", got)
	}
	if !strings.Contains(got, `"current_command":""`) {
		t.Fatalf("expected current_command cleared, got %q", got)
	}
	if !strings.Contains(got, `"cwd":"/tmp/repo"`) {
		t.Fatalf("expected cwd preserved, got %q", got)
	}
	if !strings.Contains(got, `"viewport_text":"$ "`) {
		t.Fatalf("expected viewport_text preserved, got %q", got)
	}
}

func TestLoopRunner_ClearsStalePromptCurrentCommandOnModeChange(t *testing.T) {
	callCount := 0
	requestBodies := make([]map[string]any, 0, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		requestBodies = append(requestBodies, req)
		callCount++
		switch callCount {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_1",
				"output": []map[string]any{
					{
						"type":      "function_call",
						"id":        "fc_1",
						"call_id":   "call_1",
						"name":      "exec_command",
						"arguments": `{"command":"codex -s danger-full-access"}`,
					},
				},
			})
		case 2:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_2",
				"output": []map[string]any{
					{
						"type": "message",
						"content": []map[string]any{
							{"type": "output_text", "text": "DONE"},
						},
					},
				},
			})
		default:
			t.Fatalf("unexpected call count: %d", callCount)
		}
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)
	registry := NewToolRegistry()
	if err := registry.Register(fakeNamedTool{name: "exec_command"}); err != nil {
		t.Fatalf("register exec_command failed: %v", err)
	}
	if err := registry.Register(fakeNamedTool{name: "task.input_prompt"}); err != nil {
		t.Fatalf("register task.input_prompt failed: %v", err)
	}
	runner := NewLoopRunner(client, registry, LoopRunnerOptions{MaxIterations: 4})

	resolverCalls := 0
	ctx := WithAllowedToolNamesResolver(context.Background(), func() []string {
		resolverCalls++
		if resolverCalls == 1 {
			return []string{"exec_command"}
		}
		return []string{"task.input_prompt", "write_stdin"}
	})
	prompt := "USER_INPUT_EVENT\nterminal_screen_state_json:\n" +
		`{"terminal_screen_state":{"current_command":"zsh","cwd":"/tmp/repo","viewport_text":"$ "}}`
	out, err := runner.Run(ctx, prompt)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if strings.TrimSpace(out) != "DONE" {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(requestBodies) != 2 {
		t.Fatalf("expected 2 request bodies, got %d", len(requestBodies))
	}
	secondInput, ok := requestBodies[1]["input"].([]any)
	if !ok || len(secondInput) == 0 {
		t.Fatalf("expected second input items, got %#v", requestBodies[1]["input"])
	}
	foundUserPrompt := false
	for _, raw := range secondInput {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(anyToString(item["type"])) != "message" {
			continue
		}
		contentRaw, ok := item["content"].([]any)
		if !ok || len(contentRaw) == 0 {
			continue
		}
		part, ok := contentRaw[0].(map[string]any)
		if !ok {
			continue
		}
		text := strings.TrimSpace(anyToString(part["text"]))
		if !strings.Contains(text, "USER_INPUT_EVENT") {
			continue
		}
		foundUserPrompt = true
		if !strings.Contains(text, `"current_command":""`) {
			t.Fatalf("expected stale current_command cleared in second request, got %q", text)
		}
	}
	if !foundUserPrompt {
		t.Fatalf("expected USER_INPUT_EVENT message in second input, got %#v", secondInput)
	}
}

func TestLoopRunner_ToolContract_UsesOnlyActionTools(t *testing.T) {
	registry := NewToolRegistry()
	for _, name := range TaskActionToolContractNames() {
		if err := registry.Register(fakeNamedTool{name: name}); err != nil {
			t.Fatalf("register %s failed: %v", name, err)
		}
	}
	specs := registry.Specs()
	if len(specs) != len(TaskActionToolContractNames()) {
		t.Fatalf("unexpected tool spec count: %d", len(specs))
	}
	got := map[string]struct{}{}
	for _, spec := range specs {
		got[strings.TrimSpace(spec.Name)] = struct{}{}
	}
	for _, name := range TaskActionToolContractNames() {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing contract tool: %s", name)
		}
	}
	for _, old := range []string{"gateway_http", "task.current.input", "task.tty.state"} {
		if _, ok := got[old]; ok {
			t.Fatalf("legacy tool should be excluded: %s", old)
		}
	}
}
