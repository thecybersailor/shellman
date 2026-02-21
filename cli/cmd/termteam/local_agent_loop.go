package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"termteam/cli/internal/bridge"
	"termteam/cli/internal/turn"
)

func startLocalAgentLoop(
	ctx context.Context,
	localPort int,
	dialer wsDialer,
	tmuxService bridge.TmuxService,
	httpExec gatewayHTTPExecutor,
	autoCompleteExec paneAutoCompletionExecutor,
	logger *slog.Logger,
) error {
	if logger == nil {
		logger = newRuntimeLogger(io.Discard)
	}
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/ws/agent/local", localPort)

	for {
		if ctx.Err() != nil {
			return nil
		}

		sock, err := dialer.Dial(ctx, wsURL)
		if err != nil {
			logger.Warn("local agent dial failed", "ws_url", wsURL, "err", err)
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		logger.Info("local agent connected", "ws_url", wsURL)

		wsClient := turn.NewWSClient(sock)
		runErr := runWSRuntime(ctx, wsClient, tmuxService, httpExec, autoCompleteExec, logger)
		_ = wsClient.Close()
		if ctx.Err() != nil {
			return nil
		}
		if runErr != nil {
			logger.Warn("local agent runtime ended, reconnecting", "err", runErr)
		} else {
			logger.Info("local agent runtime ended, reconnecting")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(100 * time.Millisecond):
		}
	}
}
