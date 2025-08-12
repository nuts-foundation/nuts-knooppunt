package status

import (
	"context"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"net/http"
)

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
}

// New creates an instance of the status component, which provides a simple health check endpoint.
func New() *Component {
	return &Component{}
}

func (c Component) Start() error {
	// Nothing to do
	return nil
}

func (c Component) Stop(ctx context.Context) error {
	// Nothing to do
	return nil
}

func (c Component) RegisterHttpHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}
