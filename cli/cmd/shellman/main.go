package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"shellman/cli/internal/agentloop"
	"shellman/cli/internal/application"
	"shellman/cli/internal/appserver"
	"shellman/cli/internal/bridge"
	"shellman/cli/internal/command"
	"shellman/cli/internal/config"
	"shellman/cli/internal/fsbrowser"
	"shellman/cli/internal/global"
	"shellman/cli/internal/helperconfig"
	"shellman/cli/internal/historydb"
	"shellman/cli/internal/lifecycle"
	"shellman/cli/internal/localapi"
	"shellman/cli/internal/logging"
	"shellman/cli/internal/progdetector"
	_ "shellman/cli/internal/progdetector/builtin"
	"shellman/cli/internal/projectstate"
	"shellman/cli/internal/protocol"
	"shellman/cli/internal/systempicker"
	"shellman/cli/internal/tmux"
	"shellman/cli/internal/turn"
)

var version = "dev"
var buildTime = "unknown"

var streamPumpInterval = 200 * time.Millisecond
var statusPumpInterval = 500 * time.Millisecond
var statusTransitionDelay = 3 * time.Second
var statusInputIgnoreWindow = 2 * time.Second
var traceStreamEnabled = false
var streamHistoryLines = 2000
var runtimeTmuxSocket = ""
var startApplication = application.StartApplication

type registerClient interface {
	Register() (turn.RegisterResponse, error)
}

type wsDialer interface {
	Dial(ctx context.Context, url string) (turn.Socket, error)
}

type gatewayHTTPExecutor func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error)
type paneAutoCompletionExecutor func(paneTarget string, observedLastActiveAt time.Time) (localapi.AutoCompleteByPaneResult, error)

type gatewayHTTPExecutorRef struct {
	mu   sync.RWMutex
	exec gatewayHTTPExecutor
}

func newGatewayHTTPExecutorRef(exec gatewayHTTPExecutor) *gatewayHTTPExecutorRef {
	return &gatewayHTTPExecutorRef{exec: exec}
}

func (r *gatewayHTTPExecutorRef) Set(exec gatewayHTTPExecutor) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.exec = exec
	r.mu.Unlock()
}

func (r *gatewayHTTPExecutorRef) Exec(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
	if r == nil {
		return 500, map[string]string{}, "", errors.New("gateway http executor is unavailable")
	}
	r.mu.RLock()
	exec := r.exec
	r.mu.RUnlock()
	if exec == nil {
		return 500, map[string]string{}, "", errors.New("gateway http executor is unavailable")
	}
	return exec(method, path, headers, body)
}

func main() {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := command.BuildApp(command.Deps{
		LoadConfig: config.LoadConfig,
		RunLocalMode: func(ctx context.Context, cfg config.Config) error {
			applyRuntimeConfig(cfg)
			return runServe(
				ctx,
				cfg,
				func(runCtx context.Context) error {
					return runLocal(runCtx, os.Stdout, cfg)
				},
				func(runCtx context.Context) error {
					return run(
						runCtx,
						os.Stdout,
						os.Stderr,
						turn.NewRegisterClient(cfg.WorkerBaseURL),
						turn.RealDialer{},
						tmux.NewAdapterWithSocket(&tmux.RealExec{}, cfg.TmuxSocket),
					)
				},
				newRuntimeLogger(os.Stderr).With("module", "serve"),
			)
		},
		RunTurnMode: func(ctx context.Context, cfg config.Config) error {
			applyRuntimeConfig(cfg)
			return run(
				ctx,
				os.Stdout,
				os.Stderr,
				turn.NewRegisterClient(cfg.WorkerBaseURL),
				turn.RealDialer{},
				tmux.NewAdapterWithSocket(&tmux.RealExec{}, cfg.TmuxSocket),
			)
		},
		RunMigrateUp: runMigrateUp,
	})

	if err := app.RunContext(rootCtx, os.Args); err != nil {
		logging.NewLogger(logging.Options{Level: "error", Writer: os.Stderr, Component: "shellman"}).Error("shellman failed", "err", err)
		os.Exit(1)
	}
}

func applyRuntimeConfig(cfg config.Config) {
	traceStreamEnabled = cfg.TraceStream
	streamHistoryLines = cfg.HistoryLines
	runtimeTmuxSocket = strings.TrimSpace(cfg.TmuxSocket)
}

func runMigrateUp(_ context.Context, cfg config.Config) error {
	applyRuntimeConfig(cfg)
	configDir, err := global.DefaultConfigDir()
	if err != nil {
		return err
	}
	return projectstate.InitGlobalDB(filepath.Join(configDir, "shellman.db"))
}

func runServe(
	ctx context.Context,
	cfg config.Config,
	runLocalFn func(context.Context) error,
	runTurnFn func(context.Context) error,
	logger *slog.Logger,
) error {
	if runLocalFn == nil {
		return errors.New("local mode runner is not configured")
	}

	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if cfg.TurnEnabled && runTurnFn != nil {
		started := make(chan struct{})
		go func() {
			close(started)
			if err := runTurnFn(runtimeCtx); err != nil && runtimeCtx.Err() == nil && logger != nil {
				logger.Warn("optional turn bridge failed", "err", err)
			}
		}()
		<-started
	}

	return runLocalFn(runtimeCtx)
}

func run(
	ctx context.Context,
	out io.Writer,
	errOut io.Writer,
	register registerClient,
	dialer wsDialer,
	tmuxService bridge.TmuxService,
) error {
	app, err := startApplication(ctx, application.StartOptions{
		Mode: "turn",
		Hooks: application.Hooks{
			Run: func(runCtx context.Context) error {
				return runLegacy(runCtx, out, errOut, register, dialer, tmuxService)
			},
			BootstrapTag: "cmd.main.turn",
		},
	})
	if err != nil {
		return err
	}
	return app.Run(ctx)
}

func runLegacy(
	ctx context.Context,
	out io.Writer,
	errOut io.Writer,
	register registerClient,
	dialer wsDialer,
	tmuxService bridge.TmuxService,
) error {
	_, _ = fmt.Fprintf(out, "shellman %s (built %s)\n", version, buildTime)
	logger := newRuntimeLogger(errOut)
	logger.Debug("runtime config", "trace_stream", traceStreamEnabled)

	resp, err := register.Register()
	if err != nil {
		logger.Error("register failed", "err", err)
		return err
	}

	_, _ = fmt.Fprintf(out, "visit_url=%s\n", resp.VisitURL)
	_, _ = fmt.Fprintf(out, "agent_ws_url=%s\n", resp.AgentWSURL)

	sock, err := dialer.Dial(ctx, resp.AgentWSURL)
	if err != nil {
		return err
	}
	defer func() { _ = sock.Close() }()

	configDir, err := global.DefaultConfigDir()
	if err != nil {
		return err
	}
	if err := projectstate.InitGlobalDB(filepath.Join(configDir, "shellman.db")); err != nil {
		return err
	}
	gatewayLocalServer := buildGatewayLocalAPIServer(configDir, tmuxService, systempicker.PickDirectory)
	httpExec, autoCompleteExec := newGatewayExecutors(gatewayLocalServer, "local-agent-gateway-http")

	wsClient := turn.NewWSClient(sock)
	return runWSRuntime(ctx, wsClient, tmuxService, httpExec, autoCompleteExec, logger)
}

func runWSRuntime(
	ctx context.Context,
	wsClient *turn.WSClient,
	tmuxService bridge.TmuxService,
	httpExec gatewayHTTPExecutor,
	autoCompleteExec paneAutoCompletionExecutor,
	logger *slog.Logger,
) error {
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	handler := bridge.NewHandler(tmuxService)
	handler.SetHTTPExecutor(httpExec)
	inputTracker := newInputActivityTracker()
	registry := NewRegistryActor(logger.With("module", "registry"))
	taskStateActor := NewTaskStateActor()
	if configDir, err := global.DefaultConfigDir(); err == nil {
		projectsStore := global.NewProjectsStore(configDir)
		taskStateActor.SetProjectProvider(func() ([]taskStateProject, error) {
			projects, err := projectsStore.ListProjects()
			if err != nil {
				return nil, err
			}
			out := make([]taskStateProject, 0, len(projects))
			for _, project := range projects {
				out = append(out, taskStateProject{
					ProjectID: project.ProjectID,
					RepoRoot:  project.RepoRoot,
				})
			}
			return out, nil
		})
	}
	taskStateActor.SetEventEmitter(func(ctx context.Context, msg protocol.Message) error {
		raw, err := json.Marshal(msg)
		if err != nil {
			return err
		}
		return wsClient.Send(ctx, string(raw))
	})
	paneBaseline := loadPaneRuntimeBaselineFromDB(logger.With("module", "status_baseline"))
	registry.SetPaneRuntimeBaseline(paneBaseline)
	outputSource := NewControlModeHub(runtimeCtx, runtimeTmuxSocket, logger.With("module", "control_mode_hub"))
	paneInterval := statusPumpInterval
	registry.ConfigureRuntime(runtimeCtx, wsClient, tmuxService, inputTracker, autoCompleteExec, outputSource, paneInterval, taskStateActor)
	bindMessageLoop(wsClient, handler, registry, inputTracker, logger.With("module", "message_loop"))
	go runTaskStateActorLoop(runtimeCtx, taskStateActor, time.Second)
	go runStatusPump(runtimeCtx, wsClient, tmuxService, httpExec, statusPumpInterval, inputTracker, logger.With("module", "status_pump"), paneBaseline)
	return wsClient.Run(runtimeCtx)
}

func bindMessageLoop(
	wsClient *turn.WSClient,
	handler *bridge.Handler,
	registry *RegistryActor,
	inputTracker *inputActivityTracker,
	logger *slog.Logger,
) {
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	wsClient.OnText(func(in string) {
		connID, innerRaw, unwrapErr := protocol.UnwrapMuxEnvelope([]byte(in))
		if unwrapErr != nil {
			connID = "legacy_single"
			innerRaw = []byte(in)
		}

		var msg protocol.Message
		if err := json.Unmarshal(innerRaw, &msg); err != nil {
			logger.Warn("recv invalid ws payload", "err", err, "raw", in)
			return
		}

		var conn *ConnActor
		if registry != nil {
			conn = registry.GetOrCreateConn(connID)
		}
		activePaneTarget := ""
		if conn != nil {
			activePaneTarget = strings.TrimSpace(conn.Selected())
		}
		msg = enrichGatewayHTTPMessage(msg, activePaneTarget)
		payloadTarget := ""
		switch msg.Op {
		case "tmux.select_pane", "term.input":
			var payload struct {
				Target       string `json:"target"`
				Cols         int    `json:"cols"`
				Rows         int    `json:"rows"`
				GapRecover   bool   `json:"gap_recover"`
				HistoryLines int    `json:"history_lines"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				payloadTarget = payload.Target
				if msg.Op == "term.input" {
					var inputPayload struct {
						Target string `json:"target"`
						Text   string `json:"text"`
					}
					if err := json.Unmarshal(msg.Payload, &inputPayload); err == nil {
						logger.Debug("incoming ws message", "op", "term.input", "target", inputPayload.Target, "text_len", len(inputPayload.Text), "text_preview", debugPreview(inputPayload.Text, 120))
					}
				} else {
					logger.Debug(
						"incoming ws message",
						"op",
						msg.Op,
						"target",
						payload.Target,
						"cols",
						payload.Cols,
						"rows",
						payload.Rows,
						"gap_recover",
						payload.GapRecover,
						"history_lines",
						payload.HistoryLines,
					)
				}
			}
		case "term.resize":
			var payload struct {
				Target string `json:"target"`
				Cols   int    `json:"cols"`
				Rows   int    `json:"rows"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				logger.Debug("incoming ws message", "op", "term.resize", "target", payload.Target, "cols", payload.Cols, "rows", payload.Rows)
			}
		case "gateway.http":
			var payload struct {
				Method  string            `json:"method"`
				Path    string            `json:"path"`
				Headers map[string]string `json:"headers"`
			}
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				logger.Debug("incoming ws message", "op", "gateway.http", "method", payload.Method, "path", payload.Path, "active_target", activePaneTarget, "header_active_target", strings.TrimSpace(payload.Headers["X-Shellman-Active-Pane-Target"]))
			}
		}
		out := handler.Handle(msg)
		if out.Error != nil {
			logger.Warn("ws op failed", "op", out.Op, "id", out.ID, "code", out.Error.Code, "msg", out.Error.Message)
		} else if payloadTarget != "" {
			switch msg.Op {
			case "tmux.select_pane":
				if registry != nil {
					var payload struct {
						GapRecover   bool `json:"gap_recover"`
						HistoryLines int  `json:"history_lines"`
					}
					_ = json.Unmarshal(msg.Payload, &payload)
					registry.Subscribe(connID, payloadTarget, paneSubscribeOptions{
						GapRecover:   payload.GapRecover,
						HistoryLines: payload.HistoryLines,
					})
				}
			case "term.input":
				if conn != nil {
					conn.Select(payloadTarget)
				}
				if inputTracker != nil {
					inputTracker.Mark(payloadTarget, time.Now())
				}
			}
		}
		logger.Debug("outgoing ws response", "op", out.Op, "id", out.ID, "has_error", out.Error != nil)
		respRaw, err := json.Marshal(out)
		if err != nil {
			logger.Error("marshal ws response failed", "op", out.Op, "id", out.ID, "err", err)
			return
		}

		sendRaw := respRaw
		wrapped, wrapErr := protocol.WrapMuxEnvelope(connID, respRaw)
		if wrapErr == nil {
			sendRaw = wrapped
		}
		if err := wsClient.Send(context.Background(), string(sendRaw)); err != nil {
			logger.Error("send ws response failed", "err", err)
		}
	})
}

func enrichGatewayHTTPMessage(msg protocol.Message, activePaneTarget string) protocol.Message {
	if msg.Op != "gateway.http" {
		return msg
	}
	var payload struct {
		Method  string            `json:"method"`
		Path    string            `json:"path"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return msg
	}
	if payload.Headers == nil {
		payload.Headers = map[string]string{}
	}
	if strings.TrimSpace(payload.Headers["X-Shellman-Active-Pane-Target"]) == "" && strings.TrimSpace(activePaneTarget) != "" {
		payload.Headers["X-Shellman-Active-Pane-Target"] = strings.TrimSpace(activePaneTarget)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return msg
	}
	msg.Payload = raw
	return msg
}

func runLocal(ctx context.Context, out io.Writer, cfg config.Config) error {
	app, err := startApplication(ctx, application.StartOptions{
		Mode:       "local",
		LocalHost:  cfg.LocalHost,
		LocalPort:  cfg.LocalPort,
		TmuxSocket: cfg.TmuxSocket,
		WebUI: application.WebUIOptions{
			Mode:        cfg.WebUIMode,
			DevProxyURL: cfg.WebUIDevProxyURL,
			DistDir:     cfg.WebUIDistDir,
		},
		Hooks: application.Hooks{
			Run: func(runCtx context.Context) error {
				return runLocalLegacy(runCtx, out, cfg)
			},
			BootstrapTag: "cmd.main.local",
		},
	})
	if err != nil {
		return err
	}
	return app.Run(ctx)
}

func runLocalLegacy(ctx context.Context, out io.Writer, cfg config.Config) error {
	configDir, err := global.DefaultConfigDir()
	if err != nil {
		return err
	}
	cfgStore := global.NewConfigStore(configDir)
	appProgramsStore := global.NewAppProgramsStore(configDir)
	projectsStore := global.NewProjectsStore(configDir)
	if err := projectstate.InitGlobalDB(filepath.Join(configDir, "shellman.db")); err != nil {
		return err
	}
	globalDBGORM, err := projectstate.GlobalDBGORM()
	if err != nil {
		return err
	}
	helperCfgStore, err := helperconfig.NewStore(globalDBGORM, filepath.Join(configDir, ".shellman-helper-openai-secret"))
	if err != nil {
		return err
	}
	tmuxAdapter := tmux.NewAdapterWithSocket(&tmux.RealExec{}, cfg.TmuxSocket)
	historyStore, err := historydb.NewStore(globalDBGORM)
	if err != nil {
		return err
	}
	httpExecRef := newGatewayHTTPExecutorRef(nil)
	agentRunner, agentEndpoint, agentModel := buildAgentLoopRunner(cfg, helperCfgStore, httpExecRef.Exec)
	localDeps := localapi.Deps{
		ConfigStore:         cfgStore,
		AppProgramsStore:    appProgramsStore,
		HelperConfigStore:   helperCfgStore,
		ProjectsStore:       projectsStore,
		PaneService:         tmuxAdapter,
		TaskPromptSender:    tmuxAdapter,
		PickDirectory:       systempicker.PickDirectory,
		FSBrowser:           fsbrowser.NewService(),
		DirHistory:          historyStore,
		AgentLoopRunner:     agentRunner,
		AgentOpenAIEndpoint: agentEndpoint,
		AgentOpenAIModel:    agentModel,
	}
	localServer := localapi.NewServer(localDeps)
	httpExec, autoCompleteExec := newGatewayExecutors(localServer, "local-agent-gateway-http")
	httpExecRef.Set(httpExec)
	server, err := appserver.NewServer(appserver.Deps{
		LocalAPIHandle: localServer.Handler(),
		WebUI: appserver.WebUIConfig{
			Mode:        cfg.WebUIMode,
			DevProxyURL: cfg.WebUIDevProxyURL,
			DistDir:     cfg.WebUIDistDir,
		},
	})
	if err != nil {
		return err
	}
	localServer.SetExternalEventSink(func(topic, projectID, taskID string, payload map[string]any) {
		server.PublishClientEvent("local", topic, projectID, taskID, payload)
	})
	addr := fmt.Sprintf("%s:%d", cfg.LocalHost, cfg.LocalPort)
	_, _ = fmt.Fprintf(out, "shellman local web server listening at http://%s (version=%s built=%s)\n", addr, version, buildTime)

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
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	mgr.AddRun("local-agent-loop", func(runCtx context.Context) error {
		return startLocalAgentLoop(runCtx, cfg.LocalPort, turn.RealDialer{}, tmuxAdapter, httpExecRef.Exec, autoCompleteExec, newRuntimeLogger(os.Stderr).With("module", "local_agent_loop"))
	})
	mgr.AddShutdown("close-helper-config", func(context.Context) error {
		return helperCfgStore.Close()
	})
	mgr.AddShutdown("close-history-store", func(context.Context) error {
		return historyStore.Close()
	})
	mgr.AddShutdown("http-server-shutdown", func(context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := httpServer.Shutdown(shutdownCtx)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	})
	return mgr.StartAndWait(ctx)
}

func buildAgentLoopRunner(cfg config.Config, helperStore localapi.HelperConfigStore, httpExec gatewayHTTPExecutor) (localapi.AgentLoopRunner, string, string) {
	endpoint, model, apiKey := resolveAgentOpenAIConfig(cfg, helperStore)
	if endpoint == "" || model == "" || apiKey == "" {
		return nil, endpoint, model
	}

	registry := agentloop.NewToolRegistry()
	callTaskTool := func(method, path string, payload any) (string, *agentloop.ToolError) {
		bodyText := ""
		headers := map[string]string{"Content-Type": "application/json"}
		if payload != nil {
			raw, err := json.Marshal(payload)
			if err != nil {
				return "", agentloop.NewToolError("TASK_TOOL_INVALID_PAYLOAD", "请求体 JSON 序列化失败，请检查 payload 字段")
			}
			bodyText = string(raw)
		}
		status, _, body, execErr := httpExec(method, path, headers, bodyText)
		if execErr != nil {
			return "", toAgentToolError(execErr, "本地网关请求失败，请检查 localapi/worker 日志")
		}
		if status >= 300 {
			return "", agentloop.NewToolError(
				"TASK_TOOL_HTTP_FAILED",
				fmt.Sprintf("HTTP status=%d path=%s body=%s", status, clipLogText(path, 300), clipLogText(body, 500)),
			)
		}
		return strings.TrimSpace(body), nil
	}

	type taskPaneScreen struct {
		PaneTarget     string
		CurrentCommand string
		Output         string
		HasCursor      bool
		CursorX        int
		CursorY        int
	}

	getTaskPaneScreen := func(taskID string) (taskPaneScreen, error) {
		path := "/api/v1/tasks/" + url.PathEscape(strings.TrimSpace(taskID)) + "/pane"
		body, err := callTaskTool(http.MethodGet, path, nil)
		if err != nil {
			return taskPaneScreen{}, err
		}
		var res struct {
			Data struct {
				PaneTarget     string `json:"pane_target"`
				CurrentCommand string `json:"current_command"`
				Snapshot       struct {
					Output string `json:"output"`
					Cursor *struct {
						X int `json:"x"`
						Y int `json:"y"`
					} `json:"cursor"`
				} `json:"snapshot"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &res); err != nil {
			return taskPaneScreen{}, err
		}
		out := taskPaneScreen{
			PaneTarget:     strings.TrimSpace(res.Data.PaneTarget),
			CurrentCommand: strings.TrimSpace(res.Data.CurrentCommand),
			Output:         res.Data.Snapshot.Output,
		}
		if res.Data.Snapshot.Cursor != nil {
			out.HasCursor = true
			out.CursorX = res.Data.Snapshot.Cursor.X
			out.CursorY = res.Data.Snapshot.Cursor.Y
		}
		return out, nil
	}

	getTaskPaneOutput := func(taskID string) (string, string, string, error) {
		screen, err := getTaskPaneScreen(taskID)
		if err != nil {
			return "", "", "", err
		}
		return screen.PaneTarget, screen.CurrentCommand, screen.Output, nil
	}

	waitTaskPaneStable := func(taskID, baselineOutput string, timeout time.Duration) (taskPaneScreen, bool, error) {
		if timeout <= 0 {
			timeout = 1800 * time.Millisecond
		}
		baselineHash := snapshotHash(baselineOutput)
		lastHash := baselineHash
		stableCount := 0
		lastScreen := taskPaneScreen{}
		hashChanged := false
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			time.Sleep(120 * time.Millisecond)
			screen, err := getTaskPaneScreen(taskID)
			if err != nil {
				return taskPaneScreen{}, false, err
			}
			lastScreen = screen
			currentHash := snapshotHash(screen.Output)
			if currentHash != baselineHash {
				hashChanged = true
			}
			if currentHash == lastHash {
				stableCount++
			} else {
				stableCount = 0
				lastHash = currentHash
			}
			if hashChanged && stableCount >= 1 {
				break
			}
		}
		return lastScreen, hashChanged, nil
	}

	buildPostTerminalScreenState := func(screen taskPaneScreen) map[string]any {
		state := map[string]any{
			"pane_target":     strings.TrimSpace(screen.PaneTarget),
			"current_command": strings.TrimSpace(screen.CurrentCommand),
			"viewport_text":   tailString(screen.Output, 4000),
			"snapshot_hash":   snapshotHash(screen.Output),
		}
		if screen.HasCursor {
			state["cursor"] = map[string]any{
				"x": screen.CursorX,
				"y": screen.CursorY,
			}
		}
		return state
	}

	getTaskTree := func(taskID string) (string, []map[string]any, error) {
		path := "/api/v1/tasks/" + url.PathEscape(strings.TrimSpace(taskID)) + "/pane"
		body, err := callTaskTool(http.MethodGet, path, nil)
		if err != nil {
			return "", nil, err
		}
		var paneRes struct {
			Data struct {
				ProjectID string `json:"project_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &paneRes); err != nil {
			return "", nil, err
		}
		projectID := strings.TrimSpace(paneRes.Data.ProjectID)
		if projectID == "" {
			return "", nil, errors.New("PROJECT_ID_MISSING")
		}
		treeBody, err := callTaskTool(http.MethodGet, "/api/v1/projects/"+url.PathEscape(projectID)+"/tree", nil)
		if err != nil {
			return "", nil, err
		}
		var treeRes struct {
			Data struct {
				Nodes []map[string]any `json:"nodes"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(treeBody), &treeRes); err != nil {
			return "", nil, err
		}
		return projectID, treeRes.Data.Nodes, nil
	}

	if err := registry.Register(&agentloop.TaskCurrentSetFlagTool{
		Exec: func(ctx context.Context, taskID, flag, statusMessage string) (string, *agentloop.ToolError) {
			_ = ctx
			path := "/api/v1/tasks/" + url.PathEscape(strings.TrimSpace(taskID)) + "/messages"
			payload := map[string]any{
				"source":         "task_set_flag",
				"flag":           strings.TrimSpace(flag),
				"status_message": strings.TrimSpace(statusMessage),
			}
			return callTaskTool(http.MethodPost, path, payload)
		},
	}); err != nil {
		return nil, endpoint, model
	}
	if err := registry.Register(&agentloop.WriteStdinTool{
		Exec: func(ctx context.Context, taskID, input string, timeoutMs int) (string, *agentloop.ToolError) {
			_ = ctx
			beforeScreen, _ := getTaskPaneScreen(taskID)
			path := "/api/v1/tasks/" + url.PathEscape(strings.TrimSpace(taskID)) + "/messages"
			payload := map[string]any{
				"source": "tty_write_stdin",
				"input":  input,
			}
			rawResp, err := callTaskTool(http.MethodPost, path, payload)
			if err != nil {
				return "", err
			}
			waitTimeout := time.Duration(timeoutMs) * time.Millisecond
			postScreen, hashChanged, waitErr := waitTaskPaneStable(taskID, beforeScreen.Output, waitTimeout)
			if waitErr != nil {
				latest, latestErr := getTaskPaneScreen(taskID)
				if latestErr == nil {
					postScreen = latest
					hashChanged = snapshotHash(latest.Output) != snapshotHash(beforeScreen.Output)
					waitErr = nil
				}
			}
			if waitErr != nil {
				return rawResp, nil
			}
			postState := buildPostTerminalScreenState(postScreen)
			postState["hash_changed"] = hashChanged
			postState["timeout_ms"] = timeoutMs
			registerAutoProgressSuppression(
				fmt.Sprint(postState["pane_target"]),
				fmt.Sprint(postState["snapshot_hash"]),
				hashChanged,
			)
			return attachPostTerminalScreenState(rawResp, postState), nil
		},
	}); err != nil {
		return nil, endpoint, model
	}
	if err := registry.Register(&agentloop.ExecCommandTool{
		Exec: func(_ context.Context, taskID, command string, maxOutputTokens int) (string, *agentloop.ToolError) {
			beforeScreen, err := getTaskPaneScreen(taskID)
			if err != nil {
				return "", toAgentToolError(err, "获取任务 pane 状态失败")
			}
			beforeOutput := beforeScreen.Output
			normalizedCommand := ensureCommandEndsWithEnter(command)
			if normalizedCommand == "" {
				return "", agentloop.NewToolError("INVALID_COMMAND", "命令不能为空")
			}
			path := "/api/v1/tasks/" + url.PathEscape(strings.TrimSpace(taskID)) + "/messages"
			if _, err := callTaskTool(http.MethodPost, path, map[string]any{
				"source": "tty_write_stdin",
				"input":  normalizedCommand,
			}); err != nil {
				return "", err
			}

			postScreen, hashChanged, err := waitTaskPaneStable(taskID, beforeOutput, 1800*time.Millisecond)
			if err != nil {
				return "", toAgentToolError(err, "等待命令执行后终端稳定失败")
			}
			lastOutput := postScreen.Output

			delta := lastOutput
			if strings.HasPrefix(lastOutput, beforeOutput) {
				delta = lastOutput[len(beforeOutput):]
			}
			truncated := len(delta) > maxOutputTokens
			delta = clipLogText(delta, maxOutputTokens)
			raw, _ := json.Marshal(map[string]any{
				"ok": true,
				"data": map[string]any{
					"task_id":   strings.TrimSpace(taskID),
					"command":   strings.TrimSpace(command),
					"output":    delta,
					"truncated": truncated,
					"post_terminal_screen_state": map[string]any{
						"pane_target":     strings.TrimSpace(postScreen.PaneTarget),
						"current_command": strings.TrimSpace(postScreen.CurrentCommand),
						"viewport_text":   tailString(postScreen.Output, 4000),
						"snapshot_hash":   snapshotHash(postScreen.Output),
						"hash_changed":    hashChanged,
					},
				},
			})
			registerAutoProgressSuppression(strings.TrimSpace(postScreen.PaneTarget), snapshotHash(postScreen.Output), hashChanged)
			return string(raw), nil
		},
	}); err != nil {
		return nil, endpoint, model
	}
	if err := registry.Register(&agentloop.ReadFileTool{
		Exec: func(_ context.Context, taskID, path string, maxChars int) (string, *agentloop.ToolError) {
			query := url.Values{}
			query.Set("path", strings.TrimSpace(path))
			body, err := callTaskTool(http.MethodGet, "/api/v1/tasks/"+url.PathEscape(strings.TrimSpace(taskID))+"/files/content?"+query.Encode(), nil)
			if err != nil {
				return "", err
			}
			var res struct {
				Data struct {
					Content string `json:"content"`
					Path    string `json:"path"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(body), &res); err != nil {
				return "", agentloop.NewToolError("READFILE_DECODE_FAILED", "解析文件内容响应失败")
			}
			content := res.Data.Content
			truncated := false
			if maxChars > 0 && len(content) > maxChars {
				content = content[:maxChars]
				truncated = true
			}
			raw, _ := json.Marshal(map[string]any{
				"ok": true,
				"data": map[string]any{
					"task_id":     strings.TrimSpace(taskID),
					"path":        strings.TrimSpace(res.Data.Path),
					"content":     content,
					"truncated":   truncated,
					"total_chars": len(res.Data.Content),
				},
			})
			return string(raw), nil
		},
	}); err != nil {
		return nil, endpoint, model
	}
	if err := registry.Register(&agentloop.TaskInputPromptTool{
		Exec: func(_ context.Context, taskID, prompt string) (string, *agentloop.ToolError) {
			promptText := strings.TrimRight(strings.Trim(prompt, " \t"), "\r\n")
			if strings.TrimSpace(promptText) == "" {
				return "", agentloop.NewToolError("INVALID_PROMPT", "prompt 不能为空")
			}
			beforeScreen, _ := getTaskPaneScreen(taskID)
			steps, buildErr := buildInputPromptStepsForCommand(beforeScreen.CurrentCommand, promptText)
			if buildErr != nil {
				return "", toAgentToolError(buildErr, "构建输入步骤失败")
			}
			path := "/api/v1/tasks/" + url.PathEscape(strings.TrimSpace(taskID)) + "/messages"
			rawResp := ""
			for _, step := range steps {
				if step.Delay > 0 {
					time.Sleep(step.Delay)
				}
				var toolErr *agentloop.ToolError
				rawResp, toolErr = callTaskTool(http.MethodPost, path, map[string]any{
					"source": "tty_write_stdin",
					"input":  step.Input,
				})
				if toolErr != nil {
					return "", toolErr
				}
			}
			timeoutMs := 3000
			waitTimeout := time.Duration(timeoutMs) * time.Millisecond
			postScreen, hashChanged, waitErr := waitTaskPaneStable(taskID, beforeScreen.Output, waitTimeout)
			if waitErr != nil {
				latest, latestErr := getTaskPaneScreen(taskID)
				if latestErr == nil {
					postScreen = latest
					hashChanged = snapshotHash(latest.Output) != snapshotHash(beforeScreen.Output)
					waitErr = nil
				}
			}
			if waitErr != nil {
				return rawResp, nil
			}
			postState := buildPostTerminalScreenState(postScreen)
			postState["hash_changed"] = hashChanged
			postState["timeout_ms"] = timeoutMs
			registerAutoProgressSuppression(
				fmt.Sprint(postState["pane_target"]),
				fmt.Sprint(postState["snapshot_hash"]),
				hashChanged,
			)
			return attachPostTerminalScreenState(rawResp, postState), nil
		},
	}); err != nil {
		return nil, endpoint, model
	}
	if err := registry.Register(&agentloop.TaskChildGetContextTool{
		Exec: func(_ context.Context, taskID, childTaskID string) (string, *agentloop.ToolError) {
			_, nodes, err := getTaskTree(taskID)
			if err != nil {
				return "", toAgentToolError(err, "读取任务树失败")
			}
			parentID := strings.TrimSpace(taskID)
			targetID := strings.TrimSpace(childTaskID)
			for _, node := range nodes {
				if strings.TrimSpace(fmt.Sprintf("%v", node["task_id"])) != targetID {
					continue
				}
				if strings.TrimSpace(fmt.Sprintf("%v", node["parent_task_id"])) != parentID {
					return "", agentloop.NewToolError("NOT_A_CHILD_TASK", "目标任务不是当前任务的子任务")
				}
				raw, _ := json.Marshal(map[string]any{"ok": true, "data": node})
				return string(raw), nil
			}
			return "", agentloop.NewToolError("CHILD_NOT_FOUND", "未找到对应子任务")
		},
	}); err != nil {
		return nil, endpoint, model
	}
	if err := registry.Register(&agentloop.TaskChildGetTTYOutputTool{
		Exec: func(_ context.Context, taskID, childTaskID string, offset int) (string, *agentloop.ToolError) {
			_, nodes, err := getTaskTree(taskID)
			if err != nil {
				return "", toAgentToolError(err, "读取任务树失败")
			}
			parentID := strings.TrimSpace(taskID)
			targetID := strings.TrimSpace(childTaskID)
			isChild := false
			for _, node := range nodes {
				if strings.TrimSpace(fmt.Sprintf("%v", node["task_id"])) == targetID &&
					strings.TrimSpace(fmt.Sprintf("%v", node["parent_task_id"])) == parentID {
					isChild = true
					break
				}
			}
			if !isChild {
				return "", agentloop.NewToolError("NOT_A_CHILD_TASK", "目标任务不是当前任务的子任务")
			}
			paneTarget, currentCommand, output, err := getTaskPaneOutput(targetID)
			if err != nil {
				return "", toAgentToolError(err, "读取子任务终端输出失败")
			}
			if offset > len(output) {
				offset = len(output)
			}
			start := len(output) - offset
			if start < 0 {
				start = 0
			}
			raw, _ := json.Marshal(map[string]any{
				"ok": true,
				"data": map[string]any{
					"parent_task_id":  parentID,
					"child_task_id":   targetID,
					"offset":          offset,
					"output":          output[start:],
					"has_more":        start > 0,
					"state":           "idle",
					"current_command": currentCommand,
					"cwd":             paneTarget,
				},
			})
			return string(raw), nil
		},
	}); err != nil {
		return nil, endpoint, model
	}
	if err := registry.Register(&agentloop.TaskChildSpawnTool{
		Exec: func(_ context.Context, taskID, command, title, description, prompt, taskRole string) (string, *agentloop.ToolError) {
			return executeTaskChildSpawnAction(callTaskTool, taskID, command, title, description, prompt, taskRole)
		},
	}); err != nil {
		return nil, endpoint, model
	}
	if err := registry.Register(&agentloop.TaskChildSendMessageTool{
		Exec: func(_ context.Context, taskID, childTaskID, message string) (string, *agentloop.ToolError) {
			body, err := callTaskTool(http.MethodPost, "/api/v1/tasks/"+url.PathEscape(strings.TrimSpace(childTaskID))+"/messages", map[string]any{
				"content":      strings.TrimSpace(message),
				"source":       "parent_message",
				"parent_task":  strings.TrimSpace(taskID),
				"display_text": strings.TrimSpace(message),
			})
			if err != nil {
				return "", err
			}
			var queued struct {
				OK bool `json:"ok"`
			}
			_ = json.Unmarshal([]byte(body), &queued)
			raw, _ := json.Marshal(map[string]any{
				"ok": true,
				"data": map[string]any{
					"parent_task_id": strings.TrimSpace(taskID),
					"child_task_id":  strings.TrimSpace(childTaskID),
					"enqueued":       true,
					"source":         "parent_message",
				},
			})
			return string(raw), nil
		},
	}); err != nil {
		return nil, endpoint, model
	}
	if err := registry.Register(&agentloop.TaskParentReportTool{
		Exec: func(_ context.Context, taskID, summary string) (string, *agentloop.ToolError) {
			_, err := callTaskTool(http.MethodPost, "/api/v1/tasks/"+url.PathEscape(strings.TrimSpace(taskID))+"/messages", map[string]any{
				"content": strings.TrimSpace(summary),
				"source":  "child_report",
			})
			if err != nil {
				return "", err
			}
			raw, _ := json.Marshal(map[string]any{
				"ok": true,
				"data": map[string]any{
					"task_id":  strings.TrimSpace(taskID),
					"summary":  strings.TrimSpace(summary),
					"enqueued": true,
					"source":   "child_report",
				},
			})
			return string(raw), nil
		},
	}); err != nil {
		return nil, endpoint, model
	}
	client := agentloop.NewResponsesClient(agentloop.OpenAIConfig{
		BaseURL: endpoint,
		Model:   model,
		APIKey:  apiKey,
	}, http.DefaultClient)
	return agentloop.NewLoopRunner(client, registry, agentloop.LoopRunnerOptions{MaxIterations: 8}), endpoint, model
}

func ensureCommandEndsWithEnter(command string) string {
	raw := command
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if strings.HasSuffix(raw, "<ENTER>") {
		return strings.TrimRight(strings.TrimSuffix(raw, "<ENTER>"), " \t") + "\r"
	}
	if strings.HasSuffix(raw, "\r") || strings.HasSuffix(raw, "\n") {
		return raw
	}
	return strings.TrimRight(raw, " \t") + "\r"
}

func buildInputPromptStepsForCommand(currentCommand, prompt string) ([]progdetector.PromptStep, error) {
	prompt = strings.TrimRight(strings.Trim(prompt, " \t"), "\r\n")
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("INVALID_PROMPT")
	}
	if detector, ok := progdetector.ProgramDetectorRegistry.DetectByCurrentCommand(currentCommand); ok {
		return detector.BuildInputPromptSteps(prompt)
	}
	return []progdetector.PromptStep{
		{Input: prompt, TimeoutMs: 15000},
		{Input: "\r", Delay: 50 * time.Millisecond, TimeoutMs: 1000},
	}, nil
}

func attachPostTerminalScreenState(raw string, post map[string]any) string {
	if strings.TrimSpace(raw) == "" || len(post) == 0 {
		return raw
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}
	data, _ := payload["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
		payload["data"] = data
	}
	data["post_terminal_screen_state"] = post
	next, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return string(next)
}

func tailString(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[len(text)-limit:]
}

func snapshotHash(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])
}

func executeTaskChildSpawnAction(
	callTaskTool func(method, path string, payload any) (string, *agentloop.ToolError),
	taskID, command, title, description, prompt, taskRole string,
) (string, *agentloop.ToolError) {
	parentTaskID := strings.TrimSpace(taskID)
	command = ensureCommandEndsWithEnter(command)
	prompt = strings.TrimSpace(prompt)
	taskRole = strings.ToLower(strings.TrimSpace(taskRole))
	if taskRole != projectstate.TaskRolePlanner && taskRole != projectstate.TaskRoleExecutor {
		return "", agentloop.NewToolError("INVALID_TASK_ROLE", "task_role must be planner|executor")
	}

	body, err := callTaskTool(http.MethodPost, "/api/v1/tasks/"+url.PathEscape(parentTaskID)+"/panes/child", map[string]any{
		"title":     title,
		"task_role": taskRole,
	})
	if err != nil {
		return "", err
	}
	var created struct {
		Data struct {
			TaskID     string `json:"task_id"`
			RunID      string `json:"run_id"`
			PaneTarget string `json:"pane_target"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		return "", agentloop.NewToolError("TASK_CHILD_SPAWN_DECODE_FAILED", "解析子任务创建响应失败")
	}
	childTaskID := strings.TrimSpace(created.Data.TaskID)
	if _, err := callTaskTool(http.MethodPatch, "/api/v1/tasks/"+url.PathEscape(childTaskID)+"/description", map[string]any{
		"description": description,
	}); err != nil {
		return "", err
	}

	sidecarModeBody, err := callTaskTool(http.MethodGet, "/api/v1/tasks/"+url.PathEscape(parentTaskID)+"/sidecar-mode", nil)
	if err != nil {
		return "", err
	}
	var parentSidecarMode struct {
		Data struct {
			SidecarMode string `json:"sidecar_mode"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(sidecarModeBody), &parentSidecarMode); err != nil {
		return "", agentloop.NewToolError("TASK_CHILD_SPAWN_SIDECAR_MODE_DECODE_FAILED", "解析父任务 sidecar_mode 响应失败")
	}
	if _, err := callTaskTool(http.MethodPatch, "/api/v1/tasks/"+url.PathEscape(childTaskID)+"/sidecar-mode", map[string]any{
		"sidecar_mode": parentSidecarMode.Data.SidecarMode,
	}); err != nil {
		return "", err
	}

	if command != "" {
		if _, err := callTaskTool(http.MethodPost, "/api/v1/tasks/"+url.PathEscape(childTaskID)+"/messages", map[string]any{
			"source": "tty_write_stdin",
			"input":  command,
		}); err != nil {
			return "", err
		}
	}
	if prompt != "" {
		if _, err := callTaskTool(http.MethodPost, "/api/v1/tasks/"+url.PathEscape(childTaskID)+"/messages", map[string]any{
			"source":       "parent_message",
			"parent_task":  parentTaskID,
			"content":      prompt,
			"display_text": prompt,
		}); err != nil {
			return "", err
		}
	}

	raw, _ := json.Marshal(map[string]any{
		"ok": true,
		"data": map[string]any{
			"parent_task_id": parentTaskID,
			"task_id":        childTaskID,
			"run_id":         strings.TrimSpace(created.Data.RunID),
			"pane_target":    strings.TrimSpace(created.Data.PaneTarget),
			"sidecar_mode":   parentSidecarMode.Data.SidecarMode,
		},
	})
	return string(raw), nil
}

func resolveAgentOpenAIConfig(cfg config.Config, helperStore localapi.HelperConfigStore) (string, string, string) {
	if helperStore != nil {
		if helperCfg, err := helperStore.LoadOpenAI(); err == nil {
			endpoint := strings.TrimSpace(helperCfg.Endpoint)
			model := strings.TrimSpace(helperCfg.Model)
			apiKey := strings.TrimSpace(helperCfg.APIKey)
			if endpoint != "" && model != "" && apiKey != "" {
				return endpoint, model, apiKey
			}
		}
	}
	return strings.TrimSpace(cfg.OpenAIEndpoint), strings.TrimSpace(cfg.OpenAIModel), strings.TrimSpace(cfg.OpenAIAPIKey)
}

func clipLogText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len(text) <= limit {
		return text
	}
	return text[:limit] + "...(truncated)"
}

func toAgentToolError(err error, suggest string) *agentloop.ToolError {
	if err == nil {
		return nil
	}
	if toolErr, ok := err.(*agentloop.ToolError); ok && toolErr != nil {
		return toolErr
	}
	return agentloop.NewToolError(strings.TrimSpace(err.Error()), strings.TrimSpace(suggest))
}

func newRuntimeLogger(writer io.Writer) *slog.Logger {
	level := "info"
	if traceStreamEnabled {
		level = "debug"
	}
	return logging.NewLogger(logging.Options{
		Level:     level,
		Writer:    writer,
		Component: "shellman",
	})
}

func buildGatewayLocalAPIServer(configDir string, tmuxService bridge.TmuxService, pickDirectory func() (string, error)) *localapi.Server {
	_ = projectstate.InitGlobalDB(filepath.Join(configDir, "shellman.db"))
	cfgStore := global.NewConfigStore(configDir)
	appProgramsStore := global.NewAppProgramsStore(configDir)
	projectsStore := global.NewProjectsStore(configDir)
	var helperCfgStore localapi.HelperConfigStore
	var historyStore localapi.DirHistory
	if gdb, err := projectstate.GlobalDBGORM(); err == nil {
		if st, err := helperconfig.NewStore(gdb, filepath.Join(configDir, ".shellman-helper-openai-secret")); err == nil {
			helperCfgStore = st
		}
		if st, err := historydb.NewStore(gdb); err == nil {
			historyStore = st
		}
	}
	deps := localapi.Deps{
		ConfigStore:       cfgStore,
		AppProgramsStore:  appProgramsStore,
		HelperConfigStore: helperCfgStore,
		ProjectsStore:     projectsStore,
		PickDirectory:     pickDirectory,
		FSBrowser:         fsbrowser.NewService(),
		DirHistory:        historyStore,
	}
	if paneService, ok := tmuxService.(localapi.PaneService); ok {
		deps.PaneService = paneService
	}
	if promptSender, ok := tmuxService.(localapi.TaskPromptSender); ok {
		deps.TaskPromptSender = promptSender
	}
	return localapi.NewServer(deps)
}

func newGatewayExecutors(localServer *localapi.Server, source string) (gatewayHTTPExecutor, paneAutoCompletionExecutor) {
	if localServer == nil {
		return nil, nil
	}
	exec := localapi.NewHTTPExecutor(localServer.Handler())
	httpExec := func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
		forwardHeaders := map[string]string{}
		for k, v := range headers {
			forwardHeaders[k] = v
		}
		if strings.TrimSpace(forwardHeaders["X-Shellman-Gateway-Source"]) == "" {
			forwardHeaders["X-Shellman-Gateway-Source"] = strings.TrimSpace(source)
		}
		return exec.Execute(method, path, forwardHeaders, body)
	}
	autoComplete := func(paneTarget string, observedLastActiveAt time.Time) (localapi.AutoCompleteByPaneResult, error) {
		observedUnix := int64(0)
		if !observedLastActiveAt.IsZero() {
			observedUnix = observedLastActiveAt.UTC().Unix()
		}
		meta := map[string]any{
			"caller_method":         "INTERNAL",
			"caller_path":           "internal:auto-progress",
			"caller_user_agent":     "",
			"caller_turn_uuid":      "",
			"caller_gateway_source": strings.TrimSpace(source),
			"caller_active_pane":    strings.TrimSpace(paneTarget),
		}
		return localServer.AutoCompleteByPane(localapi.AutoCompleteByPaneInput{
			PaneTarget:           strings.TrimSpace(paneTarget),
			TriggerSource:        "pane-actor",
			ObservedLastActiveAt: observedUnix,
			RequestMeta:          meta,
			CallerPath:           "internal:auto-progress",
			CallerActivePane:     strings.TrimSpace(paneTarget),
		})
	}
	return httpExec, autoComplete
}

func buildGatewayHTTPExecutor(configDir string, tmuxService bridge.TmuxService, pickDirectory func() (string, error)) gatewayHTTPExecutor {
	localServer := buildGatewayLocalAPIServer(configDir, tmuxService, pickDirectory)
	httpExec, _ := newGatewayExecutors(localServer, "local-agent-gateway-http")
	return func(method, path string, headers map[string]string, body string) (int, map[string]string, string, error) {
		if httpExec == nil {
			return 500, map[string]string{}, "", errors.New("gateway http executor is unavailable")
		}
		return httpExec(method, path, headers, body)
	}
}
