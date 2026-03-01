package agentloopadapter

import "github.com/flaboy/agentloop"

func RegisterLoopRunnerMiddleware(runner *agentloop.LoopRunner) {
	if runner == nil {
		return
	}
	runner.RegisterHook(agentloop.HookPointModelCall, func(ctx *agentloop.HookContext, next agentloop.NextFunc) error {
		if ctx != nil {
			if names, ok := AllowedToolNamesFromContext(ctx.Ctx); ok {
				ctx.SetAllowedToolNames(names)
			}
			if scope, ok := TaskScopeFromContext(ctx.Ctx); ok {
				if !scope.DisableStoreContext && ctx.Request != nil {
					store := scope.ResponsesStore
					ctx.Request.Store = &store
				}
			}
		}
		return next()
	})
}
