package main

import (
	"context"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	httpComponent "github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/nuts-foundation/nuts-knooppunt/component/nutsnode"
	"github.com/nuts-foundation/nuts-knooppunt/component/status"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	zerolog.DefaultContextLogger = &log.Logger

	publicMux := http.NewServeMux()
	internalMux := http.NewServeMux()
	components := []component.Lifecycle{
		status.New(),
		httpComponent.New(publicMux, internalMux),
		nutsnode.New(),
	}

	// Components: RegisterHandlers()
	for _, cmp := range components {
		cmp.RegisterHttpHandlers(publicMux, internalMux)
	}

	// Components: Start()
	for _, cmp := range components {
		log.Trace().Msgf("Starting component: %T", cmp)
		if err := cmp.Start(); err != nil {
			panic(errors.Wrapf(err, "failed to start component: %T", cmp))
		}
		log.Trace().Msgf("Component started: %T", cmp)
	}

	// Listen for interrupt signals (CTRL/CMD+C, OS instructing the process to stop) to cancel context.
	ctx := context.Background()
	ctx, cancelFunc := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancelFunc()

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
}
