package nutsnode

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"

	"log/slog"

	knooppuntCore "github.com/nuts-foundation/nuts-knooppunt/cmd/core"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/lib/netutil"
	"github.com/nuts-foundation/nuts-node/cmd"
	"github.com/nuts-foundation/nuts-node/core"
	httpEngine "github.com/nuts-foundation/nuts-node/http"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
)

var _ component.Lifecycle = (*Component)(nil)

func New(config Config) (*Component, error) {
	// Nuts node uses logrus, register a hook to convert logrus logs to slog.
	logrus.AddHook(&logrusSlogBridgeHook{})
	// set nil logger to avoid logrus output
	logrus.StandardLogger().SetOutput(&devNullWriter{})

	internalPort, err := netutil.FreeTCPPort()
	if err != nil {
		return nil, err
	}
	internalAddr, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(internalPort))
	if err != nil {
		return nil, fmt.Errorf("parse internal address: %w", err)
	}
	publicPort, err := netutil.FreeTCPPort()
	if err != nil {
		return nil, err
	}
	publicAddr, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(publicPort))
	if err != nil {
		return nil, fmt.Errorf("parse public address: %w", err)
	}
	return &Component{
		config:       config,
		internalAddr: internalAddr,
		publicAddr:   publicAddr,
	}, nil
}

type Component struct {
	config       Config
	coreConfig   knooppuntCore.Config
	ctx          context.Context
	cancel       context.CancelFunc
	system       *core.System
	internalAddr *url.URL
	publicAddr   *url.URL
}

type Config struct {
	Enabled bool `koanf:"enabled"`
}

func (c *Component) Start() error {
	const dataDir = "data/nuts"
	const configFile = "config/nuts.yml"
	envVars := map[string]string{
		"NUTS_HTTP_INTERNAL_ADDRESS": c.internalAddr.Host,
		"NUTS_HTTP_PUBLIC_ADDRESS":   c.publicAddr.Host,
		"NUTS_DATADIR":               dataDir,
		"NUTS_VERBOSITY":             GetLogrusLevel(slog.LevelDebug), // TODO: use configured log level when supported
		"NUTS_STRICTMODE":            strconv.FormatBool(c.coreConfig.StrictMode),
	}
	// Only set NUTS_CONFIGFILE if the config file exists
	if _, err := os.Stat(configFile); err == nil {
		envVars["NUTS_CONFIGFILE"] = configFile
	}
	for key, value := range envVars {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set environment variable %s: %w", key, err)
		}
	}

	slog.Debug("Starting Nuts node", slog.String("internal-address", c.internalAddr.String()), slog.String("public-address", c.publicAddr.String()))

	c.system = cmd.CreateSystem(func() {
		// Not sure how to handle this
		slog.Warn("Nuts node signaled exit.")
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
