package authn

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tlsutil"
	"golang.org/x/oauth2"
)

type HTTPClientProvider func(ctx context.Context, scope []string, uraNumber string, audience string) (*http.Client, error)

// MinistryAuthConfig holds the configuration for authenticating with the Ministry of Health's (MinVWS) OAuth2 Authorization Server using the client credentials flow with JWT assertion.
type MinistryAuthConfig struct {
	tlsutil.Config `koanf:",squash"`
	// TokenEndpoint is the URL of the Ministry's token endpoint, e.g. "https://oauth.proeftuin.gf.irealisatie.nl/oauth/token"
	TokenEndpoint string `koanf:"tokenendpoint"`
}

// MinVWSHTTPClient returns an HTTP client suitable for making authenticated requests to the Ministry of Health's APIs.
// Clients are cached per URA number, audience and scope, so connections and access tokens are reused across calls.
// Callers share the returned client and must not modify it.
func (c *Component) MinVWSHTTPClient(ctx context.Context, scope []string, uraNumber string, audience string) (*http.Client, error) {
	c.mux.Lock()
	defer c.mux.Unlock()
	if c.transport == nil {
		return nil, errors.New("authn component is not started")
	}
	key := clientCacheKey(scope, uraNumber, audience)
	if httpClient, ok := c.clients[key]; ok {
		return httpClient, nil
	}
	httpClient, err := c.newMinVWSHTTPClient(ctx, scope, uraNumber, audience)
	if err != nil {
		return nil, err
	}
	c.clients[key] = httpClient
	return httpClient, nil
}

func clientCacheKey(scope []string, uraNumber string, audience string) string {
	return uraNumber + "|" + audience + "|" + strings.Join(slices.Sorted(slices.Values(scope)), " ")
}

func (c *Component) newMinVWSHTTPClient(ctx context.Context, scope []string, uraNumber string, targetAudience string) (*http.Client, error) {
	tokenEndpoint := c.config.MinVWS.TokenEndpoint
	if tokenEndpoint == "" {
		slog.WarnContext(ctx, "MinVWS OAuth2 token endpoint is not configured, not using authentication for MinVWS API calls")
		// Fallback: if no token endpoint is configured, return a default HTTP client without authentication.
		return &http.Client{
			Transport: tracing.WrapTransport(c.transport),
		}, nil
	}

	if c.tlsConfig == nil {
		return nil, errors.New("token endpoint is configured but TLS client certificate is not configured")
	}

	return &http.Client{
		Transport: &oauth2.Transport{
			Base: tracing.WrapTransport(c.transport),
			Source: oauth2.ReuseTokenSource(nil, tokenSource{
				uraNumber:      uraNumber,
				targetAudience: targetAudience,
				scope:          slices.Clone(scope),
				tokenEndpoint:  tokenEndpoint,
				tlsConfig:      c.tlsConfig,
				httpClient:     &http.Client{Transport: c.transport},
			}),
		},
	}, nil
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
	// httpClient is used to call the token endpoint
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
	// TODO: Might have to support multiple key/alg types in the future
	jwtGrantTokenSigned, err := jwt.Sign(jwtGrantToken, jwt.WithKey(jwa.RS256, clientCert.PrivateKey, jws.WithProtectedHeaders(headers)))
	if err != nil {
		return nil, fmt.Errorf("sign JWT: %w", err)
	}
	httpResponse, err := t.httpClient.PostForm(t.tokenEndpoint, url.Values{
		"grant_type":            {"client_credentials"},
		"scope":                 {strings.Join(t.scope, " ")},
		"target_audience":       {t.targetAudience},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {string(jwtGrantTokenSigned)},
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
	token.Expiry = tokenExpiry(token)
	return &token, nil
}

// tokenExpiry determines when an access token expires, since the token is reused until then.
// expires_in is only RECOMMENDED (RFC 6749 section 5.1): the MinVWS AS omits it and puts the validity
// in the exp claim of the JWT access token instead.
// If the expiry can't be determined, the token is considered expired right away, so it is not reused.
func tokenExpiry(token oauth2.Token) time.Time {
	if token.ExpiresIn > 0 {
		return time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	}
	// The signature is not ours to check: the token is only parsed to read its expiry
	if accessToken, err := jwt.ParseInsecure([]byte(token.AccessToken)); err == nil && !accessToken.Expiration().IsZero() {
		return accessToken.Expiration()
	}
	return time.Now()
}
