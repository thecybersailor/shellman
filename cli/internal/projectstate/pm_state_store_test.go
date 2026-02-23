package projectstate

import (
	"path/filepath"
	"testing"
	"time"
)

func TestPMStateStore_SessionCRUDAndOrderByLastMessageAt(t *testing.T) {
	store := newPMStateStore(t)

	s1, err := store.CreatePMSession("p1", "Session A")
	if err != nil {
		t.Fatalf("CreatePMSession s1 failed: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	s2, err := store.CreatePMSession("p1", "Session B")
	if err != nil {
		t.Fatalf("CreatePMSession s2 failed: %v", err)
	}

	if _, err := store.InsertPMMessage(s1, "user", "hello-a", StatusCompleted, ""); err != nil {
		t.Fatalf("InsertPMMessage s1 failed: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := store.InsertPMMessage(s2, "user", "hello-b", StatusCompleted, ""); err != nil {
		t.Fatalf("InsertPMMessage s2 failed: %v", err)
	}

	sessions, err := store.ListPMSessionsByProject("p1", 20)
	if err != nil {
		t.Fatalf("ListPMSessionsByProject failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].SessionID != s2 {
		t.Fatalf("expected latest session first, got first=%s second=%s", sessions[0].SessionID, sessions[1].SessionID)
	}
	if sessions[1].SessionID != s1 {
		t.Fatalf("expected second session id=%s, got %s", s1, sessions[1].SessionID)
	}
}

func TestPMStateStore_ListPMMessages_ASC(t *testing.T) {
	store := newPMStateStore(t)

	sid, err := store.CreatePMSession("p1", "Session")
	if err != nil {
		t.Fatalf("CreatePMSession failed: %v", err)
	}
	if _, err := store.InsertPMMessage(sid, "user", "u1", StatusCompleted, ""); err != nil {
		t.Fatalf("InsertPMMessage user failed: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := store.InsertPMMessage(sid, "assistant", "a1", StatusCompleted, ""); err != nil {
		t.Fatalf("InsertPMMessage assistant failed: %v", err)
	}

	items, err := store.ListPMMessages(sid, 50)
	if err != nil {
		t.Fatalf("ListPMMessages failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(items))
	}
	if items[0].Role != "user" || items[1].Role != "assistant" {
		t.Fatalf("unexpected order: %#v", items)
	}
}

func newPMStateStore(t *testing.T) *Store {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "shellman.db")
	if err := InitGlobalDB(dbPath); err != nil {
		t.Fatalf("InitGlobalDB failed: %v", err)
	}
	return NewStore(t.TempDir())
}
