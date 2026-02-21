package application

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"shellman/cli/internal/appserver"
	"shellman/cli/internal/fsbrowser"
	"shellman/cli/internal/global"
	"shellman/cli/internal/helperconfig"
	"shellman/cli/internal/historydb"
	"shellman/cli/internal/lifecycle"
	"shellman/cli/internal/localapi"
	"shellman/cli/internal/projectstate"
	"shellman/cli/internal/systempicker"
	"shellman/cli/internal/tmux"
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
	if customURL := strings.TrimSpace(opts.Hooks.LocalAPIURL); customURL != "" {
		localBaseURL = customURL
	}
	bootstrapPath := bootstrapPathName
	if customTag := strings.TrimSpace(opts.Hooks.BootstrapTag); customTag != "" {
		bootstrapPath = customTag
	}

	app := &Application{
		localAPIBaseURL: localBaseURL,
		dbDSN:           strings.TrimSpace(opts.DBDSN),
		bootstrapPath:   bootstrapPath,
		runFn: func(context.Context) error {
			return nil
		},
		shutdownFn: func(context.Context) error {
			return nil
		},
	}
	if opts.Hooks.Run != nil {
		app.runFn = opts.Hooks.Run
	}
	if opts.Hooks.Shutdown != nil {
		app.shutdownFn = opts.Hooks.Shutdown
	}
	if opts.Hooks.Run == nil && mode == "local" {
		if err := bootstrapLocalRuntime(app, opts); err != nil {
			return nil, err
		}
	}
	return app, nil
}

func bootstrapLocalRuntime(app *Application, opts StartOptions) error {
	configDir := strings.TrimSpace(opts.ConfigDir)
	if configDir == "" {
		return fmt.Errorf("config dir is required")
	}
	dsn := strings.TrimSpace(opts.DBDSN)
	if dsn == "" {
		dsn = filepath.Join(configDir, "shellman.db")
	}
	if err := projectstate.InitGlobalDBWithDSN(dsn); err != nil {
		return err
	}
	gdb, err := projectstate.GlobalDBGORM()
	if err != nil {
		return err
	}
	cfgStore := global.NewConfigStore(configDir)
	appProgramsStore := global.NewAppProgramsStore(configDir)
	projectsStore := global.NewProjectsStore(configDir)
	helperCfgStore, err := helperconfig.NewStore(gdb, filepath.Join(configDir, ".shellman-helper-openai-secret"))
	if err != nil {
		return err
	}
	historyStore, err := historydb.NewStore(gdb)
	if err != nil {
		_ = helperCfgStore.Close()
		return err
	}
	tmuxAdapter := tmux.NewAdapterWithSocket(&tmux.RealExec{}, strings.TrimSpace(opts.TmuxSocket))
	localDeps := localapi.Deps{
		ConfigStore:       cfgStore,
		AppProgramsStore:  appProgramsStore,
		HelperConfigStore: helperCfgStore,
		ProjectsStore:     projectsStore,
		PaneService:       tmuxAdapter,
		TaskPromptSender:  tmuxAdapter,
		PickDirectory:     systempicker.PickDirectory,
		FSBrowser:         fsbrowser.NewService(),
		DirHistory:        historyStore,
	}
	localServer := localapi.NewServer(localDeps)
	server, err := appserver.NewServer(appserver.Deps{
		LocalAPIHandle: localServer.Handler(),
		WebUI: appserver.WebUIConfig{
			Mode:        strings.TrimSpace(opts.WebUI.Mode),
			DevProxyURL: strings.TrimSpace(opts.WebUI.DevProxyURL),
			DistDir:     strings.TrimSpace(opts.WebUI.DistDir),
		},
	})
	if err != nil {
		_ = historyStore.Close()
		_ = helperCfgStore.Close()
		return err
	}

	host := strings.TrimSpace(opts.LocalHost)
	if host == "" {
		host = "127.0.0.1"
	}
	port := opts.LocalPort
	if port <= 0 {
		port = 4621
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: server.Handler(),
	}
	mgr := lifecycle.NewManager()
	mgr.AddRun("http-server", func(runCtx context.Context) error {
		go func() {
			<-runCtx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = httpServer.Shutdown(shutdownCtx)
		}()
		err := httpServer.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	mgr.AddShutdown("http-server-shutdown", func(context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := httpServer.Shutdown(shutdownCtx)
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	})
	mgr.AddShutdown("close-helper-config", func(context.Context) error {
		return helperCfgStore.Close()
	})
	mgr.AddShutdown("close-history-store", func(context.Context) error {
		return historyStore.Close()
	})
	app.localAPIBaseURL = fmt.Sprintf("http://%s", addr)
	app.dbDSN = dsn
	app.runFn = func(ctx context.Context) error {
		return mgr.StartAndWait(ctx)
	}
	app.shutdownFn = func(context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
	return nil
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
