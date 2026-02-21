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

func TestResponsesClient_CreateResponseStream_CollectsArgumentsFromDeltaChunks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses path, got %s", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body failed: %v", err)
		}
		stream, _ := req["stream"].(bool)
		if !stream {
			t.Fatalf("expected stream=true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_delta_1\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_delta_1\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.added\",\"response_id\":\"resp_delta_1\",\"item\":{\"type\":\"function_call\",\"id\":\"call_1\",\"call_id\":\"call_1\",\"name\":\"task.current.set_flag\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"response_id\":\"resp_delta_1\",\"item_id\":\"call_1\",\"delta\":\"{\\\"flag\\\":\\\"success\\\",\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"response_id\":\"resp_delta_1\",\"item_id\":\"call_1\",\"delta\":\"\\\"status_message\\\":\\\"ok\\\"}\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"response_id\":\"resp_delta_1\",\"item\":{\"type\":\"function_call\",\"id\":\"call_1\",\"call_id\":\"call_1\",\"name\":\"task.current.set_flag\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_delta_1\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)

	out, err := client.CreateResponseStream(context.Background(), CreateResponseRequest{
		Input: "test",
	}, nil)
	if err != nil {
		t.Fatalf("CreateResponseStream failed: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil response result")
	}
	if got := strings.TrimSpace(out.ID); got != "resp_delta_1" {
		t.Fatalf("expected response id resp_delta_1, got %q", got)
	}
	if len(out.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(out.ToolCalls))
	}
	args := strings.TrimSpace(string(out.ToolCalls[0].Arguments))
	if !strings.Contains(args, `"flag":"success"`) || !strings.Contains(args, `"status_message":"ok"`) {
		t.Fatalf("expected merged arguments from delta chunks, got %q", args)
	}
}

func TestResponsesClient_CreateResponseStream_PropagatesFailedEventError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_fail_1\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.in_progress\",\"response\":{\"id\":\"resp_fail_1\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_fail_1\",\"status\":\"failed\",\"error\":{\"code\":\"server_error\",\"message\":\"The model produced invalid content\"}}}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := NewResponsesClient(OpenAIConfig{
		BaseURL: srv.URL,
		Model:   "gpt-5-mini",
		APIKey:  "test-key",
	}, http.DefaultClient)

	_, err := client.CreateResponseStream(context.Background(), CreateResponseRequest{
		Input: "test",
	}, nil)
	if err == nil {
		t.Fatal("expected stream failure error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `response_id="resp_fail_1"`) {
		t.Fatalf("expected error includes response_id, got %q", msg)
	}
	if !strings.Contains(msg, `code="server_error"`) {
		t.Fatalf("expected error includes code, got %q", msg)
	}
	if !strings.Contains(msg, "invalid content") {
		t.Fatalf("expected error includes provider message, got %q", msg)
	}
}
