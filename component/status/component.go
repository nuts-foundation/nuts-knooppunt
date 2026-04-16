package status

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/nuts-foundation/nuts-knooppunt/component"
)

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	ready atomic.Bool
}

// New creates an instance of the status component, which provides a simple health check endpoint.
func New() *Component {
	return &Component{}
}

func (c *Component) Start() error {
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.ready.Store(false)
	return nil
}

// SetReady marks the system as ready, causing /status to return 200 OK.
// Should be called after all components have been started, so readiness probes
// don't report the system as up before its dependencies (e.g. PDP's OPA
// service) are initialized.
func (c *Component) SetReady() {
	c.ready.Store(true)
}

func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if !c.ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("starting"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	internalMux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(BuildInfo()))
	})
}
