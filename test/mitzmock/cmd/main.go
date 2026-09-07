// Command mitzmock runs the mock XACML MITZ closed-question service as a
// standalone HTTP server, so docker compose can offer a Mitz consent endpoint
// without access to the real (national) Mitz.
//
// It answers Permit by default. The magic-BSN overrides of the in-process mock
// apply here too: a request body containing "bsn:deny" yields Deny, and one
// containing "bsn:error" yields HTTP 500.
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/nuts-foundation/nuts-knooppunt/test/mitzmock"
)

func main() {
	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8080"
	}

	service, err := mitzmock.New(listenAddr)
	if err != nil {
		slog.Error("failed to start mock MITZ service", "error", err)
		os.Exit(1)
	}
	slog.Info("mock MITZ closed-question service listening", "addr", listenAddr)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	sig := <-signals
	slog.Info("shutting down", "signal", sig.String())

	if err := service.Stop(); err != nil {
		slog.Error("error stopping mock MITZ service", "error", err)
		os.Exit(1)
	}
}
