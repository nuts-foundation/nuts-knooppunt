package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/authn"
	libHTTPComponent "github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/nutsnode"
	"github.com/nuts-foundation/nuts-knooppunt/component/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
	"github.com/nuts-foundation/nuts-knooppunt/component/status"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/pkg/errors"
)

func Start(ctx context.Context, config Config) error {
	if !config.StrictMode {
		slog.WarnContext(ctx, "Strict mode is disabled. This is NOT recommended for production environments!")
	}

	publicMux := http.NewServeMux()
	internalMux := http.NewServeMux()

	// Tracing component must be started first to capture logs and spans from other components.
	// We start it immediately (not in the component loop) so that logs from other component
	// constructors (New functions) are also captured via OTLP.
	config.Tracing.ServiceVersion = status.Version()
	tracingComponent := tracing.New(config.Tracing)
	if err := tracingComponent.Start(); err != nil {
		return errors.Wrap(err, "failed to start tracing component")
	}

	mcsdUpdateClient, err := mcsd.New(config.MCSD)
	if err != nil {
		return errors.Wrap(err, "failed to create mCSD Update Client")
	}
	httpComponent := libHTTPComponent.New(config.HTTP, publicMux, internalMux)
	components := []component.Lifecycle{
		mcsdUpdateClient,
		mcsdadmin.New(config.MCSDAdmin),
		status.New(),
		httpComponent,
	}

	if config.Nuts.Enabled {
		// Pass tracing config to nuts-node so it can create its own TracerProvider
		config.Nuts.TracingConfig = nutsnode.TracingConfig{
			OTLPEndpoint: config.Tracing.OTLPEndpoint,
			Insecure:     config.Tracing.Insecure,
		}
		nutsNode, err := nutsnode.New(config.Nuts)
		if err != nil {
			return errors.Wrap(err, "failed to create nuts node component")
		}
		components = append(components, nutsNode)
	} else {
		slog.InfoContext(ctx, "Nuts node is disabled")
	}

	// Create AuthN component
	authnComponent, err := authn.New(config.AuthN, httpComponent, config.Config)
	if err != nil {
		return errors.Wrap(err, "failed to create AuthN component")
	}
	components = append(components, authnComponent)

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
		slog.InfoContext(ctx, "MITZ component is disabled")
	}

	// Create NVI component
	if config.NVI.Enabled() {
		nviComponent, err := nvi.New(config.NVI)
		if err != nil {
			return errors.Wrap(err, "failed to create NVI component")
		}
		components = append(components, nviComponent)
	} else {
		slog.InfoContext(ctx, "NVI component is disabled")
	}

	// Components: RegisterHandlers()
	for _, cmp := range components {
		cmp.RegisterHttpHandlers(publicMux, internalMux)
	}

	// Components: Start()
	for _, cmp := range components {
		slog.DebugContext(ctx, "Starting component", logging.Component(cmp))
		if err := cmp.Start(); err != nil {
			return errors.Wrapf(err, "failed to start component: %T", cmp)
		}
		slog.DebugContext(ctx, "Component started", logging.Component(cmp))
	}

	slog.DebugContext(ctx, "System started, waiting for shutdown...")
	<-ctx.Done()

	// Components: Stop()
	slog.DebugContext(ctx, "Shutdown signalled, stopping components...")
	for _, cmp := range components {
		slog.DebugContext(ctx, "Stopping component", logging.Component(cmp))
		if err := cmp.Stop(ctx); err != nil {
			slog.ErrorContext(ctx, "Error stopping component", logging.Component(cmp), logging.Error(err))
		}
		slog.DebugContext(ctx, "Component stopped", logging.Component(cmp))
	}
	slog.InfoContext(ctx, "Goodbye!")

	// Stop tracing last to ensure all shutdown logs are captured
	if err := tracingComponent.Stop(ctx); err != nil {
		// Can't use slog here as the handler may already be shut down
		fmt.Printf("Error stopping tracing component: %v\n", err)
	}
	return nil
}
