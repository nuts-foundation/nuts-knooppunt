package nutsnode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strconv"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/lib/netutil"
	"github.com/nuts-foundation/nuts-node/cmd"
	"github.com/nuts-foundation/nuts-node/core"
	httpEngine "github.com/nuts-foundation/nuts-node/http"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

var _ component.Lifecycle = (*Component)(nil)

func New() (*Component, error) {
	// Nuts node uses logrus, register a hook to convert logrus logs to zerolog.
	logrus.AddHook(&logrusZerologBridgeHook{})
	// set nil logger to avoid logrus output
	logrus.StandardLogger().SetOutput(&devNullWriter{})

	internalAddr, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(netutil.FreeTCPPort()))
	if err != nil {
		return nil, fmt.Errorf("parse internal address: %w", err)
	}
	publicAddr, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(netutil.FreeTCPPort()))
	if err != nil {
		return nil, fmt.Errorf("parse public address: %w", err)
	}
	return &Component{
		internalAddr: internalAddr,
		publicAddr:   publicAddr,
	}, nil
}

type Component struct {
	ctx          context.Context
	cancel       context.CancelFunc
	system       *core.System
	internalAddr *url.URL
	publicAddr   *url.URL
}

func (c *Component) Start() error {
	configDir := os.Getenv("KNPT_CONFIGDIR")
	if configDir == "" {
		configDir = "config"
	}
	const dataDir = "data/nuts"
	envVars := map[string]string{
		"NUTS_CONFIGFILE":            path.Join(configDir, "nuts.yaml"),
		"NUTS_HTTP_INTERNAL_ADDRESS": c.internalAddr.Host,
		"NUTS_HTTP_PUBLIC_ADDRESS":   c.publicAddr.Host,
		"NUTS_DATADIR":               dataDir,
		"NUTS_VERBOSITY":             zerolog.GlobalLevel().String(),
	}
	for key, value := range envVars {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set environment variable %s: %w", key, err)
		}
	}

	log.Debug().Msgf("Starting Nuts node (internal-address: %s, public-address: %s)", c.internalAddr, c.publicAddr)

	c.system = cmd.CreateSystem(func() {
		// Not sure how to handle this
		log.Warn().Msg("Nuts node signaled exit.")
	})
	if err := c.system.Load(&pflag.FlagSet{}); err != nil {
		return fmt.Errorf("load Nuts node config: %w", err)
	}

	if err := c.system.Configure(); err != nil {
		return err
	}
	if err := c.system.Migrate(); err != nil {
		return err
	}
	if engine, ok := c.system.FindEngineByName("http").(*httpEngine.Engine); ok {
		for _, r := range c.system.Routers {
			r.Routes(engine.Router())
		}
	}
	if err := c.system.Start(); err != nil {
		return err
	}
	return nil
}

func (c Component) Stop(_ context.Context) error {
	return c.system.Shutdown()
}

func (c Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	const componentHTTPBasePath = "/nuts"
	publicProxy := createProxy(c.publicAddr, RemovePrefixRewriter(componentHTTPBasePath))
	publicMux.Handle("/nuts/{rest...}", publicProxy)
	// Nuts uses compliant well-known paths, e.g.:
	//  /.well-known/oauth-authorization-server/nuts/oauth2/<subject>
	// has to be rewritten to:
	//  /.well-known/oauth-authorization-server/oauth2/<subject>
	wellKnownProxy := createProxy(c.publicAddr, func(request *httputil.ProxyRequest) {
		request.Out.URL.Path = "/.well-known/" + request.In.PathValue("identifier") + "/" + request.In.PathValue("rest")
	})
	publicMux.Handle("/.well-known/{identifier}/nuts/{rest...}", wellKnownProxy)
	internalProxy := createProxy(c.internalAddr, RemovePrefixRewriter(componentHTTPBasePath))
	internalMux.Handle("/nuts/{rest...}", internalProxy)
}
