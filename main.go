package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
)

func main() {
	logging.Init()

	// Load configuration
	config, err := cmd.LoadConfig()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Listen for interrupt signals (CTRL/CMD+C, OS instructing the process to stop) to cancel context.
	ctx, cancelFunc := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelFunc()
	if err := cmd.Start(ctx, config); err != nil {
		panic(err)
	}
}
