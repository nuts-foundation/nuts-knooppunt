package status

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/component"
)

var _ component.Lifecycle = (*Component)(nil)

type Component struct {
	ready atomic.Bool
}

// New creates an instance of the status component, which provides a simple health check endpoint.
func New() *Component {
	return &Component{}
}

func (c *Component) Start() error {
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.ready.Store(false)
	return nil
}

// SetReady marks the system as ready, causing /status to return 200 OK.
// Should be called after all components have been started, so readiness probes
// don't report the system as up before its dependencies (e.g. PDP's OPA
// service) are initialized.
func (c *Component) SetReady() {
	c.ready.Store(true)
}

// Ready reports whether the system has been marked ready via SetReady.
func (c *Component) Ready() bool {
	return c.ready.Load()
}

// RegisterHttpHandlers registers no routes: /status and /version are served through the
// generated OpenAPI strict server, wired up in cmd.RegisterAPIRoutes.
func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
}

func (c *Component) GetStatus(_ context.Context, _ api.GetStatusRequestObject) (api.GetStatusResponseObject, error) {
	if !c.Ready() {
		return api.GetStatus503TextResponse("starting"), nil
	}
	return api.GetStatus200TextResponse("OK"), nil
}

func (c *Component) GetVersion(_ context.Context, _ api.GetVersionRequestObject) (api.GetVersionResponseObject, error) {
	return api.GetVersion200TextResponse(BuildInfo()), nil
}
