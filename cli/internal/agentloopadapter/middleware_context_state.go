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
		}
		return next()
	})
}
