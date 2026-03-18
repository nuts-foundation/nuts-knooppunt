package pdp

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureHandler is a slog.Handler that records all log messages for inspection.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	msgs := make([]string, len(h.records))
	for i, r := range h.records {
		msgs[i] = r.Message
	}
	return msgs
}

func TestOPALogsGoThroughSlog(t *testing.T) {
	// Set up a capturing slog handler and bridge logrus to it
	handler := &captureHandler{}
	originalDefault := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(originalDefault)
	logging.InitLogrus()

	// Start a test HTTP server to serve bundles
	mux := http.NewServeMux()
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	service, err := New(Config{Enabled: true}, nil)
	require.NoError(t, err)
	service.opaBundleBaseURL = httpServer.URL + "/pdp/bundles/"
	service.RegisterHttpHandlers(nil, mux)

	// Start OPA - this triggers bundle loading which produces log messages
	require.NoError(t, service.Start())
	defer func() {
		require.NoError(t, service.Stop(context.Background()))
	}()

	// OPA should have logged "Starting bundle loader." and "Bundle loaded and activated successfully."
	// messages through slog, not directly to stdout via logrus.
	msgs := handler.messages()
	hasBundleLog := false
	for _, msg := range msgs {
		if msg == "Starting bundle loader." || msg == "Bundle loaded and activated successfully." {
			hasBundleLog = true
			break
		}
	}
	assert.True(t, hasBundleLog,
		"expected OPA log messages to go through slog, but none were captured. Got messages: %v", msgs)
}
