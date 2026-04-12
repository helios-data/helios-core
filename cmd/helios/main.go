package main

import (
	"context"
	"fmt"
	"helios/internal/logger"
	"helios/internal/server"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"
)

func main() {
	root := &cli.Command{
		Name: "helios",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "address",
				Usage:   "address to listen on",
				Value:   "0.0.0.0:5000",
				Aliases: []string{"a"},
			},
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

	err := root.Run(context.Background(), os.Args)
	if err != nil {
		logger.Errorw("Error running Helios", "error", err)
		os.Exit(1)
	}
}

func runHelios(ctx context.Context, cmd *cli.Command) error {
	// Initialize logger
	logger.MustInit(cmd.String("log-level"), os.Stderr)
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting Helios")
	logger.Debugw("Arguments", "args", os.Args)

	// Cancel on Ctrl-C (SIGINT) or SIGTERM so work tied to this context stops cleanly.
	shutdownCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// coreClient := core.Initialize(runType, cmd.String("runtime-hash"))
	// defer coreClient.Close()

	// Start the Helios tcp server
	address := cmd.String("address")
	srv, err := server.StartServer(shutdownCtx, address)
	if err != nil {
		logger.Errorw("Error starting server", "error", err)
		return fmt.Errorf("error starting server: %w", err)
	}
	defer func() { _ = srv.Close() }()

	// coreClient.InitializeComponentTree(cmd.String("component-tree"))
	// go coreClient.StartAllComponents()

	logger.Info("Running...")

	<-shutdownCtx.Done()
	logger.Info("shutting down")
	return nil
}
