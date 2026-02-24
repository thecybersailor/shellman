package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shellman/cli/internal/config"
)

func TestRunServe_LocalAlwaysRequired(t *testing.T) {
	cfg := config.Config{TurnEnabled: false}
	localCalled := false
	turnCalled := false

	err := runServe(
		context.Background(),
		cfg,
		func(context.Context) error {
			localCalled = true
			return nil
		},
		func(context.Context) error {
			turnCalled = true
			return nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("runServe failed: %v", err)
	}
	if !localCalled || turnCalled {
		t.Fatalf("expected local only, turnCalled=%v localCalled=%v", turnCalled, localCalled)
	}
}

func TestRunServe_TurnFailureDoesNotFailLocal(t *testing.T) {
	cfg := config.Config{TurnEnabled: true}
	localCalled := false
	turnCalled := false

	err := runServe(
		context.Background(),
		cfg,
		func(context.Context) error {
			localCalled = true
			return nil
		},
		func(context.Context) error {
			turnCalled = true
			return errors.New("register failed")
		},
		testLogger(),
	)
	if err != nil {
		t.Fatalf("runServe should ignore turn failure, got %v", err)
	}
	if !turnCalled || !localCalled {
		t.Fatalf("expected both paths attempted, turnCalled=%v localCalled=%v", turnCalled, localCalled)
	}
}

func TestLocalBrowserURL_UsesLoopbackForWildcardHost(t *testing.T) {
	cfg := config.Config{LocalHost: "0.0.0.0", LocalPort: 8000}
	if got := localBrowserURL(cfg); got != "http://127.0.0.1:8000" {
		t.Fatalf("unexpected browser url: %s", got)
	}
}

func TestMaybeOpenBrowser_EnabledCallsOpener(t *testing.T) {
	cfg := config.Config{LocalHost: "127.0.0.1", LocalPort: 8000, OpenBrowser: true}
	var gotURL string
	var out bytes.Buffer
	maybeOpenBrowser(&out, cfg, func(rawURL string) error {
		gotURL = rawURL
		return nil
	})
	if gotURL != "http://127.0.0.1:8000" {
		t.Fatalf("unexpected opened url: %s", gotURL)
	}
}

func TestMaybeOpenBrowser_FailureDoesNotReturnError(t *testing.T) {
	cfg := config.Config{LocalHost: "127.0.0.1", LocalPort: 8000, OpenBrowser: true}
	var out bytes.Buffer
	maybeOpenBrowser(&out, cfg, func(string) error {
		return errors.New("open failed")
	})
	if out.String() == "" {
		t.Fatal("expected warning output when open fails")
	}
}

func TestResolveServePIDFile_DefaultUsesConfigDir(t *testing.T) {
	cfg := config.Config{}
	got := resolveServePIDFile("/tmp/shellman-config", cfg)
	if got != "/tmp/shellman-config/shellman-serve.pid" {
		t.Fatalf("unexpected pid file path: %s", got)
	}
}

func TestResolveServePIDFile_UsesOverride(t *testing.T) {
	cfg := config.Config{PIDFile: "/tmp/custom.pid"}
	got := resolveServePIDFile("/tmp/shellman-config", cfg)
	if got != "/tmp/custom.pid" {
		t.Fatalf("unexpected pid file path: %s", got)
	}
}

func TestAcquireServePIDFileLock_MutualExclusion(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "shellman.pid")
	release1, err := acquireServePIDFileLock(pidPath)
	if err != nil {
		t.Fatalf("first lock should succeed: %v", err)
	}
	defer release1()

	_, err = acquireServePIDFileLock(pidPath)
	if err == nil {
		t.Fatal("second lock should fail when pid file exists")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Fatalf("unexpected lock error: %v", err)
	}

	release1()
	release2, err := acquireServePIDFileLock(pidPath)
	if err != nil {
		t.Fatalf("lock should succeed after release: %v", err)
	}
	release2()
}

func TestAcquireServePIDFileLock_StalePIDFileCanBeRecovered(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "shellman.pid")
	if err := os.WriteFile(pidPath, []byte("99999999\n"), 0o644); err != nil {
		t.Fatalf("write stale pid file: %v", err)
	}

	release, err := acquireServePIDFileLock(pidPath)
	if err != nil {
		t.Fatalf("stale pid lock should be recovered: %v", err)
	}
	defer release()
}
