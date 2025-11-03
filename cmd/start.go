package cmd

import (
	"context"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	httpComponent "github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/nutsnode"
	"github.com/nuts-foundation/nuts-knooppunt/component/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
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
	mcsdUpdateClient, err := mcsd.New(config.MCSD)
	if err != nil {
		return errors.Wrap(err, "failed to create mCSD Update Client")
	}
	components := []component.Lifecycle{
		mcsdUpdateClient,
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
	} else {
		log.Ctx(ctx).Info().Msg("Nuts node is disabled")
	}

	// Create MITZ component
	if config.MITZ.Enabled() {
		mitzComponent, err := mitz.New(config.MITZ)
		if err != nil {
			return errors.Wrap(err, "failed to create MITZ component")
		}
		components = append(components, mitzComponent)

		// Create PDP component
		if config.PDP.Enabled {
			pdpComponent, err := pdp.New(config.PDP, mitzComponent)
			if err != nil {
				return errors.Wrap(err, "failed to create PDP component")
			}
			components = append(components, pdpComponent)
		}

	} else {
		log.Ctx(ctx).Info().Msg("MITZ component is disabled")
	}

	// Create NVI component
	if config.NVI.Enabled() {
		nviComponent, err := nvi.New(config.NVI)
		if err != nil {
			return errors.Wrap(err, "failed to create NVI component")
		}
		components = append(components, nviComponent)
	} else {
		log.Ctx(ctx).Info().Msg("NVI component is disabled")
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
