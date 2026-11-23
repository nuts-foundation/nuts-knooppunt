package lrza

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
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

	t.Run("a sync that stops making progress ends after pageTimeout and frees the slot", func(t *testing.T) {
		setDuration(t, &pageTimeout, 100*time.Millisecond)
		source := newHangingServer(t)
		c, err := New(Config{LRZABaseUrl: source.URL, QueryBaseUrl: "http://query.example/fhir"})
		require.NoError(t, err)

		_, err = c.update(context.Background())

		require.ErrorIs(t, err, errPageTimeout)
		assert.Empty(t, c.syncSlot, "the next sync must be able to run")
	})

	t.Run("a slow sync that keeps making progress is not cut off by pageTimeout", func(t *testing.T) {
		// Every page takes 30 ms and there are 14 of them (10 Organization pages, plus one empty page for
		// each other type), so the sync as a whole takes several times pageTimeout.
		setDuration(t, &pageTimeout, 200*time.Millisecond)
		const organizationPages = 10
		var source *httptest.Server
		source = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(30 * time.Millisecond)
			bundle := map[string]any{"resourceType": "Bundle", "type": "searchset"}
			if r.URL.Path == "/Organization" {
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				bundle["entry"] = []any{map[string]any{"resource": map[string]any{"resourceType": "Organization", "id": fmt.Sprintf("org-%d", page)}}}
				if page+1 < organizationPages {
					bundle["link"] = []any{map[string]any{"relation": "next", "url": fmt.Sprintf("%s/Organization?page=%d", source.URL, page+1)}}
				}
			}
			writeJSON(w, http.StatusOK, bundle)
		}))
		t.Cleanup(source.Close)
		query := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"resourceType": "Bundle", "type": "transaction-response",
				"entry": []any{map[string]any{"response": map[string]any{"status": "201 Created"}}}})
		}))
		t.Cleanup(query.Close)
		c, err := New(Config{LRZABaseUrl: source.URL, QueryBaseUrl: query.URL})
		require.NoError(t, err)

		start := time.Now()
		report, err := c.update(context.Background())

		require.NoError(t, err)
		assert.Equal(t, organizationPages, report.CountCreated)
		assert.Greater(t, time.Since(start), 2*pageTimeout, "the test must outlast pageTimeout to show anything")
	})

	t.Run("a sync that never ends stops after syncTimeout and frees the slot", func(t *testing.T) {
		setDuration(t, &syncTimeout, 100*time.Millisecond)
		source := newHangingServer(t)
		c, err := New(Config{LRZABaseUrl: source.URL, QueryBaseUrl: "http://query.example/fhir"})
		require.NoError(t, err)

		_, err = c.update(context.Background())

		require.ErrorIs(t, err, errSyncTimeout)
		assert.Empty(t, c.syncSlot, "the next sync must be able to run")
	})
}
