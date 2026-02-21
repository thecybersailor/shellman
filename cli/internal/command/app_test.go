package command

import (
	"context"
	"testing"

	"termteam/cli/internal/config"
)

func TestBuildApp_DefaultCommandIsServeLocal(t *testing.T) {
	localCalled := 0
	turnCalled := 0
	migrateCalled := 0
	app := BuildApp(Deps{
		LoadConfig: func() config.Config {
			return config.Config{Mode: ""}
		},
		RunLocalMode: func(context.Context, config.Config) error {
			localCalled++
			return nil
		},
		RunTurnMode: func(context.Context, config.Config) error {
			turnCalled++
			return nil
		},
		RunMigrateUp: func(context.Context, config.Config) error {
			migrateCalled++
			return nil
		},
	})
	if err := app.RunContext(context.Background(), []string{"termteam"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if localCalled != 1 || turnCalled != 0 || migrateCalled != 0 {
		t.Fatalf("unexpected call count local=%d turn=%d migrate=%d", localCalled, turnCalled, migrateCalled)
	}
}

func TestBuildApp_ServeTurnCommand(t *testing.T) {
	localCalled := 0
	turnCalled := 0
	app := BuildApp(Deps{
		LoadConfig: func() config.Config {
			return config.Config{Mode: "local"}
		},
		RunLocalMode: func(context.Context, config.Config) error {
			localCalled++
			return nil
		},
		RunTurnMode: func(context.Context, config.Config) error {
			turnCalled++
			return nil
		},
	})
	if err := app.RunContext(context.Background(), []string{"termteam", "serve", "turn"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if localCalled != 0 || turnCalled != 1 {
		t.Fatalf("unexpected call count local=%d turn=%d", localCalled, turnCalled)
	}
}

func TestBuildApp_MigrateUpCommand(t *testing.T) {
	migrateCalled := 0
	app := BuildApp(Deps{
		LoadConfig: func() config.Config {
			return config.Config{Mode: "local"}
		},
		RunLocalMode: func(context.Context, config.Config) error { return nil },
		RunTurnMode:  func(context.Context, config.Config) error { return nil },
		RunMigrateUp: func(context.Context, config.Config) error {
			migrateCalled++
			return nil
		},
	})
	if err := app.RunContext(context.Background(), []string{"termteam", "migrate", "up"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if migrateCalled != 1 {
		t.Fatalf("expected migrate command called once, got %d", migrateCalled)
	}
}
