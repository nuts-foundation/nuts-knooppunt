package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
)

func main() {
	// Load configuration
	config, err := cmd.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Listen for interrupt signals (CTRL/CMD+C, OS instructing the process to stop) to cancel context.
	ctx, cancelFunc := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelFunc()
	if err := cmd.Start(ctx, config); err != nil {
		panic(err)
	}
}
