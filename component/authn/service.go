package authn

import (
	"context"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component"
)

var _ component.Lifecycle = (*Component)(nil)

type Config struct {
	MinVWS MinistryAuthConfig `koanf:"minvws"`
}

// Component handles authentication with external services (MinVWS).
type Component struct {
	config Config
}

func (c *Component) Start() error {
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	return nil
}

func (c *Component) RegisterHttpHandlers(_ *http.ServeMux, _ *http.ServeMux) {
}

func New(config Config) *Component {
	return &Component{config: config}
}
