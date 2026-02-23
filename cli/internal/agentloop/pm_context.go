package agentloop

import (
	"context"
	"strings"
)

type PMScope struct {
	SessionID string
	ProjectID string
	Source    string
}

type pmScopeContextKey struct{}

func WithPMScope(ctx context.Context, scope PMScope) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	scope.SessionID = strings.TrimSpace(scope.SessionID)
	scope.ProjectID = strings.TrimSpace(scope.ProjectID)
	scope.Source = strings.TrimSpace(scope.Source)
	return context.WithValue(ctx, pmScopeContextKey{}, scope)
}

func PMScopeFromContext(ctx context.Context) (PMScope, bool) {
	if ctx == nil {
		return PMScope{}, false
	}
	v := ctx.Value(pmScopeContextKey{})
	scope, ok := v.(PMScope)
	if !ok {
		return PMScope{}, false
	}
	return scope, true
}
