package agentloopadapter

import (
	"context"
	"testing"

	"github.com/flaboy/agentloop"
	core "github.com/flaboy/agentloop/core"
)

type middlewareTestClient struct {
	requests []core.CreateResponseRequest
}

func (c *middlewareTestClient) CreateResponse(_ context.Context, req core.CreateResponseRequest) (*core.CreateResponseResult, error) {
	c.requests = append(c.requests, req)
	return &core.CreateResponseResult{FinalText: "ok"}, nil
}

type middlewareEchoTool struct{}

type middlewareOtherTool struct{}

func (middlewareEchoTool) Name() string { return "echo" }
func (middlewareOtherTool) Name() string { return "other" }

func (middlewareEchoTool) Spec() core.ResponseToolSpec {
	return core.ResponseToolSpec{Type: "function", Name: "echo"}
}

func (middlewareOtherTool) Spec() core.ResponseToolSpec {
	return core.ResponseToolSpec{Type: "function", Name: "other"}
}

func (middlewareEchoTool) Execute(_ context.Context, _ struct{}, _ string, _ string) (string, *core.ToolError) {
	return "ok", nil
}

func (middlewareOtherTool) Execute(_ context.Context, _ struct{}, _ string, _ string) (string, *core.ToolError) {
	return "ok", nil
}

func TestRegisterLoopRunnerMiddleware_InjectsAllowedToolNames(t *testing.T) {
	client := &middlewareTestClient{}
	registry := core.NewToolRegistry[struct{}]()
	if err := registry.Register(middlewareEchoTool{}); err != nil {
		t.Fatalf("register echo failed: %v", err)
	}
	if err := registry.Register(middlewareOtherTool{}); err != nil {
		t.Fatalf("register other failed: %v", err)
	}
	runner := agentloop.NewLoopRunner(client, registry, agentloop.LoopRunnerOptions{MaxIterations: 1})
	RegisterLoopRunnerMiddleware(runner)

	runCtx := WithAllowedToolNames(context.Background(), []string{"echo"})
	runCtx = WithTaskScope(runCtx, TaskScope{
		TaskID:              "t1",
		ProjectID:           "p1",
		Source:              "unit",
		ResponsesStore:      true,
		DisableStoreContext: false,
	})
	out, err := runner.Run(runCtx, "hello")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected one request")
	}
	tools := client.requests[0].Tools
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool spec, got %d", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Fatalf("unexpected tool name: %q", tools[0].Name)
	}
	if client.requests[0].Store == nil || !*client.requests[0].Store {
		t.Fatalf("expected request store=true, got %#v", client.requests[0].Store)
	}
}

func TestRegisterLoopRunnerMiddleware_DisableStoreContextSkipsStoreInjection(t *testing.T) {
	client := &middlewareTestClient{}
	registry := core.NewToolRegistry[struct{}]()
	if err := registry.Register(middlewareEchoTool{}); err != nil {
		t.Fatalf("register echo failed: %v", err)
	}
	runner := agentloop.NewLoopRunner(client, registry, agentloop.LoopRunnerOptions{MaxIterations: 1})
	RegisterLoopRunnerMiddleware(runner)

	runCtx := WithTaskScope(context.Background(), TaskScope{
		TaskID:              "t1",
		ProjectID:           "p1",
		Source:              "unit",
		ResponsesStore:      true,
		DisableStoreContext: true,
	})
	_, err := runner.Run(runCtx, "hello")
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected one request")
	}
	if client.requests[0].Store != nil {
		t.Fatalf("expected request store to stay nil when disable_store_context=true, got %#v", client.requests[0].Store)
	}
}
