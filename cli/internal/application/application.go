package application

import (
	"context"
	"fmt"
	"strings"
)

const bootstrapPathName = "application.start.v1"

type Application struct {
	localAPIBaseURL string
	dbDSN           string
	bootstrapPath   string
	runFn           func(context.Context) error
	shutdownFn      func(context.Context) error
}

func StartApplication(_ context.Context, opts StartOptions) (*Application, error) {
	mode := strings.TrimSpace(opts.Mode)
	if mode == "" {
		mode = "local"
	}
	if mode != "local" && mode != "turn" {
		return nil, fmt.Errorf("unsupported mode: %s", mode)
	}

	host := strings.TrimSpace(opts.LocalHost)
	if host == "" {
		host = "127.0.0.1"
	}
	port := opts.LocalPort
	if port <= 0 {
		port = 4621
	}
	localBaseURL := fmt.Sprintf("http://%s:%d", host, port)

	app := &Application{
		localAPIBaseURL: localBaseURL,
		dbDSN:           strings.TrimSpace(opts.DBDSN),
		bootstrapPath:   bootstrapPathName,
		runFn: func(context.Context) error {
			return nil
		},
		shutdownFn: func(context.Context) error {
			return nil
		},
	}
	return app, nil
}

func (a *Application) LocalAPIBaseURL() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.localAPIBaseURL)
}

func (a *Application) DBDSN() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.dbDSN)
}

func (a *Application) BootstrapPath() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.bootstrapPath)
}

func (a *Application) Run(ctx context.Context) error {
	if a == nil || a.runFn == nil {
		return nil
	}
	return a.runFn(ctx)
}

func (a *Application) Shutdown(ctx context.Context) error {
	if a == nil || a.shutdownFn == nil {
		return nil
	}
	return a.shutdownFn(ctx)
}
