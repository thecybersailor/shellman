package localapi

import (
	"strings"
	"testing"

	"shellman/cli/internal/projectstate"
)

func TestBuildTaskPromptHistory_UsesLocalMessagesInChronologicalOrder(t *testing.T) {
	msgs := []projectstate.TaskMessageRecord{
		{ID: 1, Role: "user", Content: "first"},
		{ID: 2, Role: "assistant", Content: `{"text":"reply-1"}`},
		{ID: 3, Role: "user", Content: "second"},
	}
	block, meta := buildTaskPromptHistory(msgs, TaskPromptHistoryOptions{MaxMessages: 50, MaxChars: 8000})
	if !strings.Contains(block, "[user#1] first") {
		t.Fatalf("expected history contains first user message, got: %q", block)
	}
	if !strings.Contains(block, "[assistant#2] reply-1") {
		t.Fatalf("expected history contains assistant message, got: %q", block)
	}
	if !strings.Contains(block, "[user#3] second") {
		t.Fatalf("expected history contains second user message, got: %q", block)
	}
	if meta.TotalMessages != 3 {
		t.Fatalf("expected total messages 3, got %d", meta.TotalMessages)
	}
}

func TestBuildTaskPromptHistory_WhenOverflow_AddsSummaryAndKeepsRecentTurns(t *testing.T) {
	msgs := make([]projectstate.TaskMessageRecord, 0, 120)
	for i := 1; i <= 120; i++ {
		role := "assistant"
		if i%2 == 1 {
			role = "user"
		}
		msgs = append(msgs, projectstate.TaskMessageRecord{
			ID:      int64(i),
			Role:    role,
			Content: strings.Repeat("m", 120),
		})
	}
	block, meta := buildTaskPromptHistory(msgs, TaskPromptHistoryOptions{MaxMessages: 30, MaxChars: 2000})
	if !strings.Contains(block, "history_summary:") {
		t.Fatalf("expected summary section, got: %q", block)
	}
	if !strings.Contains(block, "recent_history:") {
		t.Fatalf("expected recent history section, got: %q", block)
	}
	if meta.Dropped <= 0 {
		t.Fatalf("expected dropped messages > 0, got %d", meta.Dropped)
	}
}

func TestBuildTaskPromptHistory_Deterministic(t *testing.T) {
	msgs := []projectstate.TaskMessageRecord{
		{ID: 1, Role: "user", Content: "alpha"},
		{ID: 2, Role: "assistant", Content: "beta"},
		{ID: 3, Role: "user", Content: "gamma"},
		{ID: 4, Role: "assistant", Content: "delta"},
	}
	opts := TaskPromptHistoryOptions{MaxMessages: 3, MaxChars: 60}
	a, _ := buildTaskPromptHistory(msgs, opts)
	b, _ := buildTaskPromptHistory(msgs, opts)
	if a != b {
		t.Fatalf("expected deterministic output, a=%q b=%q", a, b)
	}
}
