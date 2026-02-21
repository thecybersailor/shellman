package agentloop

import (
	"context"
	"strings"
)

type TaskScope struct {
	TaskID              string
	ProjectID           string
	Source              string
	ResponsesStore      bool
	DisableStoreContext bool
}

type taskScopeContextKey struct{}

func WithTaskScope(ctx context.Context, scope TaskScope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	scope.TaskID = strings.TrimSpace(scope.TaskID)
	scope.ProjectID = strings.TrimSpace(scope.ProjectID)
	scope.Source = strings.TrimSpace(scope.Source)
	return context.WithValue(ctx, taskScopeContextKey{}, scope)
}

func TaskScopeFromContext(ctx context.Context) (TaskScope, bool) {
	if ctx == nil {
		return TaskScope{}, false
	}
	v := ctx.Value(taskScopeContextKey{})
	scope, ok := v.(TaskScope)
	if !ok {
		return TaskScope{}, false
	}
	return scope, true
}
