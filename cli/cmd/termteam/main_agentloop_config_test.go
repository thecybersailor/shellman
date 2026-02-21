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

	"termteam/cli/internal/agentloop"
	"termteam/cli/internal/config"
	"termteam/cli/internal/helperconfig"
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
