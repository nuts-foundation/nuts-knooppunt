package nutsnode

import (
	"context"
	"fmt"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-node/cmd"
	"github.com/nuts-foundation/nuts-node/core"
	httpEngine "github.com/nuts-foundation/nuts-node/http"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"net/http"
	"os"
	"strconv"
)

var _ component.Lifecycle = (*Component)(nil)

func New() *Component {
	return &Component{}
}

type Component struct {
	ctx          context.Context
	cancel       context.CancelFunc
	system       *core.System
	internalAddr string
	publicAddr   string
}

func (c *Component) Start() error {
	os.Setenv("NUTS_CONFIGFILE", "config.yaml")
	defer os.Unsetenv("NUTS_CONFIGFILE")

	c.internalAddr = "127.0.0.1:" + strconv.Itoa(freeTCPPort())
	c.publicAddr = "127.0.0.1:" + strconv.Itoa(freeTCPPort())
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
	publicProxy := createProxy(c.publicAddr)
	publicMux.Handle("/nuts/{rest...}", publicProxy)
	internalProxy := createProxy(c.internalAddr)
	internalMux.Handle("/nuts/{rest...}", internalProxy)
}
