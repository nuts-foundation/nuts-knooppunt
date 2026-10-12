package harness

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// NotifyIntake is where the harness's Knooppunt forwards Mitz notifications
// to. The Knooppunt needs the address when it starts, which is before any test
// has built the sandbox that will receive them, so the listener comes first and
// the handler is attached later.
type NotifyIntake struct {
	// URL is the address to configure as the Knooppunt's notifyforwardurl.
	URL string

	mu      sync.Mutex
	handler http.Handler
}

func startNotifyIntake(t *testing.T) *NotifyIntake {
	t.Helper()
	intake := &NotifyIntake{}
	server := httptest.NewServer(intake)
	t.Cleanup(server.Close)
	intake.URL = server.URL + "/mitz/notify"
	return intake
}

// Attach makes handler receive what is forwarded from now on. Nil detaches.
func (n *NotifyIntake) Attach(handler http.Handler) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.handler = handler
}

func (n *NotifyIntake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	handler := n.handler
	n.mu.Unlock()
	if handler == nil {
		http.Error(w, "no sandbox is attached to the notification intake", http.StatusServiceUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}
