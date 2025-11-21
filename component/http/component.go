package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

var _ component.Lifecycle = (*Component)(nil)
var _ InterfaceInfo = (*Component)(nil)

type InterfaceInfo interface {
	Public() InterfaceConfig
	Internal() InterfaceConfig
}

type Config struct {
	InternalInterface InterfaceConfig `koanf:"internal"`
	PublicInterface   InterfaceConfig `koanf:"public"`
}

type InterfaceConfig struct {
	Address string `koanf:"address"`
	BaseURL string `koanf:"url"`
}

func (c InterfaceConfig) URL() *url.URL {
	u := c.BaseURL
	if u == "" {
		if strings.HasPrefix(c.Address, ":") {
			// E.g. :8080
			hostname, err := os.Hostname()
			if err != nil {
				log.Warn().Err(err).Msg("Failed to get hostname, defaulting to localhost")
				hostname = "localhost"
			}
			u = "http://" + hostname + c.Address
		} else {
			// E.g. localhost:8080
			u = "http://" + c.Address
		}
	}
	result, _ := url.Parse(u)
	return result
}

func DefaultConfig() Config {
	return Config{
		InternalInterface: InterfaceConfig{
			Address: ":8081",
		},
		PublicInterface: InterfaceConfig{
			Address: ":8080",
		},
	}
}

type Component struct {
	publicMux      *http.ServeMux
	publicServer   *http.Server
	internalMux    *http.ServeMux
	internalServer *http.Server
	config         Config
}

// New creates an instance of the HTTP component, which handles the HTTP interfaces for the application.
func New(config Config, publicMux *http.ServeMux, internalMux *http.ServeMux) *Component {
	return &Component{
		config:      config,
		publicMux:   publicMux,
		internalMux: internalMux,
	}
}

func (c *Component) Start() error {
	publicAddr := c.config.PublicInterface.Address
	internalAddr := c.config.InternalInterface.Address
	log.Info().Msgf("Starting HTTP servers (public-address: %s, internal-address: %s)", publicAddr, internalAddr)

	// Wrap muxes with OpenTelemetry instrumentation for automatic span creation
	publicHandler := otelhttp.NewHandler(c.publicMux, "public-api")
	internalHandler := otelhttp.NewHandler(c.internalMux, "internal-api")

	var err error
	c.publicServer, err = createServer(publicAddr, publicHandler)
	if err != nil {
		return fmt.Errorf("create public HTTP server: %w", err)
	}
	c.internalServer, err = createServer(internalAddr, internalHandler)
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

func (c *Component) Public() InterfaceConfig {
	return c.config.PublicInterface
}

func (c *Component) Internal() InterfaceConfig {
	return c.config.InternalInterface
}

func (c *Component) RegisterHttpHandlers(_ *http.ServeMux, _ *http.ServeMux) {
	// Nothing to do here
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
