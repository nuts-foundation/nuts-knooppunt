// Package subscription is the TTA Notifications v0.6 POC component
// (docs/tta-notifications-v06/PLAN.md): one component, two independently toggled roles.
// The Subscription Server role serves in-band creation, $status, $events and
// SubscriptionTopic discovery on the public mux and pushes handshake/heartbeat/event
// notification Bundles to partners; the Subscription Client role serves the rest-hook
// notification endpoint on the public mux and normalises whatever arrives into the
// internal inbox. The vendor-facing internal API (event ingest, authorization bases,
// intents, inbox, transport log) is wired through the generated strict server in package
// cmd, like every other internal operation.
package subscription

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/api/subscriptionpublic"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
)

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	config     Config
	store      *store
	aliaser    *aliaser
	httpClient *http.Client

	// backgroundCtx outlives request contexts: handshakes, deliveries and scheduler
	// work run on it and stop when the component stops.
	backgroundCtx context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup

	tickInterval time.Duration

	pollMu   sync.Mutex
	lastPoll string
}

func New(config Config) (*Component, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &Component{
		config:  config,
		store:   newStore(),
		aliaser: newAliaser(config.Server.AliasKey),
		httpClient: &http.Client{
			Transport: tracing.WrapTransport(nil),
			Timeout:   30 * time.Second,
		},
		backgroundCtx: ctx,
		cancel:        cancel,
		tickInterval:  defaultTickInterval,
	}, nil
}

// SetTickInterval shortens the heartbeat/miss-detection scheduler pace; used by tests.
// Must be called before Start.
func (c *Component) SetTickInterval(interval time.Duration) {
	c.tickInterval = interval
}

func (c *Component) Start() error {
	// One scheduler drives both roles: due heartbeats (server) and missed-heartbeat
	// detection (client). Kept deliberately simple — per-subscription timers would be
	// more precise but are lifecycle-heavy for a POC.
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-c.backgroundCtx.Done():
				return
			case <-ticker.C:
				c.heartbeatTick(c.backgroundCtx)
				c.heartbeatMissTick(c.backgroundCtx)
			}
		}
	}()

	if c.config.Server.Enabled && c.config.Server.FHIRBaseURL != "" && c.config.Server.PollIntervalSeconds > 0 {
		interval := time.Duration(c.config.Server.PollIntervalSeconds) * time.Second
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-c.backgroundCtx.Done():
					return
				case <-ticker.C:
					c.pollTasks(c.backgroundCtx)
				}
			}
		}()
	}
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.cancel()
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
	}
	return nil
}

// RegisterHttpHandlers mounts the public FHIR surface (generated from
// subscription-public.openapi.yaml) on the public mux. The internal vendor-facing
// operations are part of openapi.yaml and mounted by strictAPIServer in package cmd.
func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, _ *http.ServeMux) {
	handler := subscriptionpublic.NewStrictHandler(c, nil)
	subscriptionpublic.HandlerFromMux(handler, publicMux)
}
