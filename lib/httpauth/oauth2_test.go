package httpauth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/httpauth"
	"github.com/stretchr/testify/require"
)

const hourExpiry = 3600

// tokenResponse is a test helper matching the OAuth2 token endpoint response format.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type,omitempty"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

// newOAuth2TokenServer creates a test OAuth2 token server that returns the given access token.
// An optional validate function can inspect the request before the response is written.
func newOAuth2TokenServer(t *testing.T, accessToken string, expiresIn int, validate func(r *http.Request)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if validate != nil {
			validate(r)
		}
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: accessToken,
			TokenType:   "Bearer",
			ExpiresIn:   expiresIn,
		})
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	return server
}

// newCaptureServer returns an httptest.Server that captures the Authorization header
// from each incoming request and a function to retrieve the last captured value.
func newCaptureServer(t *testing.T) (*httptest.Server, func() string) {
	t.Helper()
	var capturedAuth atomic.Value
	capturedAuth.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	return server, func() string { return capturedAuth.Load().(string) }
}

func TestOAuth2Config_IsConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config httpauth.OAuth2Config
		want   bool
	}{
		{
			name:   "empty config",
			config: httpauth.OAuth2Config{},
		},
		{
			name: "missing token URL",
			config: httpauth.OAuth2Config{
				ClientID:     "id",
				ClientSecret: "secret",
			},
		},
		{
			name: "missing client ID",
			config: httpauth.OAuth2Config{
				TokenEndpoint:     "http://example.com/token",
				ClientSecret:      "secret",
			},
		},
		{
			name: "missing client secret",
			config: httpauth.OAuth2Config{
				TokenEndpoint: "http://example.com/token",
				ClientID:      "id",
			},
		},
		{
			name: "fully configured",
			config: httpauth.OAuth2Config{
				TokenEndpoint: "http://example.com/token",
				ClientID:      "id",
				ClientSecret:  "secret",
			},
			want: true,
		},
		{
			name: "with scopes",
			config: httpauth.OAuth2Config{
				TokenEndpoint: "http://example.com/token",
				ClientID:      "id",
				ClientSecret:  "secret",
				Scopes:        []string{"read", "write"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, tt.config.IsConfigured())
		})
	}
}

func TestNewOAuth2HTTPClient(t *testing.T) {
	t.Parallel()

	t.Run("returns error for incomplete config", func(t *testing.T) {
		t.Parallel()
		_, err := httpauth.NewOAuth2HTTPClient(httpauth.OAuth2Config{}, nil)
		require.Error(t, err)
	})

	t.Run("makes authenticated requests", func(t *testing.T) {
		t.Parallel()
		tokenServer := newOAuth2TokenServer(t, "my-access-token", hourExpiry, nil)

		resourceServer, getAuth := newCaptureServer(t)

		config := httpauth.OAuth2Config{
			TokenEndpoint: tokenServer.URL,
			ClientID:      "id",
			ClientSecret:  "secret",
		}

		client, err := httpauth.NewOAuth2HTTPClient(config, nil)
		require.NoError(t, err)

		resp, err := client.Get(resourceServer.URL)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, "Bearer my-access-token", getAuth())
	})

	t.Run("sends client credentials in form body", func(t *testing.T) {
		t.Parallel()
		tokenServer := newOAuth2TokenServer(t, "token", hourExpiry, func(r *http.Request) {
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))

			err := r.ParseForm()
			require.NoError(t, err)
			require.Equal(t, "client_credentials", r.PostForm.Get("grant_type"))
			require.Equal(t, "test-client", r.PostForm.Get("client_id"))
			require.Equal(t, "test-secret", r.PostForm.Get("client_secret"))
		})

		config := httpauth.OAuth2Config{
			TokenEndpoint: tokenServer.URL,
			ClientID:     "test-client",
			ClientSecret: "test-secret",
		}

		client, err := httpauth.NewOAuth2HTTPClient(config, nil)
		require.NoError(t, err)

		resourceServer, _ := newCaptureServer(t)
		resp, err := client.Get(resourceServer.URL)
		require.NoError(t, err)
		defer resp.Body.Close()
	})

	t.Run("includes scopes in token request", func(t *testing.T) {
		t.Parallel()
		tokenServer := newOAuth2TokenServer(t, "token", hourExpiry, func(r *http.Request) {
			err := r.ParseForm()
			require.NoError(t, err)
			require.Equal(t, "read write", r.PostForm.Get("scope"))
		})

		config := httpauth.OAuth2Config{
			TokenEndpoint: tokenServer.URL,
			ClientID:     "id",
			ClientSecret: "secret",
			Scopes:       []string{"read", "write"},
		}

		client, err := httpauth.NewOAuth2HTTPClient(config, nil)
		require.NoError(t, err)

		resourceServer, _ := newCaptureServer(t)
		resp, err := client.Get(resourceServer.URL)
		require.NoError(t, err)
		defer resp.Body.Close()
	})

	t.Run("uses base transport for requests", func(t *testing.T) {
		t.Parallel()
		var transportUsed bool
		customTransport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			transportUsed = true
			return http.DefaultTransport.RoundTrip(req)
		})

		tokenServer := newOAuth2TokenServer(t, "token", hourExpiry, nil)

		config := httpauth.OAuth2Config{
			TokenEndpoint: tokenServer.URL,
			ClientID:     "id",
			ClientSecret: "secret",
		}

		client, err := httpauth.NewOAuth2HTTPClient(config, customTransport)
		require.NoError(t, err)

		resourceServer, _ := newCaptureServer(t)
		resp, err := client.Get(resourceServer.URL)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.True(t, transportUsed, "custom base transport should be used")
	})
}

// roundTripFunc is an adapter to allow use of ordinary functions as http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
