package httpclient

import (
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

// trustingTLSConfig returns a TLS config that trusts server's test certificate.
func trustingTLSConfig(server *httptest.Server) *tls.Config {
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	return &tls.Config{RootCAs: pool}
}

func get(t *testing.T, client *http.Client, url string) (*http.Response, error) {
	t.Helper()
	resp, err := client.Get(url)
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}
	return resp, err
}

func TestNewTransport(t *testing.T) {
	t.Run("sets a timeout for every phase of an attempt", func(t *testing.T) {
		transport := NewTransport(nil)

		assert.NotNil(t, transport.DialContext)
		assert.Equal(t, DefaultTLSHandshakeTimeout, transport.TLSHandshakeTimeout)
		assert.Equal(t, DefaultResponseHeaderTimeout, transport.ResponseHeaderTimeout)
	})

	t.Run("an attempt gives up when the server doesn't answer", func(t *testing.T) {
		server := newHangingServer(t)
		transport := NewTransport(nil)
		transport.ResponseHeaderTimeout = 50 * time.Millisecond

		start := time.Now()
		_, err := get(t, New(transport), server.URL)

		require.ErrorContains(t, err, "timeout awaiting response headers")
		assert.Less(t, time.Since(start), 5*time.Second)
	})

	t.Run("keeps the TLS config, without changing the caller's", func(t *testing.T) {
		tlsConfig := &tls.Config{Certificates: []tls.Certificate{{}}}

		transport := NewTransport(tlsConfig)

		assert.Len(t, transport.TLSClientConfig.Certificates, 1)
		assert.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
		assert.Zero(t, tlsConfig.MinVersion)
	})

	t.Run("requires at least TLS 1.2 without a TLS config", func(t *testing.T) {
		assert.Equal(t, uint16(tls.VersionTLS12), NewTransport(nil).TLSClientConfig.MinVersion)
	})

	t.Run("keeps a stricter minimum TLS version", func(t *testing.T) {
		transport := NewTransport(&tls.Config{MinVersion: tls.VersionTLS13})

		assert.Equal(t, uint16(tls.VersionTLS13), transport.TLSClientConfig.MinVersion)
	})

	t.Run("keeps the proxy from the environment", func(t *testing.T) {
		assert.NotNil(t, NewTransport(nil).Proxy)
	})
}

func TestNew(t *testing.T) {
	t.Run("bounds a request as a whole", func(t *testing.T) {
		assert.Equal(t, DefaultRequestTimeout, New(NewTransport(nil)).Timeout)

		server := newHangingServer(t)
		client := New(NewTransport(nil))
		client.Timeout = 50 * time.Millisecond

		_, err := get(t, client, server.URL)

		require.ErrorContains(t, err, "Client.Timeout exceeded")
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

		_, err := get(t, New(NewTransport(trustingTLSConfig(secure))), secure.URL)

		require.ErrorContains(t, err, "refusing redirect from HTTPS to http")
		assert.Zero(t, plainRequests)
	})

	t.Run("a redirect from HTTPS to HTTPS is followed", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.Handle("/old", http.RedirectHandler("/new", http.StatusFound))
		mux.HandleFunc("/new", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
		secure := httptest.NewTLSServer(mux)
		t.Cleanup(secure.Close)

		resp, err := get(t, New(NewTransport(trustingTLSConfig(secure))), secure.URL+"/old")

		require.NoError(t, err)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("redirects stay limited", func(t *testing.T) {
		via := make([]*http.Request, maxRedirects)
		for i := range via {
			via[i] = httptest.NewRequest(http.MethodGet, "http://server.example/fhir", nil)
		}

		err := RefuseDowngrade(httptest.NewRequest(http.MethodGet, "http://server.example/fhir", nil), via)

		require.ErrorContains(t, err, "stopped after 10 redirects")
	})
}
