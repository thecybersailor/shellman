package main

import (
	"context"
	"io"
	"testing"

	"shellman/cli/internal/application"
	"shellman/cli/internal/config"
)

func TestRunLocal_DelegatesToStartApplication(t *testing.T) {
	original := startApplication
	defer func() { startApplication = original }()

	called := 0
	startApplication = func(ctx context.Context, opts application.StartOptions) (*application.Application, error) {
		called++
		if opts.Mode != "local" {
			t.Fatalf("expected local mode, got %q", opts.Mode)
		}
		return application.StartApplication(ctx, application.StartOptions{
			Mode: opts.Mode,
			Hooks: application.Hooks{
				Run: func(context.Context) error { return nil },
			},
		})
	}

	err := runLocal(context.Background(), io.Discard, config.Config{LocalHost: "127.0.0.1", LocalPort: 4621})
	if err != nil {
		t.Fatalf("runLocal failed: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected startApplication called once, got %d", called)
	}
}

func TestRunTurn_DelegatesToStartApplication(t *testing.T) {
	original := startApplication
	defer func() { startApplication = original }()

	called := 0
	startApplication = func(ctx context.Context, opts application.StartOptions) (*application.Application, error) {
		called++
		if opts.Mode != "turn" {
			t.Fatalf("expected turn mode, got %q", opts.Mode)
		}
		return application.StartApplication(ctx, application.StartOptions{
			Mode: opts.Mode,
			Hooks: application.Hooks{
				Run: func(context.Context) error { return nil },
			},
		})
	}

	err := run(context.Background(), io.Discard, io.Discard, &fakeRegisterClient{}, &fakeDialer{}, &fakeTmux{})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected startApplication called once, got %d", called)
	}
}
