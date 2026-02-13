package authn

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tlsutil"
	"golang.org/x/oauth2"
)

// MinistryAuthConfig holds the configuration for authenticating with the Ministry of Health's (MinVWS) OAuth2 Authorization Server using the client credentials flow with JWT assertion.
type MinistryAuthConfig struct {
	tlsutil.Config
	// TokenEndpoint is the URL of the Ministry's token endpoint, e.g. "https://oauth.proeftuin.gf.irealisatie.nl/oauth/token"
	TokenEndpoint string `json:"tokenendpoint"`
}

// MinVWSHTTPClient creates an HTTP client suitable for making authenticated requests to the Ministry of Health's APIs.
func (c *Component) MinVWSHTTPClient(ctx context.Context, scope []string, uraNumber string, audience string) (*http.Client, error) {
	return HTTPClient(ctx, scope, uraNumber, audience, c.config.MinVWS)
}

func HTTPClient(ctx context.Context, scope []string, uraNumber string, targetAudience string, cfg MinistryAuthConfig) (*http.Client, error) {
	var tlsConfig *tls.Config
	if cfg.TLSCertFile != "" {
		var err error
		tlsConfig, err = tlsutil.CreateTLSConfig(cfg.Config)
		if err != nil {
			return nil, fmt.Errorf("TLS is configured but failed to load: %w", err)
		}
	}

	if cfg.TokenEndpoint == "" {
		// Fallback: if no token endpoint is configured, return a default HTTP client without authentication.
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
		}, nil
	}

	return oauth2.NewClient(ctx, tokenSource{
		uraNumber:      uraNumber,
		targetAudience: targetAudience,
		scope:          scope,
		tokenEndpoint:  cfg.TokenEndpoint,
		tlsConfig:      tlsConfig,
	}), nil
}

// certThumbprint calculates the x5t SHA-256 thumbprint of the certificate.
func certThumbprint(cert *x509.Certificate) string {
	h := sha256.Sum256(cert.Raw)
	return base64.RawURLEncoding.EncodeToString(h[:])
}

var _ oauth2.TokenSource = (*tokenSource)(nil)

type tokenSource struct {
	uraNumber      string
	targetAudience string
	scope          []string
	tokenEndpoint  string
	tlsConfig      *tls.Config
	// httpClient is optional and only used for testing
	httpClient *http.Client
}

func (t tokenSource) Token() (*oauth2.Token, error) {
	clientCert := t.tlsConfig.Certificates[0]
	jwtGrantToken := jwt.New()
	certThumbprint := certThumbprint(clientCert.Leaf)
	claims := map[string]any{
		jwt.IssuerKey:     t.uraNumber,
		jwt.SubjectKey:    t.uraNumber,
		jwt.AudienceKey:   []string{t.tokenEndpoint},
		jwt.IssuedAtKey:   time.Now(),
		jwt.ExpirationKey: time.Now().Add(time.Minute),
		jwt.JwtIDKey:      uuid.NewString(),
		"cnf": map[string]any{
			"x5t#S256": certThumbprint,
		},
		"scope":           strings.Join(t.scope, " "),
		"target_audience": t.targetAudience,
	}
	for key, value := range claims {
		if err := jwtGrantToken.Set(key, value); err != nil {
			return nil, fmt.Errorf("set %s: %w", key, err)
		}
	}
	headers := jws.NewHeaders()
	if err := headers.Set(jws.KeyIDKey, certThumbprint); err != nil {
		return nil, fmt.Errorf("set kid header: %w", err)
	}
	// TOOD: Might have to support multiple key/alg types in the future
	jwtGrantTokenSigned, err := jwt.Sign(jwtGrantToken, jwt.WithKey(jwa.RS256, clientCert.PrivateKey, jws.WithProtectedHeaders(headers)))
	if err != nil {
		return nil, fmt.Errorf("sign JWT: %w", err)
	}
	tokenHTTPClient := t.httpClient
	if tokenHTTPClient == nil {
		tokenHTTPClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: t.tlsConfig,
			},
		}
	}
	httpResponse, err := tokenHTTPClient.PostForm(t.tokenEndpoint, url.Values{
		"grant_type":              {"client_credentials"},
		"scope":                   {strings.Join(t.scope, " ")},
		"target_audience":         {t.targetAudience},
		"client_credentials_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_credentials":      {string(jwtGrantTokenSigned)},
	})
	if err != nil {
		return nil, fmt.Errorf("request token: %w", err)
	}
	defer httpResponse.Body.Close()
	if httpResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned status %d", httpResponse.StatusCode)
	}
	// Use LimitReader to prevent malicious servers from sending huge responses that exhaust memory
	responseData, err := io.ReadAll(io.LimitReader(httpResponse.Body, 1<<20+1))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}
	if len(responseData) > 1<<20 {
		return nil, fmt.Errorf("token response too large")
	}
	var token oauth2.Token
	if err := json.Unmarshal(responseData, &token); err != nil {
		return nil, fmt.Errorf("unmarshal token response: %w", err)
	}
	return &token, nil
}
