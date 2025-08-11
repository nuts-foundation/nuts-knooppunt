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
)

var _ component.Lifecycle = (*Component)(nil)

func New() *Component {
	return &Component{}
}

type Component struct {
	ctx    context.Context
	cancel context.CancelFunc
	system *core.System
}

func (c *Component) Start() error {
	os.Setenv("NUTS_CONFIGFILE", "config.yaml")
	defer os.Unsetenv("NUTS_CONFIGFILE")

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

func (c Component) RegisterHttpHandlers(mux *http.ServeMux) {

}
