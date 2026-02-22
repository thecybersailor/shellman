package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeControlSessionClient struct {
	lines     chan string
	paneMap   map[string]string
	paneMapMu sync.RWMutex
	refreshFn func() error
}

func (f *fakeControlSessionClient) Lines() <-chan string {
	return f.lines
}

func (f *fakeControlSessionClient) PaneMap() map[string]string {
	f.paneMapMu.RLock()
	defer f.paneMapMu.RUnlock()
	out := make(map[string]string, len(f.paneMap))
	for k, v := range f.paneMap {
		out[k] = v
	}
	return out
}

func (f *fakeControlSessionClient) RefreshPaneMap() error {
	if f.refreshFn == nil {
		return nil
	}
	return f.refreshFn()
}

func (f *fakeControlSessionClient) Close() error {
	close(f.lines)
	return nil
}

func TestControlModeHub_SubscribeRoutesByTarget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &fakeControlSessionClient{
		lines:   make(chan string, 8),
		paneMap: map[string]string{"%1": "e2e:0.0"},
	}

	hub := newControlModeHubWithFactory(ctx, "", testLogger(), func(context.Context, string, string) (controlSessionClient, error) {
		return client, nil
	})

	out, unsubscribe, err := hub.Subscribe("e2e:0.0")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer unsubscribe()

	client.lines <- `%output %1 hi\012`

	select {
	case got := <-out:
		if got != "hi\n" {
			t.Fatalf("unexpected routed payload: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected routed control output")
	}
}

func TestControlModeHub_RefreshesPaneMapForNewPane(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	refreshCalls := 0
	client := &fakeControlSessionClient{
		lines: make(chan string, 8),
		paneMap: map[string]string{
			"%1": "e2e:0.0",
		},
	}
	client.refreshFn = func() error {
		refreshCalls++
		// Simulate pane creation happening after initial subscribe-time refresh.
		if refreshCalls >= 2 {
			client.paneMapMu.Lock()
			client.paneMap["%2"] = "e2e:0.1"
			client.paneMapMu.Unlock()
		}
		return nil
	}

	hub := newControlModeHubWithFactory(ctx, "", testLogger(), func(context.Context, string, string) (controlSessionClient, error) {
		return client, nil
	})

	out, unsubscribe, err := hub.Subscribe("e2e:0.1")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer unsubscribe()

	client.lines <- `%output %2 hi\012`

	select {
	case got := <-out:
		if got != "hi\n" {
			t.Fatalf("unexpected routed payload: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected routed control output after pane map refresh")
	}

	if refreshCalls == 0 {
		t.Fatal("expected pane map refresh to be attempted")
	}
}

func TestControlModeHub_BuffersPartialUTF8AcrossChunks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := &fakeControlSessionClient{
		lines:   make(chan string, 8),
		paneMap: map[string]string{"%1": "e2e:0.0"},
	}

	hub := newControlModeHubWithFactory(ctx, "", testLogger(), func(context.Context, string, string) (controlSessionClient, error) {
		return client, nil
	})

	out, unsubscribe, err := hub.Subscribe("e2e:0.0")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}
	defer unsubscribe()

	// "你" in UTF-8 is E4 BD A0. Split it across two control chunks.
	client.lines <- `%output %1 \344\275`

	select {
	case got := <-out:
		t.Fatalf("partial utf8 chunk should be buffered, got immediate output: %q", got)
	case <-time.After(80 * time.Millisecond):
	}

	client.lines <- `%output %1 \240`

	select {
	case got := <-out:
		if got != "你" {
			t.Fatalf("unexpected reconstructed output: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("expected reconstructed utf8 output")
	}
}

func TestLoadPaneIDMap_UsesSessionScopeListPanes(t *testing.T) {
	binDir := t.TempDir()
	tmuxPath := filepath.Join(binDir, "tmux")
	script := `#!/bin/sh
has_list=0
has_s=0
for arg in "$@"; do
  if [ "$arg" = "list-panes" ]; then
    has_list=1
  fi
  if [ "$arg" = "-s" ]; then
    has_s=1
  fi
done

if [ "$has_list" -ne 1 ]; then
  echo "unexpected command" >&2
  exit 64
fi
if [ "$has_s" -ne 1 ]; then
  echo "missing -s" >&2
  exit 65
fi

printf "%%16|%%16\n%%17|%%17\n%%18|%%18\n"
`
	if err := os.WriteFile(tmuxPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux failed: %v", err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	paneMap, err := loadPaneIDMap("", "e2e")
	if err != nil {
		t.Fatalf("loadPaneIDMap failed: %v", err)
	}
	if got := paneMap["%16"]; got != "%16" {
		t.Fatalf("unexpected pane mapping for %%16: %q", got)
	}
	if got := paneMap["%17"]; got != "%17" {
		t.Fatalf("unexpected pane mapping for %%17: %q", got)
	}
	if got := paneMap["%18"]; got != "%18" {
		t.Fatalf("unexpected pane mapping for %%18: %q", got)
	}
}

func TestResolveSessionFromPaneTarget_PaneID(t *testing.T) {
	binDir := t.TempDir()
	tmuxPath := filepath.Join(binDir, "tmux")
	script := `#!/bin/sh
if [ "$1" != "display-message" ]; then
  echo "unexpected command" >&2
  exit 64
fi
printf "e2e\n"
`
	if err := os.WriteFile(tmuxPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tmux failed: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	session, err := resolveSessionFromPaneTarget("", "%42")
	if err != nil {
		t.Fatalf("resolveSessionFromPaneTarget failed: %v", err)
	}
	if session != "e2e" {
		t.Fatalf("unexpected session: %q", session)
	}
}

func TestResolveSessionFromPaneTarget_InvalidTarget(t *testing.T) {
	_, err := resolveSessionFromPaneTarget("", "bad-target")
	if err == nil {
		t.Fatal("expected invalid target error")
	}
	if !strings.Contains(err.Error(), "invalid pane target") {
		t.Fatalf("unexpected error: %v", err)
	}
}
