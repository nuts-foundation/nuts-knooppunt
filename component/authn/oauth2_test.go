package authn

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tlsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestHTTPClient_IntegrationTest(t *testing.T) {
	t.Run("proeftuin", func(t *testing.T) {
		t.Skip()
		const (
			tokenEndpoint = "https://oauth.proeftuin.gf.irealisatie.nl/oauth/token"
			audience      = "https://pseudoniemendienst.proeftuin.gf.irealisatie.nl/"
			ura           = "URA123456789"
		)
		component := New(Config{MinVWS: MinistryAuthConfig{
			Config:        tlsutil.Config{TLSCertFile: "cert.pem", TLSKeyFile: "cert-key.pem"},
			TokenEndpoint: tokenEndpoint,
		}})
		require.NoError(t, component.Start())
		httpClient, err := component.MinVWSHTTPClient(t.Context(), []string{"prs:read"}, ura, audience)
		require.NoError(t, err)

		httpResponse, err := httpClient.Get("https://example.com")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, httpResponse.StatusCode)
	})
}

func TestComponent_MinVWSHTTPClient(t *testing.T) {
	const (
		ura      = "URA123456789"
		audience = "https://nvi.example.com"
	)
	scope := []string{"epd:read", "epd:write"}

	t.Run("reuses the client and its access token", func(t *testing.T) {
		tokenServer, tokenRequests := startTokenServer(t, 3600)
		apiServer, authorizations := startAPIServer(t)
		component := startComponent(t, tokenServer.URL)

		first, err := component.MinVWSHTTPClient(t.Context(), scope, ura, audience)
		require.NoError(t, err)
		second, err := component.MinVWSHTTPClient(t.Context(), []string{"epd:write", "epd:read"}, ura, audience)
		require.NoError(t, err)
		assert.Same(t, first, second)

		for range 3 {
			get(t, first, apiServer.URL)
		}
		assert.Equal(t, int32(1), tokenRequests.Load())
		assert.Equal(t, []string{"Bearer token-1", "Bearer token-1", "Bearer token-1"}, authorizations())
	})
	t.Run("concurrent callers share one access token", func(t *testing.T) {
		tokenServer, tokenRequests := startTokenServer(t, 3600)
		apiServer, _ := startAPIServer(t)
		component := startComponent(t, tokenServer.URL)

		var wg sync.WaitGroup
		for range 10 {
			wg.Go(func() {
				httpClient, err := component.MinVWSHTTPClient(t.Context(), scope, ura, audience)
				if assert.NoError(t, err) {
					get(t, httpClient, apiServer.URL)
				}
			})
		}
		wg.Wait()
		assert.Equal(t, int32(1), tokenRequests.Load())
	})
	t.Run("separate clients per URA number, audience and scope", func(t *testing.T) {
		tokenServer, tokenRequests := startTokenServer(t, 3600)
		apiServer, _ := startAPIServer(t)
		component := startComponent(t, tokenServer.URL)

		base, err := component.MinVWSHTTPClient(t.Context(), scope, ura, audience)
		require.NoError(t, err)
		otherURA, err := component.MinVWSHTTPClient(t.Context(), scope, "URA987654321", audience)
		require.NoError(t, err)
		otherAudience, err := component.MinVWSHTTPClient(t.Context(), scope, ura, "https://prs.example.com")
		require.NoError(t, err)
		otherScope, err := component.MinVWSHTTPClient(t.Context(), []string{"epd:read"}, ura, audience)
		require.NoError(t, err)
		assert.NotSame(t, base, otherURA)
		assert.NotSame(t, base, otherAudience)
		assert.NotSame(t, base, otherScope)

		for _, httpClient := range []*http.Client{base, otherURA, otherAudience, otherScope} {
			get(t, httpClient, apiServer.URL)
		}
		assert.Equal(t, int32(4), tokenRequests.Load())
	})
	t.Run("requests a new access token when it expires", func(t *testing.T) {
		// A token that expires within a few seconds is already considered expired, so every call needs a new one.
		tokenServer, tokenRequests := startTokenServer(t, 1)
		apiServer, authorizations := startAPIServer(t)
		component := startComponent(t, tokenServer.URL)

		httpClient, err := component.MinVWSHTTPClient(t.Context(), scope, ura, audience)
		require.NoError(t, err)
		get(t, httpClient, apiServer.URL)
		get(t, httpClient, apiServer.URL)

		assert.Equal(t, int32(2), tokenRequests.Load())
		assert.Equal(t, []string{"Bearer token-1", "Bearer token-2"}, authorizations())
	})
	t.Run("without token endpoint, returns a client without authentication", func(t *testing.T) {
		apiServer, authorizations := startAPIServer(t)
		component := startComponent(t, "")

		first, err := component.MinVWSHTTPClient(t.Context(), scope, ura, audience)
		require.NoError(t, err)
		second, err := component.MinVWSHTTPClient(t.Context(), scope, ura, audience)
		require.NoError(t, err)
		assert.Same(t, first, second)

		get(t, first, apiServer.URL)
		assert.Equal(t, []string{""}, authorizations())
	})
	t.Run("token endpoint without certificate", func(t *testing.T) {
		component := New(Config{MinVWS: MinistryAuthConfig{TokenEndpoint: "https://example.com/token"}})
		require.NoError(t, component.Start())

		httpClient, err := component.MinVWSHTTPClient(t.Context(), scope, ura, audience)
		assert.EqualError(t, err, "token endpoint is configured but TLS client certificate is not configured")
		assert.Nil(t, httpClient)
	})
	t.Run("not started", func(t *testing.T) {
		component := New(Config{})

		httpClient, err := component.MinVWSHTTPClient(t.Context(), scope, ura, audience)
		assert.EqualError(t, err, "authn component is not started")
		assert.Nil(t, httpClient)
	})
}

func TestComponent_Start(t *testing.T) {
	t.Run("loads the configured certificate", func(t *testing.T) {
		certFile, keyFile := writeClientCertificate(t)
		component := New(Config{MinVWS: MinistryAuthConfig{
			Config: tlsutil.Config{TLSCertFile: certFile, TLSKeyFile: keyFile},
		}})

		require.NoError(t, component.Start())
		require.NotNil(t, component.tlsConfig)
		assert.Len(t, component.tlsConfig.Certificates, 1)
	})
	t.Run("no certificate configured", func(t *testing.T) {
		component := New(Config{})

		require.NoError(t, component.Start())
		assert.Nil(t, component.tlsConfig)
	})
	t.Run("missing certificate file", func(t *testing.T) {
		dir := t.TempDir()
		component := New(Config{MinVWS: MinistryAuthConfig{
			Config: tlsutil.Config{TLSCertFile: filepath.Join(dir, "cert.pem"), TLSKeyFile: filepath.Join(dir, "key.pem")},
		}})

		err := component.Start()
		assert.ErrorContains(t, err, "MinVWS TLS is configured but failed to load")
	})
	t.Run("unreadable certificate file", func(t *testing.T) {
		certFile, keyFile := writeClientCertificate(t)
		require.NoError(t, os.WriteFile(certFile, []byte("not a certificate"), 0o600))
		component := New(Config{MinVWS: MinistryAuthConfig{
			Config: tlsutil.Config{TLSCertFile: certFile, TLSKeyFile: keyFile},
		}})

		err := component.Start()
		assert.ErrorContains(t, err, "MinVWS TLS is configured but failed to load")
	})
}

// startComponent starts an authn component with a freshly generated client certificate and the given token endpoint.
func startComponent(t *testing.T, tokenEndpoint string) *Component {
	t.Helper()
	certFile, keyFile := writeClientCertificate(t)
	component := New(Config{MinVWS: MinistryAuthConfig{
		Config:        tlsutil.Config{TLSCertFile: certFile, TLSKeyFile: keyFile},
		TokenEndpoint: tokenEndpoint,
	}})
	require.NoError(t, component.Start())
	t.Cleanup(func() {
		_ = component.Stop(context.Background())
	})
	return component
}

// startTokenServer starts a token endpoint that issues token-1, token-2, ..., each valid for expiresIn seconds.
func startTokenServer(t *testing.T, expiresIn int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.PostFormValue("grant_type") != "client_credentials" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		n := requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"token-%d","token_type":"Bearer","expires_in":%d}`, n, expiresIn)
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

// startAPIServer starts a server that records the Authorization header of each request it receives.
func startAPIServer(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mux sync.Mutex
	var authorizations []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.Lock()
		defer mux.Unlock()
		authorizations = append(authorizations, r.Header.Get("Authorization"))
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mux.Lock()
		defer mux.Unlock()
		return slices.Clone(authorizations)
	}
}

func get(t *testing.T, httpClient *http.Client, url string) {
	t.Helper()
	response, err := httpClient.Get(url)
	if !assert.NoError(t, err) {
		return
	}
	_ = response.Body.Close()
	assert.Equal(t, http.StatusOK, response.StatusCode)
}

// writeClientCertificate writes a self-signed client certificate and its key as PEM files.
func writeClientCertificate(t *testing.T) (certFile string, keyFile string) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test client"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certBytes}), 0o600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}), 0o600))
	return certFile, keyFile
}

func TestHTTPClient(t *testing.T) {
	t.Run("successful token retrieval", func(t *testing.T) {
		var receivedFormDataChan = make(chan url.Values, 1)

		tokenServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			require.NoError(t, err)
			receivedFormDataChan <- r.Form
			assert.Equal(t, "POST", r.Method)

			// Validate form parameters
			assert.Equal(t, "client_credentials", r.Form.Get("grant_type"))
			assert.Equal(t, "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", r.Form.Get("client_assertion_type"))
			assert.NotEmpty(t, r.Form.Get("client_assertion"))

			// Return mock access token
			token := oauth2.Token{
				AccessToken: "test-access-token",
				TokenType:   "Bearer",
				Expiry:      time.Now().Add(1 * time.Hour),
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(token)
		}))
		defer tokenServer.Close()

		scope := []string{"resource.read", "resource.write"}
		uraNumber := "URA123456789"
		targetAudience := "https://example.com/fhir"

		// Create tokenSource with test server's HTTP client
		ts := tokenSource{
			uraNumber:      uraNumber,
			targetAudience: targetAudience,
			scope:          scope,
			tokenEndpoint:  tokenServer.URL,
			tlsConfig:      tokenServer.TLS,
			httpClient:     tokenServer.Client(),
		}

		// Get token
		token, err := ts.Token()
		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Equal(t, "test-access-token", token.AccessToken)
		assert.Equal(t, "Bearer", token.TokenType)

		// Verify JWT grant token
		receivedFormData := <-receivedFormDataChan
		clientCredentials := receivedFormData.Get("client_assertion")
		require.NotEmpty(t, clientCredentials)

		// Parse and validate the JWT
		parsedToken, err := jwt.Parse([]byte(clientCredentials), jwt.WithVerify(false))
		require.NoError(t, err)

		// Validate claims
		assert.Equal(t, uraNumber, parsedToken.Issuer())
		assert.Equal(t, uraNumber, parsedToken.Subject())
		assert.Contains(t, parsedToken.Audience(), tokenServer.URL)
		assert.NotEmpty(t, parsedToken.JwtID())

		// Validate custom claims
		actualScope, _ := parsedToken.Get("scope")
		assert.Equal(t, strings.Join(scope, " "), actualScope)
		actualTargetAudience, _ := parsedToken.Get("target_audience")
		assert.Equal(t, targetAudience, actualTargetAudience)

		cnfClaim, _ := parsedToken.Get("cnf")
		assert.NotEmpty(t, cnfClaim.(map[string]interface{})["x5t#S256"])

		// Validate scope and target_audience in form data
		assert.Equal(t, strings.Join(scope, " "), receivedFormData.Get("scope"))
		assert.Equal(t, targetAudience, receivedFormData.Get("target_audience"))
	})
}

func TestTokenSource_Token(t *testing.T) {
	t.Run("successful token request", func(t *testing.T) {
		// Create mock token endpoint
		tokenServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := oauth2.Token{
				AccessToken: "test-token-123",
				TokenType:   "Bearer",
				Expiry:      time.Now().Add(1 * time.Hour),
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(token)
		}))
		defer tokenServer.Close()

		ts := tokenSource{
			uraNumber:      "URA987654321",
			targetAudience: "https://target.example.com",
			scope:          []string{"read", "write"},
			tokenEndpoint:  tokenServer.URL,
			tlsConfig:      tokenServer.TLS,
			httpClient:     tokenServer.Client(),
		}

		token, err := ts.Token()
		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Equal(t, "test-token-123", token.AccessToken)
		assert.Equal(t, "Bearer", token.TokenType)
	})

	t.Run("sets the expiry from expires_in", func(t *testing.T) {
		tokenServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"test-token-123","token_type":"Bearer","expires_in":3600}`))
		}))
		defer tokenServer.Close()

		ts := tokenSource{
			uraNumber:      "URA987654321",
			targetAudience: "https://target.example.com",
			scope:          []string{"read"},
			tokenEndpoint:  tokenServer.URL,
			tlsConfig:      tokenServer.TLS,
			httpClient:     tokenServer.Client(),
		}

		token, err := ts.Token()
		require.NoError(t, err)
		assert.WithinDuration(t, time.Now().Add(time.Hour), token.Expiry, 5*time.Second)
		assert.True(t, token.Valid())
	})

	t.Run("token endpoint returns error", func(t *testing.T) {
		// Create mock token endpoint that returns an error
		tokenServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "invalid_client"}`))
		}))
		defer tokenServer.Close()

		ts := tokenSource{
			uraNumber:      "URA123",
			targetAudience: "https://target.example.com",
			scope:          []string{"read"},
			tokenEndpoint:  tokenServer.URL,
			tlsConfig:      tokenServer.TLS,
			httpClient:     tokenServer.Client(),
		}

		token, err := ts.Token()
		assert.Error(t, err)
		assert.Nil(t, token)
	})
}

func TestCertThumbprint(t *testing.T) {
	// Create a test certificate
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   "test.example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certBytes)
	require.NoError(t, err)

	// Calculate thumbprint
	thumbprint := certThumbprint(cert)

	// Verify it's not empty and is base64url encoded
	assert.NotEmpty(t, thumbprint)
	assert.True(t, len(thumbprint) > 0)
	// SHA-256 produces 32 bytes, base64url encoded should be 43 characters (without padding)
	assert.Equal(t, 43, len(thumbprint))
}
