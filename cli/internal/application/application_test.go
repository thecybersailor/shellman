package application

import (
	"context"
	"testing"
)

func TestStartApplication_LocalMode_BootstrapAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app, err := StartApplication(ctx, StartOptions{
		Mode:      "local",
		ConfigDir: t.TempDir(),
		DBDSN:     "file:startapp_local?mode=memory&cache=shared",
		LocalHost: "127.0.0.1",
		LocalPort: pickFreePort(t),
	})
	if err != nil {
		t.Fatalf("start application failed: %v", err)
	}
	if app == nil {
		t.Fatal("expected non-nil app")
	}
	if app.LocalAPIBaseURL() == "" {
		t.Fatal("expected local api base url")
	}
	if err := app.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestStartApplication_UsesInjectedMemoryDBDSN(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfgDir := t.TempDir()
	app, err := StartApplication(ctx, StartOptions{
		Mode:      "local",
		ConfigDir: cfgDir,
		DBDSN:     "file:startapp_mem_only?mode=memory&cache=shared",
		LocalHost: "127.0.0.1",
		LocalPort: pickFreePort(t),
	})
	if err != nil {
		t.Fatalf("start application failed: %v", err)
	}
	if app == nil {
		t.Fatal("expected non-nil app")
	}
	if app.DBDSN() != "file:startapp_mem_only?mode=memory&cache=shared" {
		t.Fatalf("expected in-memory dsn kept, got %q", app.DBDSN())
	}
	if err := app.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestStartApplication_ProdAndTestShareSameBootstrapPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	prodApp, err := StartApplication(ctx, StartOptions{
		Mode:      "local",
		ConfigDir: t.TempDir(),
		DBDSN:     "file:startapp_prod_like?mode=memory&cache=shared",
		LocalHost: "127.0.0.1",
		LocalPort: pickFreePort(t),
	})
	if err != nil {
		t.Fatalf("prod-like start failed: %v", err)
	}
	testApp, err := StartApplication(ctx, StartOptions{
		Mode:      "local",
		ConfigDir: t.TempDir(),
		DBDSN:     "file:startapp_test_like?mode=memory&cache=shared",
		LocalHost: "127.0.0.1",
		LocalPort: pickFreePort(t),
	})
	if err != nil {
		t.Fatalf("test-like start failed: %v", err)
	}

	if prodApp.BootstrapPath() == "" || testApp.BootstrapPath() == "" {
		t.Fatal("expected bootstrap path marker")
	}
	if prodApp.BootstrapPath() != testApp.BootstrapPath() {
		t.Fatalf("expected shared bootstrap path, got prod=%q test=%q", prodApp.BootstrapPath(), testApp.BootstrapPath())
	}
	_ = prodApp.Shutdown(ctx)
	_ = testApp.Shutdown(ctx)
}
