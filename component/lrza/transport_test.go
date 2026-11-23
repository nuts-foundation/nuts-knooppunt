package lrza

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newHangingServer returns a server that accepts requests and never answers them, until the test ends or
// the client gives up on the request.
func newHangingServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	// Cleanups run last-in first-out: release the handlers before Close waits for them.
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	return server
}

// setDuration sets *target to value for the duration of the test.
func setDuration(t *testing.T, target *time.Duration, value time.Duration) {
	t.Helper()
	original := *target
	*target = value
	t.Cleanup(func() { *target = original })
}

// trustingTLSConfig returns a TLS config that trusts server's test certificate.
func trustingTLSConfig(server *httptest.Server) *tls.Config {
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return &tls.Config{RootCAs: pool}
}

func TestNewTransport(t *testing.T) {
	t.Run("an attempt gives up when the server doesn't answer", func(t *testing.T) {
		setDuration(t, &responseHeaderTimeout, 50*time.Millisecond)
		server := newHangingServer(t)
		client := newHTTPClient(newTransport(nil))

		start := time.Now()
		resp, err := client.Get(server.URL)
		if resp != nil {
			_ = resp.Body.Close()
		}

		require.ErrorContains(t, err, "timeout awaiting response headers")
		assert.Less(t, time.Since(start), 5*time.Second)
	})

	t.Run("the client bounds a request as a whole", func(t *testing.T) {
		setDuration(t, &requestTimeout, 50*time.Millisecond)
		server := newHangingServer(t)
		client := newHTTPClient(newTransport(nil))

		resp, err := client.Get(server.URL)
		if resp != nil {
			_ = resp.Body.Close()
		}

		require.ErrorContains(t, err, "Client.Timeout exceeded")
	})

	t.Run("keeps the mTLS config, without changing the caller's", func(t *testing.T) {
		tlsConfig := &tls.Config{Certificates: []tls.Certificate{{}}}

		transport := newTransport(tlsConfig)

		assert.Len(t, transport.TLSClientConfig.Certificates, 1)
		assert.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
		assert.Zero(t, tlsConfig.MinVersion)
	})

	t.Run("keeps a stricter minimum TLS version", func(t *testing.T) {
		transport := newTransport(&tls.Config{MinVersion: tls.VersionTLS13})

		assert.Equal(t, uint16(tls.VersionTLS13), transport.TLSClientConfig.MinVersion)
	})

	t.Run("keeps the proxy from the environment", func(t *testing.T) {
		assert.NotNil(t, newTransport(nil).Proxy)
	})
}

func TestRefuseDowngrade(t *testing.T) {
	t.Run("a redirect from HTTPS to HTTP is refused", func(t *testing.T) {
		var plainRequests int
		plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			plainRequests++
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(plain.Close)
		secure := httptest.NewTLSServer(http.RedirectHandler(plain.URL, http.StatusFound))
		t.Cleanup(secure.Close)
		client := newHTTPClient(newTransport(trustingTLSConfig(secure)))

		resp, err := client.Get(secure.URL)
		if resp != nil {
			_ = resp.Body.Close()
		}

		require.ErrorContains(t, err, "refusing redirect from HTTPS to http")
		assert.Zero(t, plainRequests)
	})

	t.Run("a redirect from HTTPS to HTTPS is followed", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.Handle("/old", http.RedirectHandler("/new", http.StatusFound))
		mux.HandleFunc("/new", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
		secure := httptest.NewTLSServer(mux)
		t.Cleanup(secure.Close)
		client := newHTTPClient(newTransport(trustingTLSConfig(secure)))

		resp, err := client.Get(secure.URL + "/old")
		require.NoError(t, err)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("redirects stay limited", func(t *testing.T) {
		via := make([]*http.Request, maxRedirects)
		for i := range via {
			via[i] = httptest.NewRequest(http.MethodGet, "http://lrza.example/fhir", nil)
		}

		err := refuseDowngrade(httptest.NewRequest(http.MethodGet, "http://lrza.example/fhir", nil), via)

		require.ErrorContains(t, err, "stopped after 10 redirects")
	})
}

func TestUpdate_Bounded(t *testing.T) {
	t.Run("waiting for a running sync ends with the caller's context", func(t *testing.T) {
		c, err := New(Config{LRZABaseUrl: "http://lrza.example/fhir", QueryBaseUrl: "http://query.example/fhir"})
		require.NoError(t, err)
		c.syncSlot <- struct{}{} // a sync is running

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err = c.update(ctx)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorContains(t, err, "waiting for a running LRZA sync")
	})

	t.Run("a sync that stops making progress ends after syncTimeout and frees the slot", func(t *testing.T) {
		setDuration(t, &syncTimeout, 100*time.Millisecond)
		source := newHangingServer(t)
		c, err := New(Config{LRZABaseUrl: source.URL, QueryBaseUrl: "http://query.example/fhir"})
		require.NoError(t, err)

		_, err = c.update(context.Background())

		require.ErrorIs(t, err, errSyncTimeout)
		assert.Empty(t, c.syncSlot, "the next sync must be able to run")
	})
}
