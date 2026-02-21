package localapi

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type taskCompletionAuditLogger struct {
	logger *slog.Logger
	file   io.Closer
}

func newTaskCompletionAuditLogger(repoRoot string) *taskCompletionAuditLogger {
	return newAuditLogger(repoRoot, "task-completion-automation.log", "task_completion_automation")
}

func newTaskMessagesAuditLogger(repoRoot string) *taskCompletionAuditLogger {
	return newAuditLogger(repoRoot, "task-messages.log", "task_messages")
}

func newAuditLogger(repoRoot, fileName, component string) *taskCompletionAuditLogger {
	if strings.TrimSpace(repoRoot) == "" {
		return nil
	}
	dir := filepath.Join(repoRoot, ".muxt", "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil
	}
	logPath := filepath.Join(dir, strings.TrimSpace(fileName))
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil
	}
	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &taskCompletionAuditLogger{
		logger: slog.New(handler).With("component", strings.TrimSpace(component)),
		file:   f,
	}
}

func (l *taskCompletionAuditLogger) Close() {
	if l == nil || l.file == nil {
		return
	}
	_ = l.file.Close()
}

func (l *taskCompletionAuditLogger) Log(stage string, fields map[string]any) {
	if l == nil || l.logger == nil {
		return
	}
	attrs := make([]any, 0, 2+len(fields)*2)
	attrs = append(attrs, "stage", strings.TrimSpace(stage))
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		attrs = append(attrs, k, fields[k])
	}
	l.logger.Info(strings.TrimSpace(stage), attrs...)
}
