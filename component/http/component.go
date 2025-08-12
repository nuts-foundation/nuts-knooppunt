package http

import (
	"context"
	"errors"
	"fmt"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/rs/zerolog/log"
	"net"
	"net/http"
)

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	publicMux      *http.ServeMux
	publicServer   *http.Server
	publicAddr     string
	internalMux    *http.ServeMux
	internalServer *http.Server
	internalAddr   string
}

// New creates an instance of the HTTP component, which handles the HTTP interfaces for the application.
func New(publicMux *http.ServeMux, internalMux *http.ServeMux) *Component {
	return &Component{
		publicMux:    publicMux,
		publicAddr:   ":8080", // Default public address
		internalMux:  internalMux,
		internalAddr: ":8081", // Default internal address
	}
}

func (c *Component) Start() error {
	log.Info().Msgf("Starting HTTP servers (public-address: %s, internal-address: %s)", c.publicAddr, c.internalAddr)
	var err error
	c.publicServer, err = createServer(c.publicAddr, c.publicMux)
	if err != nil {
		return fmt.Errorf("create public HTTP server: %w", err)
	}
	c.internalServer, err = createServer(c.internalAddr, c.internalMux)
	if err != nil {
		return fmt.Errorf("create internal HTTP server: %w", err)
	}
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	if c.publicServer != nil {
		if err := c.publicServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown public HTTP server: %w", err)
		}
	}
	if c.internalServer != nil {
		if err := c.internalServer.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown internal HTTP server: %w", err)
		}
	}
	return nil
}

func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, _ *http.ServeMux) {
	// Nothing to do here
	publicMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello, World!"))
	})
}

func createServer(addr string, handler http.Handler) (*http.Server, error) {
	server := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		if err := server.Serve(listener); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Err(err).Msgf("Failed to start HTTP server (address: %s)", addr)
			}
		}
	}()
	return server, nil
}
