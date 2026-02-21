package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

type Options struct {
	Level     string
	Writer    io.Writer
	Component string
}

func NewLogger(opts Options) *slog.Logger {
	writer := opts.Writer
	if writer == nil {
		writer = os.Stderr
	}
	h := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: parseLevel(opts.Level)})
	lg := slog.New(h)
	if strings.TrimSpace(opts.Component) != "" {
		lg = lg.With("component", strings.TrimSpace(opts.Component))
	}
	return lg
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
