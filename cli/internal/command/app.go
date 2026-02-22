package command

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	"shellman/cli/internal/config"
)

type Deps struct {
	LoadConfig   func() config.Config
	RunLocalMode func(context.Context, config.Config) error
	RunTurnMode  func(context.Context, config.Config) error
	RunMigrateUp func(context.Context, config.Config) error
}

func BuildApp(deps Deps) *cli.App {
	return &cli.App{
		Name:  "shellman",
		Usage: "tmux runtime bridge",
		Action: func(ctx *cli.Context) error {
			cfg := loadConfig(deps)
			return runServe(ctx.Context, deps, cfg, ctx)
		},
		Commands: []*cli.Command{
			{
				Name:  "serve",
				Usage: "start runtime",
				Flags: serveFlags(),
				Action: func(ctx *cli.Context) error {
					cfg := loadConfig(deps)
					return runServe(ctx.Context, deps, cfg, ctx)
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

func serveFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "host",
			Usage: "local listen host",
		},
		&cli.IntFlag{
			Name:  "port",
			Usage: "local listen port",
		},
		&cli.StringFlag{
			Name:  "config-dir",
			Usage: "shellman config directory",
		},
		&cli.StringFlag{
			Name:  "tmux-socket",
			Usage: "tmux socket path",
		},
		&cli.StringFlag{
			Name:  "webui-dist-dir",
			Usage: "web ui dist directory",
		},
	}
}

func runServe(ctx context.Context, deps Deps, cfg config.Config, cliCtx *cli.Context) error {
	if cliCtx != nil && cliCtx.Args().Len() > 0 {
		return fmt.Errorf("unexpected argument: %s", cliCtx.Args().First())
	}
	cfg = applyServeFlagOverrides(cliCtx, cfg)
	return runLocalMode(ctx, deps, cfg)
}

func applyServeFlagOverrides(cliCtx *cli.Context, cfg config.Config) config.Config {
	if cliCtx == nil {
		return cfg
	}

	if cliCtx.IsSet("host") {
		cfg.LocalHost = strings.TrimSpace(cliCtx.String("host"))
	}
	if cliCtx.IsSet("port") {
		cfg.LocalPort = cliCtx.Int("port")
	}
	if cliCtx.IsSet("tmux-socket") {
		cfg.TmuxSocket = strings.TrimSpace(cliCtx.String("tmux-socket"))
	}
	if cliCtx.IsSet("webui-dist-dir") {
		cfg.WebUIDistDir = strings.TrimSpace(cliCtx.String("webui-dist-dir"))
	}
	if cliCtx.IsSet("config-dir") {
		_ = os.Setenv("SHELLMAN_CONFIG_DIR", strings.TrimSpace(cliCtx.String("config-dir")))
	}

	return cfg
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
