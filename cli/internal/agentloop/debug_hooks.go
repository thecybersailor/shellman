package agentloop

import "context"

type ResponsesDebugHooks struct {
	OnRequestRaw       func(raw string)
	OnResponseRaw      func(raw string)
	OnStreamEventRaw   func(raw string)
}

type responsesDebugHooksContextKey struct{}

func WithResponsesDebugHooks(ctx context.Context, hooks ResponsesDebugHooks) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, responsesDebugHooksContextKey{}, hooks)
}

func responsesDebugHooksFromContext(ctx context.Context) (ResponsesDebugHooks, bool) {
	if ctx == nil {
		return ResponsesDebugHooks{}, false
	}
	hooks, ok := ctx.Value(responsesDebugHooksContextKey{}).(ResponsesDebugHooks)
	if !ok {
		return ResponsesDebugHooks{}, false
	}
	return hooks, true
}

func emitResponsesRequestRaw(ctx context.Context, raw string) {
	hooks, ok := responsesDebugHooksFromContext(ctx)
	if !ok || hooks.OnRequestRaw == nil {
		return
	}
	hooks.OnRequestRaw(raw)
}

func emitResponsesResponseRaw(ctx context.Context, raw string) {
	hooks, ok := responsesDebugHooksFromContext(ctx)
	if !ok || hooks.OnResponseRaw == nil {
		return
	}
	hooks.OnResponseRaw(raw)
}

func emitResponsesStreamEventRaw(ctx context.Context, raw string) {
	hooks, ok := responsesDebugHooksFromContext(ctx)
	if !ok || hooks.OnStreamEventRaw == nil {
		return
	}
	hooks.OnStreamEventRaw(raw)
}
