package application

import "context"

func (a *Application) setRunHook(fn func(context.Context) error) {
	if a == nil {
		return
	}
	a.runFn = fn
}

func (a *Application) setShutdownHook(fn func(context.Context) error) {
	if a == nil {
		return
	}
	a.shutdownFn = fn
}
