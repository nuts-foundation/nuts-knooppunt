package http

import (
	"context"
	"errors"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/rs/zerolog/log"
	"net/http"
)

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	mux    *http.ServeMux
	server *http.Server
}

// New creates an instance of the HTTP component, which handles the HTTP interfaces for the application.
func New(mux *http.ServeMux) *Component {
	return &Component{
		mux: mux,
	}
}

func (c *Component) Start() error {
	const addr = ":8080"
	c.server = &http.Server{
		Addr:    addr,
		Handler: c.mux,
	}
	log.Info().Msgf("Public interface listens on %s", addr)
	go func() {
		if err := c.server.ListenAndServe(); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				log.Err(err).Msg("Failed to start server")
			}
		}
	}()
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	return c.server.Shutdown(ctx)
}

func (c *Component) RegisterHttpHandlers(mux *http.ServeMux) {
	// Nothing to do here
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello, World!"))
	})
}
