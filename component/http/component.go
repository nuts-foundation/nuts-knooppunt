package http

import (
	"context"
	"errors"
	"fmt"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/rs/zerolog/log"
	"net/http"
)

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	publicMux      *http.ServeMux
	publicServer   *http.Server
	internalMux    *http.ServeMux
	internalServer *http.Server
}

// New creates an instance of the HTTP component, which handles the HTTP interfaces for the application.
func New(publicMux *http.ServeMux, internalMux *http.ServeMux) *Component {
	return &Component{
		publicMux:   publicMux,
		internalMux: internalMux,
	}
}

func (c *Component) Start() error {
	c.publicServer = &http.Server{
		Addr:    ":8080",
		Handler: c.publicMux,
	}
	c.internalServer = &http.Server{
		Addr:    ":8081",
		Handler: c.internalMux,
	}
	log.Info().Msgf("Starting HTTP servers (public-address: %s, internal-address: %s)", c.publicServer.Addr, c.internalServer.Addr)
	go func() {
		if err := c.publicServer.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Err(err).Msg("Failed to start public HTTP server")
			}
		}
	}()
	go func() {
		if err := c.internalServer.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Err(err).Msg("Failed to start internal HTTP server")
			}
		}
	}()
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	if err := c.publicServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown public HTTP server: %w", err)
	}
	if err := c.internalServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown internal HTTP server: %w", err)
	}
	return nil
}

func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, _ *http.ServeMux) {
	// Nothing to do here
	publicMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello, World!"))
	})
}
