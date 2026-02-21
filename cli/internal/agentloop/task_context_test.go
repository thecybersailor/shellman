package agentloop

import (
	"context"
	"testing"
)

func TestWithTaskScopeAndFromContext(t *testing.T) {
	base := context.Background()
	scoped := WithTaskScope(base, TaskScope{
		TaskID:              "t1",
		ProjectID:           "p1",
		Source:              "user",
		ResponsesStore:      true,
		DisableStoreContext: true,
	})
	got, ok := TaskScopeFromContext(scoped)
	if !ok {
		t.Fatal("expected task scope exists in context")
	}
	if got.TaskID != "t1" {
		t.Fatalf("expected task_id=t1, got %q", got.TaskID)
	}
	if got.ProjectID != "p1" {
		t.Fatalf("expected project_id=p1, got %q", got.ProjectID)
	}
	if got.Source != "user" {
		t.Fatalf("expected source=user, got %q", got.Source)
	}
	if !got.ResponsesStore {
		t.Fatalf("expected responses_store=true, got false")
	}
	if !got.DisableStoreContext {
		t.Fatalf("expected disable_store_context=true, got false")
	}
}

func TestTaskScopeFromContext_Missing(t *testing.T) {
	_, ok := TaskScopeFromContext(context.Background())
	if ok {
		t.Fatal("expected missing task scope")
	}
}
