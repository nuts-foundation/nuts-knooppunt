package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/internal/subsystems"
	"github.com/nuts-foundation/nuts-knooppunt/internal/subsystems/nuts"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	const interfaceAddress = ":8080"
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	zerolog.DefaultContextLogger = &log.Logger
	log.Info().Msgf("Nuts Knooppunt starting on %s", interfaceAddress)

	// Create subsystem manager
	manager := subsystems.NewManager()

	// Register Nuts node subsystem
	nutsSubsystem := nuts.NewSubsystem()
	if err := manager.Register(nutsSubsystem); err != nil {
		log.Fatal().Err(err).Msg("Failed to register Nuts subsystem")
	}

	// Start all subsystems
	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to start subsystems")
	}

	// Create main HTTP handler
	mux := http.NewServeMux()

	// Knooppunt endpoints
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message": "Nuts Knooppunt", "version": "1.0.0"}`))
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status": "ok", "service": "nuts-knooppunt"}`))
	})

	// Mount subsystem handlers
	subsystemHandler := manager.CreateHandler()
	mux.Handle("/nuts/", subsystemHandler)

	// Create HTTP server
	server := &http.Server{
		Addr:    interfaceAddress,
		Handler: mux,
	}

	// Start HTTP server
	go func() {
		log.Info().Str("addr", interfaceAddress).Msg("Starting Nuts Knooppunt server")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Wait for shutdown signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	log.Info().Msg("Shutting down Nuts Knooppunt")

	// Shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Stop HTTP server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error shutting down HTTP server")
	}

	// Stop all subsystems
	if err := manager.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error stopping subsystems")
	}

	log.Info().Msg("Goodbye!")
}
