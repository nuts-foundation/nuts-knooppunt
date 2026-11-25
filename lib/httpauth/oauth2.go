package httpauth

import (
	"context"
	"fmt"
	"net/http"

	"github.com/nuts-foundation/nuts-knooppunt/lib/httpclient"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

// OAuth2Config holds the configuration for OAuth2 client credentials authentication.
type OAuth2Config struct {
	TokenEndpoint string   `koanf:"tokenendpoint"`
	ClientID     string   `koanf:"clientid"`
	ClientSecret string   `koanf:"clientsecret"`
	Scopes       []string `koanf:"scopes"`
}

// IsConfigured returns true if the OAuth2 configuration has all required fields set.
func (c OAuth2Config) IsConfigured() bool {
	return c.TokenEndpoint != "" && c.ClientID != "" && c.ClientSecret != ""
}

// NewOAuth2HTTPClient creates an http.Client that automatically handles OAuth2 client credentials authentication.
// It uses golang.org/x/oauth2/clientcredentials for token acquisition, caching, and automatic refresh.
// The baseTransport is used for both token endpoint calls and resource requests (e.g., for tracing).
// Pass nil to use http.DefaultTransport.
func NewOAuth2HTTPClient(config OAuth2Config, baseTransport http.RoundTripper) (*http.Client, error) {
	if !config.IsConfigured() {
		return nil, fmt.Errorf("oauth2 configuration is incomplete: tokenendpoint, clientid, and clientsecret are required")
	}

	conf := &clientcredentials.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		TokenURL:     config.TokenEndpoint,
		Scopes:       config.Scopes,
		AuthStyle:    oauth2.AuthStyleInParams,
	}

	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	// Inject the base transport via context so x/oauth2 uses it for both
	// token requests and the returned client's underlying transport. The returned
	// client also takes over tokenClient's Timeout and CheckRedirect (see
	// oauth2.NewClient), so both get the limits of httpclient.New.
	// Token requests run under this context, not the caller's request context, so
	// without a timeout here a token endpoint that stops answering blocks the caller's
	// request forever, whatever timeout the caller set on the returned client.
	tokenClient := httpclient.New(baseTransport)
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, tokenClient)

	return conf.Client(ctx), nil
}