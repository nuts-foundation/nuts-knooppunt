package authn

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
		httpClient, err := HTTPClient(t.Context(), []string{"prs:read"}, ura, audience, MinistryAuthConfig{
			Config:        tlsutil.Config{TLSCertFile: "cert.pem", TLSKeyFile: "cert-key.pem"},
			TokenEndpoint: tokenEndpoint,
		})
		require.NoError(t, err)

		httpResponse, err := httpClient.Get("https://example.com")
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, httpResponse.StatusCode)
	})
}

func TestHTTPClient(t *testing.T) {
	t.Run("successful token retrieval", func(t *testing.T) {
		// Generate test certificate
		//cert := generateTestCertificate(t)
		var receivedFormDataChan = make(chan url.Values, 1)

		tokenServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := r.ParseForm()
			require.NoError(t, err)
			receivedFormDataChan <- r.Form
			assert.Equal(t, "POST", r.Method)

			// Validate form parameters
			assert.Equal(t, "client_credentials", r.Form.Get("grant_type"))
			assert.Equal(t, "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", r.Form.Get("client_credentials_type"))
			assert.NotEmpty(t, r.Form.Get("client_credentials"))

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
		clientCredentials := receivedFormData.Get("client_credentials")
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
		// Generate test certificate
		//cert := generateTestCertificate(t)

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

	t.Run("token endpoint returns error", func(t *testing.T) {
		// Generate test certificate
		//cert := generateTestCertificate(t)

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

// generateTestCertificate creates a self-signed certificate for testing
func generateTestCertificate(t *testing.T) tls.Certificate {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Organization"},
			CommonName:   "test.example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certBytes)
	require.NoError(t, err)

	return tls.Certificate{
		Certificate: [][]byte{certBytes},
		PrivateKey:  privateKey,
		Leaf:        cert,
	}
}
