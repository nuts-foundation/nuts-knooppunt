package component

import (
	"context"
	"net/http"
)

// Lifecycle is an isolated unit of functionality that can be started and stopped.
type Lifecycle interface {
	// Start causes the component to initialize any resources it couldn't during its creation, e.g. timers.
	// It must be non-blocking.
	Start() error
	// Stop causes the component to release any resources it has acquired, e.g. timers.
	Stop(ctx context.Context) error
	// RegisterHttpHandlers registers the HTTP handlers for this component.
	RegisterHttpHandlers(mux *http.ServeMux)
}
