package subsystems

import (
	"context"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/internal/subsystems"
	"github.com/nuts-foundation/nuts-knooppunt/internal/subsystems/nuts"
	"github.com/rs/zerolog/log"
)

var _ component.Lifecycle = (*Component)(nil)

// Component manages subsystems as a component in the main application
type Component struct {
	manager *subsystems.Manager
}

// New creates a new subsystems component
func New() *Component {
	return &Component{
		manager: subsystems.NewManager(),
	}
}

func (c *Component) Start() error {
	log.Info().Msg("Starting subsystems component")
	
	// Register Nuts node subsystem
	nutsSubsystem := nuts.NewSubsystem()
	if err := c.manager.Register(nutsSubsystem); err != nil {
		log.Error().Err(err).Msg("Failed to register Nuts subsystem")
		return err
	}

	// Start all subsystems
	ctx := context.Background()
	if err := c.manager.Start(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to start subsystems")
		return err
	}

	log.Info().Msg("All subsystems started successfully")
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	log.Info().Msg("Stopping subsystems component")
	
	if err := c.manager.Stop(ctx); err != nil {
		log.Error().Err(err).Msg("Error stopping subsystems")
		return err
	}
	
	log.Info().Msg("All subsystems stopped successfully")
	return nil
}

func (c *Component) RegisterHttpHandlers(mux *http.ServeMux) {
	log.Info().Msg("Registering subsystem HTTP handlers")
	
	// Mount subsystem handlers with proxy
	subsystemHandler := c.manager.CreateHandler()
	mux.Handle("/nuts/", subsystemHandler)
	
	log.Info().Msg("Subsystem HTTP handlers registered")
}