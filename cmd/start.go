package cmd

import (
	"context"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	httpComponent "github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin"
	"github.com/nuts-foundation/nuts-knooppunt/component/nutsnode"
	"github.com/nuts-foundation/nuts-knooppunt/component/status"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Start(ctx context.Context, config Config) error {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	zerolog.DefaultContextLogger = &log.Logger

	publicMux := http.NewServeMux()
	internalMux := http.NewServeMux()
	components := []component.Lifecycle{
		mcsd.New(config.MCSD),
		mcsdadmin.New(config.MCSDAdmin),
		status.New(),
		httpComponent.New(publicMux, internalMux),
	}

	if config.Nuts.Enabled {
		nutsNode, err := nutsnode.New(config.Nuts)
		if err != nil {
			return errors.Wrap(err, "failed to create nuts node component")
		}
		components = append(components, nutsNode)
	}

	// Components: RegisterHandlers()
	for _, cmp := range components {
		cmp.RegisterHttpHandlers(publicMux, internalMux)
	}

	// Components: Start()
	for _, cmp := range components {
		log.Trace().Msgf("Starting component: %T", cmp)
		if err := cmp.Start(); err != nil {
			return errors.Wrapf(err, "failed to start component: %T", cmp)
		}
		log.Trace().Msgf("Component started: %T", cmp)
	}

	log.Debug().Msgf("System started, waiting for shutdown...")
	<-ctx.Done()

	// Components: Stop()
	log.Trace().Msgf("Shutdown signalled, stopping components...")
	for _, cmp := range components {
		log.Trace().Msgf("Stopping component: %T", cmp)
		if err := cmp.Stop(ctx); err != nil {
			log.Error().Err(err).Msgf("Error stopping component: %T", cmp)
		}
		log.Trace().Msgf("Component stopped: %T", cmp)
	}
	log.Info().Msg("Goodbye!")
	return nil
}
