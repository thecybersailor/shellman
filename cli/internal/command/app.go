package command

import (
	"context"
	"errors"
	"strings"

	"github.com/urfave/cli/v2"

	"termteam/cli/internal/config"
)

type Deps struct {
	LoadConfig   func() config.Config
	RunLocalMode func(context.Context, config.Config) error
	RunTurnMode  func(context.Context, config.Config) error
	RunMigrateUp func(context.Context, config.Config) error
}

func BuildApp(deps Deps) *cli.App {
	return &cli.App{
		Name:  "termteam",
		Usage: "tmux runtime bridge",
		Action: func(ctx *cli.Context) error {
			cfg := loadConfig(deps)
			return runServeByConfig(ctx.Context, deps, cfg)
		},
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "start runtime",
				Action: func(ctx *cli.Context) error {
					cfg := loadConfig(deps)
					return runServeByConfig(ctx.Context, deps, cfg)
				},
				Subcommands: []*cli.Command{
					{
						Name:  "local",
						Usage: "start local runtime",
						Action: func(ctx *cli.Context) error {
							cfg := loadConfig(deps)
							cfg.Mode = "local"
							return runLocalMode(ctx.Context, deps, cfg)
						},
					},
					{
						Name:  "turn",
						Usage: "start turn runtime",
						Action: func(ctx *cli.Context) error {
							cfg := loadConfig(deps)
							cfg.Mode = "turn"
							return runTurnMode(ctx.Context, deps, cfg)
						},
					},
				},
			},
			{
				Name:  "migrate",
				Usage: "run database migration",
				Subcommands: []*cli.Command{
					{
						Name:  "up",
						Usage: "apply pending migrations",
						Action: func(ctx *cli.Context) error {
							cfg := loadConfig(deps)
							return runMigrateUp(ctx.Context, deps, cfg)
						},
					},
				},
			},
		},
	}
}

func loadConfig(deps Deps) config.Config {
	if deps.LoadConfig != nil {
		return deps.LoadConfig()
	}
	return config.LoadConfig()
}

func runServeByConfig(ctx context.Context, deps Deps, cfg config.Config) error {
	mode := strings.TrimSpace(strings.ToLower(cfg.Mode))
	if mode == "turn" {
		return runTurnMode(ctx, deps, cfg)
	}
	return runLocalMode(ctx, deps, cfg)
}

func runLocalMode(ctx context.Context, deps Deps, cfg config.Config) error {
	if deps.RunLocalMode == nil {
		return errors.New("local mode runner is not configured")
	}
	return deps.RunLocalMode(ctx, cfg)
}

func runTurnMode(ctx context.Context, deps Deps, cfg config.Config) error {
	if deps.RunTurnMode == nil {
		return errors.New("turn mode runner is not configured")
	}
	return deps.RunTurnMode(ctx, cfg)
}

func runMigrateUp(ctx context.Context, deps Deps, cfg config.Config) error {
	if deps.RunMigrateUp == nil {
		return errors.New("migrate up runner is not configured")
	}
	return deps.RunMigrateUp(ctx, cfg)
}
