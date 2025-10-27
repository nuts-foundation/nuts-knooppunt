package pdp

import (
	"context"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component"
)

type Config struct {
	Enabled bool
}

func DefaultConfig() Config {
	return Config{
		Enabled: true,
	}
}

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	Config Config
}

// New creates an instance of the pdp component, which provides a simple policy decision endpoint.
func New(config Config) (*Component, error) {
	return &Component{
		Config: config,
	}, nil
}

func (c Component) Start() error {
	// Nothing to do
	return nil
}

func (c Component) Stop(ctx context.Context) error {
	// Nothing to do
	return nil
}

func (c Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	internalMux.HandleFunc("/pdp", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
}
