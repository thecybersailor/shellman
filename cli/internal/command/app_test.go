package command

import (
	"context"
	"os"
	"testing"

	"shellman/cli/internal/config"
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
		RunMigrateUp: func(context.Context, config.Config) error {
			migrateCalled++
			return nil
		},
	})
	if err := app.RunContext(context.Background(), []string{"shellman"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if localCalled != 1 || turnCalled != 0 || migrateCalled != 0 {
		t.Fatalf("unexpected call count local=%d turn=%d migrate=%d", localCalled, turnCalled, migrateCalled)
	}
}

func TestBuildApp_ServeCommand_AlwaysRunsLocal(t *testing.T) {
	localCalled := 0
	turnCalled := 0
	app := BuildApp(Deps{
		LoadConfig: func() config.Config {
			return config.Config{Mode: "turn"}
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
	if err := app.RunContext(context.Background(), []string{"shellman", "serve"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if localCalled != 1 || turnCalled != 0 {
		t.Fatalf("unexpected call count local=%d turn=%d", localCalled, turnCalled)
	}
}

func TestBuildApp_ServeFlags_OverrideConfig(t *testing.T) {
	var got config.Config
	app := BuildApp(Deps{
		LoadConfig: func() config.Config {
			return config.Config{
				LocalHost:    "127.0.0.1",
				LocalPort:    4621,
				TmuxSocket:   "",
				WebUIDistDir: "/default",
			}
		},
		RunLocalMode: func(_ context.Context, cfg config.Config) error {
			got = cfg
			return nil
		},
	})
	args := []string{
		"shellman", "serve",
		"--host", "0.0.0.0",
		"--port", "4701",
		"--tmux-socket", "/tmp/tmux.sock",
		"--webui-dist-dir", "/tmp/webui",
	}
	if err := app.RunContext(context.Background(), args); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got.LocalHost != "0.0.0.0" || got.LocalPort != 4701 || got.TmuxSocket != "/tmp/tmux.sock" || got.WebUIDistDir != "/tmp/webui" {
		t.Fatalf("override failed: %#v", got)
	}
}

func TestBuildApp_ServeFlagConfigDir_OverridesEnv(t *testing.T) {
	t.Setenv("SHELLMAN_CONFIG_DIR", "/env/dir")
	app := BuildApp(Deps{
		LoadConfig:   func() config.Config { return config.Config{} },
		RunLocalMode: func(context.Context, config.Config) error { return nil },
	})
	if err := app.RunContext(context.Background(), []string{"shellman", "serve", "--config-dir", "/flag/dir"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if got := os.Getenv("SHELLMAN_CONFIG_DIR"); got != "/flag/dir" {
		t.Fatalf("unexpected config dir env: %s", got)
	}
}

func TestBuildApp_ServeTurnSubcommandRemoved(t *testing.T) {
	app := BuildApp(Deps{
		LoadConfig:   func() config.Config { return config.Config{} },
		RunLocalMode: func(context.Context, config.Config) error { return nil },
	})
	err := app.RunContext(context.Background(), []string{"shellman", "serve", "turn"})
	if err == nil {
		t.Fatal("expected unknown command error for removed serve turn")
	}
}

func TestBuildApp_MigrateUpCommand(t *testing.T) {
	migrateCalled := 0
	app := BuildApp(Deps{
		LoadConfig: func() config.Config {
			return config.Config{Mode: "local"}
		},
		RunLocalMode: func(context.Context, config.Config) error { return nil },
		RunMigrateUp: func(context.Context, config.Config) error {
			migrateCalled++
			return nil
		},
	})
	if err := app.RunContext(context.Background(), []string{"shellman", "migrate", "up"}); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if migrateCalled != 1 {
		t.Fatalf("expected migrate command called once, got %d", migrateCalled)
	}
}
