package authn

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tlsutil"
)

var _ component.Lifecycle = (*Component)(nil)

type Config struct {
	MinVWS MinistryAuthConfig `koanf:"minvws"`
}

// Component handles authentication with external services (MinVWS).
type Component struct {
	config Config
	// tlsConfig holds the MinVWS client certificate, loaded on Start. It is nil if no certificate is configured.
	tlsConfig *tls.Config
	// transport is shared by all MinVWS clients and their token requests, so connections are reused.
	transport *http.Transport

	mux sync.Mutex
	// clients caches the MinVWS HTTP clients by clientCacheKey, so access tokens are reused until they expire.
	clients map[string]*http.Client
}

func (c *Component) Start() error {
	if c.config.MinVWS.TLSCertFile != "" {
		tlsConfig, err := tlsutil.CreateTLSConfig(c.config.MinVWS.Config)
		if err != nil {
			return fmt.Errorf("MinVWS TLS is configured but failed to load: %w", err)
		}
		c.tlsConfig = tlsConfig
	}
	c.transport = &http.Transport{
		TLSClientConfig: c.tlsConfig,
	}
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
	return nil
}

func (c *Component) RegisterHttpHandlers(_ *http.ServeMux, _ *http.ServeMux) {
}

func New(config Config) *Component {
	return &Component{
		config:  config,
		clients: make(map[string]*http.Client),
	}
}
