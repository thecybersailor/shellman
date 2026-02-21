package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"shellman/cli/internal/agentloop"
	"shellman/cli/internal/config"
	"shellman/cli/internal/helperconfig"
)

type fakeAgentHelperConfigStore struct {
	cfg helperconfig.OpenAIConfig
	err error
}

func (f *fakeAgentHelperConfigStore) LoadOpenAI() (helperconfig.OpenAIConfig, error) {
	if f.err != nil {
		return helperconfig.OpenAIConfig{}, f.err
	}
	return f.cfg, nil
}

func (f *fakeAgentHelperConfigStore) SaveOpenAI(cfg helperconfig.OpenAIConfig) error {
	f.cfg = cfg
	return nil
}

func TestResolveAgentOpenAIConfig_PrefersHelperConfig(t *testing.T) {
	cfg := config.Config{
		OpenAIEndpoint: "https://env.example/v1",
		OpenAIModel:    "env-model",
		OpenAIAPIKey:   "env-key",
	}
	helperStore := &fakeAgentHelperConfigStore{
		cfg: helperconfig.OpenAIConfig{
			Endpoint: "https://helper.example/v1",
			Model:    "helper-model",
			APIKey:   "helper-key",
		},
	}

	endpoint, model, apiKey := resolveAgentOpenAIConfig(cfg, helperStore)
	if endpoint != "https://helper.example/v1" || model != "helper-model" || apiKey != "helper-key" {
		t.Fatalf("expected helper config to be preferred, got endpoint=%q model=%q apiKey=%q", endpoint, model, apiKey)
	}
}

func TestResolveAgentOpenAIConfig_FallsBackToEnvWhenHelperIncomplete(t *testing.T) {
	cfg := config.Config{
		OpenAIEndpoint: "https://env.example/v1",
		OpenAIModel:    "env-model",
		OpenAIAPIKey:   "env-key",
	}
	helperStore := &fakeAgentHelperConfigStore{
		cfg: helperconfig.OpenAIConfig{
			Endpoint: "https://helper.example/v1",
			Model:    "helper-model",
			APIKey:   "",
		},
	}

	endpoint, model, apiKey := resolveAgentOpenAIConfig(cfg, helperStore)
	if endpoint != "https://env.example/v1" || model != "env-model" || apiKey != "env-key" {
		t.Fatalf("expected env fallback, got endpoint=%q model=%q apiKey=%q", endpoint, model, apiKey)
	}
}

func TestBuildAgentLoopRunner_UsesHelperConfig(t *testing.T) {
	cfg := config.Config{
		OpenAIEndpoint: "https://env.example/v1",
		OpenAIModel:    "env-model",
		OpenAIAPIKey:   "env-key",
	}
	helperStore := &fakeAgentHelperConfigStore{
		cfg: helperconfig.OpenAIConfig{
			Endpoint: "https://helper.example/v1",
			Model:    "helper-model",
			APIKey:   "helper-key",
		},
	}

	dummyExec := func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
		return 200, map[string]string{}, `{"ok":true}`, nil
	}

	runner, endpoint, model := buildAgentLoopRunner(cfg, helperStore, dummyExec)
	if runner == nil {
		t.Fatal("expected non-nil agent loop runner")
	}
	if endpoint != "https://helper.example/v1" || model != "helper-model" {
		t.Fatalf("expected helper endpoint/model, got endpoint=%q model=%q", endpoint, model)
	}

}

func TestBuildAgentLoopRunner_RegistersExpectedTaskTools(t *testing.T) {
	var firstReq map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("expected /responses path, got %s", r.URL.Path)
		}
		if firstReq == nil {
			if err := json.NewDecoder(r.Body).Decode(&firstReq); err != nil {
				t.Fatalf("decode request failed: %v", err)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "resp_1",
			"output": []map[string]any{
				{
					"type": "message",
					"content": []map[string]any{
						{"type": "output_text", "text": "ok"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := config.Config{}
	helperStore := &fakeAgentHelperConfigStore{
		cfg: helperconfig.OpenAIConfig{
			Endpoint: srv.URL,
			Model:    "gpt-5-mini",
			APIKey:   "test-key",
		},
	}
	httpExec := func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
		return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true}`, nil
	}
	runner, _, _ := buildAgentLoopRunner(cfg, helperStore, httpExec)
	if runner == nil {
		t.Fatal("expected non-nil agent loop runner")
	}
	out, err := runner.Run(context.Background(), "reply ok")
	if err != nil {
		t.Fatalf("runner run failed: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	toolsAny, ok := firstReq["tools"].([]any)
	if !ok {
		t.Fatalf("expected tools in first request, got %#v", firstReq["tools"])
	}
	got := make([]string, 0, len(toolsAny))
	for _, item := range toolsAny {
		spec, _ := item.(map[string]any)
		got = append(got, strings.TrimSpace(anyToTestString(spec["name"])))
	}
	want := agentloop.TaskActionToolContractNames()
	slices.Sort(got)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected registered task tools: got=%#v want=%#v", got, want)
	}
}

func anyToTestString(v any) string {
	s, _ := v.(string)
	return s
}

func TestBuildAgentLoopRunner_WriteStdinIncludesPostTerminalScreenState(t *testing.T) {
	var requestBodies []map[string]any
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
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
						"name":      "write_stdin",
						"arguments": `{"input":"codex\n"}`,
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
						{"type": "output_text", "text": "ok"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := config.Config{}
	helperStore := &fakeAgentHelperConfigStore{
		cfg: helperconfig.OpenAIConfig{
			Endpoint: srv.URL,
			Model:    "gpt-5-mini",
			APIKey:   "test-key",
		},
	}

	paneGetCount := 0
	httpExec := func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
		switch {
		case method == http.MethodGet && path == "/api/v1/tasks/t1/pane":
			paneGetCount++
			snapshot := "shell prompt"
			currentCommand := "zsh"
			if paneGetCount >= 2 {
				snapshot = "OpenAI Codex (v0.104.0)"
				currentCommand = "codex (/Users/wanglei/.)"
			}
			return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true,"data":{"pane_target":"botworks:8.0","current_command":"` + currentCommand + `","snapshot":{"output":"` + snapshot + `","cursor":{"x":1,"y":2}}}}`, nil
		case method == http.MethodPost && path == "/api/v1/tasks/t1/messages":
			return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true,"data":{"task_id":"t1","delivery_status":"sent"}}`, nil
		default:
			return 404, map[string]string{"Content-Type": "application/json"}, `{"ok":false}`, nil
		}
	}

	runner, _, _ := buildAgentLoopRunner(cfg, helperStore, httpExec)
	if runner == nil {
		t.Fatal("expected non-nil agent loop runner")
	}
	ctx := agentloop.WithTaskScope(context.Background(), agentloop.TaskScope{TaskID: "t1", ProjectID: "p1"})
	if _, err := runner.Run(ctx, "start codex"); err != nil {
		t.Fatalf("runner run failed: %v", err)
	}
	if len(requestBodies) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requestBodies))
	}
	gotOut := extractLastFunctionCallOutput(t, requestBodies[1])
	screen, ok := gotOut["data"].(map[string]any)["post_terminal_screen_state"].(map[string]any)
	if !ok {
		t.Fatalf("expected post_terminal_screen_state in tool output, got %#v", gotOut)
	}
	if !strings.Contains(strings.TrimSpace(anyToTestString(screen["viewport_text"])), "OpenAI Codex") {
		t.Fatalf("expected viewport_text contains codex screen, got %#v", screen["viewport_text"])
	}
}

func TestBuildAgentLoopRunner_TaskInputPromptIncludesPostTerminalScreenState(t *testing.T) {
	var requestBodies []map[string]any
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
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
						"name":      "task.input_prompt",
						"arguments": `{"prompt":"请继续执行"}`,
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
						{"type": "output_text", "text": "ok"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := config.Config{}
	helperStore := &fakeAgentHelperConfigStore{
		cfg: helperconfig.OpenAIConfig{
			Endpoint: srv.URL,
			Model:    "gpt-5-mini",
			APIKey:   "test-key",
		},
	}

	paneGetCount := 0
	httpExec := func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
		switch {
		case method == http.MethodGet && path == "/api/v1/tasks/t1/pane":
			paneGetCount++
			snapshot := "OpenAI Codex (v0.104.0)"
			if paneGetCount >= 2 {
				snapshot = "OpenAI Codex (v0.104.0)\\n任务已接收"
			}
			return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true,"data":{"pane_target":"botworks:8.0","current_command":"codex (/Users/wanglei/.)","snapshot":{"output":"` + snapshot + `","cursor":{"x":1,"y":2}}}}`, nil
		case method == http.MethodPost && path == "/api/v1/tasks/t1/messages":
			return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true,"data":{"task_id":"t1","delivery_status":"sent"}}`, nil
		default:
			return 404, map[string]string{"Content-Type": "application/json"}, `{"ok":false}`, nil
		}
	}

	runner, _, _ := buildAgentLoopRunner(cfg, helperStore, httpExec)
	if runner == nil {
		t.Fatal("expected non-nil agent loop runner")
	}
	ctx := agentloop.WithTaskScope(context.Background(), agentloop.TaskScope{TaskID: "t1", ProjectID: "p1"})
	if _, err := runner.Run(ctx, "start codex"); err != nil {
		t.Fatalf("runner run failed: %v", err)
	}
	if len(requestBodies) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requestBodies))
	}
	gotOut := extractLastFunctionCallOutput(t, requestBodies[1])
	screen, ok := gotOut["data"].(map[string]any)["post_terminal_screen_state"].(map[string]any)
	if !ok {
		t.Fatalf("expected post_terminal_screen_state in tool output, got %#v", gotOut)
	}
	if !strings.Contains(strings.TrimSpace(anyToTestString(screen["viewport_text"])), "任务已接收") {
		t.Fatalf("expected viewport_text contains updated screen, got %#v", screen["viewport_text"])
	}
	if timeout, _ := screen["timeout_ms"].(float64); timeout != 3000 {
		t.Fatalf("expected timeout_ms=3000, got %#v", screen["timeout_ms"])
	}
}

func TestBuildAgentLoopRunner_TaskInputPromptSplitsPromptAndEnterWrites(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "resp_1",
				"output": []map[string]any{
					{
						"type":      "function_call",
						"id":        "fc_1",
						"call_id":   "call_1",
						"name":      "task.input_prompt",
						"arguments": `{"prompt":"请继续执行"}`,
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
						{"type": "output_text", "text": "ok"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := config.Config{}
	helperStore := &fakeAgentHelperConfigStore{
		cfg: helperconfig.OpenAIConfig{
			Endpoint: srv.URL,
			Model:    "gpt-5-mini",
			APIKey:   "test-key",
		},
	}

	paneGetCount := 0
	var ttyInputs []string
	httpExec := func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
		switch {
		case method == http.MethodGet && path == "/api/v1/tasks/t1/pane":
			paneGetCount++
			snapshot := "OpenAI Codex (v0.104.0)"
			if paneGetCount >= 2 {
				snapshot = "OpenAI Codex (v0.104.0)\\n任务已接收"
			}
			return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true,"data":{"pane_target":"botworks:8.0","current_command":"codex (/Users/wanglei/.)","snapshot":{"output":"` + snapshot + `","cursor":{"x":1,"y":2}}}}`, nil
		case method == http.MethodPost && path == "/api/v1/tasks/t1/messages":
			var payload map[string]any
			if err := json.Unmarshal([]byte(body), &payload); err != nil {
				t.Fatalf("decode body failed: %v", err)
			}
			if strings.TrimSpace(anyToTestString(payload["source"])) == "tty_write_stdin" {
				ttyInputs = append(ttyInputs, anyToTestString(payload["input"]))
			}
			return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true,"data":{"task_id":"t1","delivery_status":"sent"}}`, nil
		default:
			return 404, map[string]string{"Content-Type": "application/json"}, `{"ok":false}`, nil
		}
	}

	runner, _, _ := buildAgentLoopRunner(cfg, helperStore, httpExec)
	if runner == nil {
		t.Fatal("expected non-nil agent loop runner")
	}
	ctx := agentloop.WithTaskScope(context.Background(), agentloop.TaskScope{TaskID: "t1", ProjectID: "p1"})
	if _, err := runner.Run(ctx, "start codex"); err != nil {
		t.Fatalf("runner run failed: %v", err)
	}
	want := []string{"请继续执行", "\r"}
	if !reflect.DeepEqual(ttyInputs, want) {
		t.Fatalf("expected split tty_write_stdin inputs %#v, got %#v", want, ttyInputs)
	}
}

func TestBuildAgentLoopRunner_ExecCommandIncludesPostTerminalScreenStateWhenDeltaEmpty(t *testing.T) {
	var requestBodies []map[string]any
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request failed: %v", err)
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
						"name":      "exec_command",
						"arguments": `{"command":"sleep 0.3","max_output_tokens":128}`,
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
						{"type": "output_text", "text": "ok"},
					},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := config.Config{}
	helperStore := &fakeAgentHelperConfigStore{
		cfg: helperconfig.OpenAIConfig{
			Endpoint: srv.URL,
			Model:    "gpt-5-mini",
			APIKey:   "test-key",
		},
	}
	httpExec := func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
		switch {
		case method == http.MethodGet && path == "/api/v1/tasks/t1/pane":
			return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true,"data":{"pane_target":"botworks:8.0","current_command":"codex (/Users/wanglei/.)","snapshot":{"output":"OpenAI Codex (v0.104.0)"}}}`, nil
		case method == http.MethodPost && path == "/api/v1/tasks/t1/messages":
			return 200, map[string]string{"Content-Type": "application/json"}, `{"ok":true,"data":{"task_id":"t1","delivery_status":"sent"}}`, nil
		default:
			return 404, map[string]string{"Content-Type": "application/json"}, `{"ok":false}`, nil
		}
	}

	runner, _, _ := buildAgentLoopRunner(cfg, helperStore, httpExec)
	if runner == nil {
		t.Fatal("expected non-nil agent loop runner")
	}
	ctx := agentloop.WithTaskScope(context.Background(), agentloop.TaskScope{TaskID: "t1", ProjectID: "p1"})
	start := time.Now()
	if _, err := runner.Run(ctx, "sleep"); err != nil {
		t.Fatalf("runner run failed: %v", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("unexpectedly slow run: %v", time.Since(start))
	}
	if len(requestBodies) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requestBodies))
	}
	gotOut := extractLastFunctionCallOutput(t, requestBodies[1])
	data, ok := gotOut["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %#v", gotOut)
	}
	if strings.TrimSpace(anyToTestString(data["output"])) != "" {
		t.Fatalf("expected empty delta output for sleep 0.3, got %#v", data["output"])
	}
	screen, ok := data["post_terminal_screen_state"].(map[string]any)
	if !ok {
		t.Fatalf("expected post_terminal_screen_state in tool output, got %#v", gotOut)
	}
	if !strings.Contains(strings.TrimSpace(anyToTestString(screen["viewport_text"])), "OpenAI Codex") {
		t.Fatalf("expected viewport_text contains codex screen, got %#v", screen["viewport_text"])
	}
}

func extractLastFunctionCallOutput(t *testing.T, req map[string]any) map[string]any {
	t.Helper()
	input, ok := req["input"].([]any)
	if !ok || len(input) == 0 {
		t.Fatalf("expected request input items, got %#v", req["input"])
	}
	last := input[len(input)-1]
	item, ok := last.(map[string]any)
	if !ok {
		t.Fatalf("expected input item object, got %#v", last)
	}
	if strings.TrimSpace(anyToTestString(item["type"])) != "function_call_output" {
		t.Fatalf("expected last item type=function_call_output, got %#v", item)
	}
	raw := strings.TrimSpace(anyToTestString(item["output"]))
	if raw == "" {
		t.Fatalf("expected non-empty function_call_output")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("decode function_call_output failed: %v raw=%q", err, raw)
	}
	return parsed
}
