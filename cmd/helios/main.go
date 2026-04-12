package main

import (
	"context"
	"fmt"
	"helios/internal/core"
	"helios/internal/logger"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"
)

func main() {
	root := &cli.Command{
		Name: "helios",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "local",
				Aliases: []string{"l"},
				Usage:   "run components locally instead of Docker",
			},
			&cli.StringFlag{
				Name:  "component-tree",
				Usage: "path to component tree configuration",
			},
			&cli.StringFlag{
				Name:    "runtime-hash",
				Aliases: []string{"rt"},
				Usage:   "runtime identifier hash",
			},
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "logging level: error, warn, info, debug, verbose",
				Value:   "info",
				Aliases: []string{"L"},
			},
		},
		Action: runHelios,
	}

	if err := root.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "helios: %v\n", err)
		os.Exit(1)
	}
}

func runHelios(ctx context.Context, cmd *cli.Command) error {
	logger.MustInit(cmd.String("log-level"), os.Stderr)
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting Helios")
	logger.Debugw("Arguments", "args", os.Args)

	runType := "docker"
	if cmd.Bool("local") {
		runType = "local"
	}

	coreClient := core.Initialize(runType, cmd.String("runtime-hash"))
	defer coreClient.Close()

	coreClient.InitializeComponentTree(cmd.String("component-tree"))
	go coreClient.StartAllComponents()

	logger.Info("Running...")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		logger.Info("shutting down")
		return nil
	case <-ctx.Done():
		logger.Info("shutting down")
		return ctx.Err()
	}
}
