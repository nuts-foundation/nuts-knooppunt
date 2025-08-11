package nuts

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/internal/subsystems"
	"github.com/rs/zerolog/log"
)

// Subsystem implements the Nuts node subsystem
type Subsystem struct {
	publicServer   *http.Server
	internalServer *http.Server
	publicAddr     string
	internalAddr   string
}

// NewSubsystem creates a new Nuts node subsystem
func NewSubsystem() *Subsystem {
	return &Subsystem{
		publicAddr:   "127.0.0.1:8280", // Use different ports to avoid conflicts
		internalAddr: "127.0.0.1:8281",
	}
}

// Name returns the name of the subsystem
func (s *Subsystem) Name() string {
	return "nuts-node"
}

// RoutePrefix returns the prefix under which this subsystem should be mounted
func (s *Subsystem) RoutePrefix() string {
	return "/nuts"
}

// PublicAddress returns the address where the public interface is listening
func (s *Subsystem) PublicAddress() string {
	return fmt.Sprintf("http://%s", s.publicAddr)
}

// InternalAddress returns the address where the internal interface is listening
func (s *Subsystem) InternalAddress() string {
	return fmt.Sprintf("http://%s", s.internalAddr)
}

// Start starts the Nuts node subsystem
func (s *Subsystem) Start(ctx context.Context) error {
	log.Info().Msg("Starting Nuts node subsystem")

	// Create public server
	publicMux := http.NewServeMux()
	publicMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message": "Nuts node public API", "path": "` + r.URL.Path + `"}`))
	})
	publicMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status": "ok", "service": "nuts-node-public"}`))
	})

	s.publicServer = &http.Server{
		Addr:    s.publicAddr,
		Handler: publicMux,
	}

	// Create internal server
	internalMux := http.NewServeMux()
	internalMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message": "Nuts node internal API", "path": "` + r.URL.Path + `"}`))
	})
	internalMux.HandleFunc("/internal/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status": "ok", "service": "nuts-node-internal"}`))
	})

	s.internalServer = &http.Server{
		Addr:    s.internalAddr,
		Handler: internalMux,
	}

	// Start public server
	go func() {
		log.Info().Str("addr", s.publicAddr).Msg("Starting Nuts node public server")
		if err := s.publicServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Nuts node public server failed")
		}
	}()

	// Start internal server
	go func() {
		log.Info().Str("addr", s.internalAddr).Msg("Starting Nuts node internal server")
		if err := s.internalServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Nuts node internal server failed")
		}
	}()

	// Wait a moment for servers to start
	time.Sleep(100 * time.Millisecond)

	log.Info().
		Str("public", s.PublicAddress()).
		Str("internal", s.InternalAddress()).
		Msg("Nuts node subsystem started")

	return nil
}

// Stop stops the Nuts node subsystem
func (s *Subsystem) Stop(ctx context.Context) error {
	log.Info().Msg("Stopping Nuts node subsystem")

	if s.publicServer != nil {
		if err := s.publicServer.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("Error shutting down Nuts node public server")
		}
	}

	if s.internalServer != nil {
		if err := s.internalServer.Shutdown(ctx); err != nil {
			log.Error().Err(err).Msg("Error shutting down Nuts node internal server")
		}
	}

	log.Info().Msg("Nuts node subsystem stopped")
	return nil
}

// Verify implementation
var _ subsystems.Subsystem = (*Subsystem)(nil)