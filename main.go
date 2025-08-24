package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
)

func main() {
	// Listen for interrupt signals (CTRL/CMD+C, OS instructing the process to stop) to cancel context.
	ctx, cancelFunc := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelFunc()
	if err := cmd.Start(ctx, cmd.DefaultConfig()); err != nil {
		panic(err)
	}
}
