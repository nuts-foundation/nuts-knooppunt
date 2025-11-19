package authn

import (
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

// IDTokenLifetime defines the lifetime of ID tokens issued to clients.
// TODO: Adjust this if needed
const IDTokenLifetime = time.Hour

type Client struct {
	ID           string   `koanf:"id"`
	Secret       string   `koanf:"secret"`
	RedirectURLs []string `koanf:"redirecturls"`
	devMode      bool
}

func (c Client) GetID() string {
	return c.ID
}

func (c Client) RedirectURIs() []string {
	return append([]string{}, c.RedirectURLs...)
}

func (c Client) PostLogoutRedirectURIs() []string {
	//TODO implement me
	panic("implement me")
}

func (c Client) ApplicationType() op.ApplicationType {
	return op.ApplicationTypeWeb
}

func (c Client) AuthMethod() oidc.AuthMethod {
	return oidc.AuthMethodBasic
}

func (c Client) ResponseTypes() []oidc.ResponseType {
	return []oidc.ResponseType{oidc.ResponseTypeCode}
}

func (c Client) GrantTypes() []oidc.GrantType {
	// Extend this list as we add support for new grant types
	return []oidc.GrantType{
		oidc.GrantTypeCode,
	}
}

func (c Client) LoginURL(authRequestID string) string {
	return loginFormEndpointPath + "?authRequestID=" + authRequestID
}

func (c Client) AccessTokenType() op.AccessTokenType {
	return op.AccessTokenTypeBearer
}

func (c Client) IDTokenLifetime() time.Duration {
	return IDTokenLifetime
}

func (c Client) DevMode() bool {
	return c.devMode
}

func (c Client) RestrictAdditionalIdTokenScopes() func(scopes []string) []string {
	return func(scopes []string) []string {
		return scopes
	}
}

func (c Client) RestrictAdditionalAccessTokenScopes() func(scopes []string) []string {
	return func(scopes []string) []string {
		return scopes
	}
}

func (c Client) IsScopeAllowed(scope string) bool {
	return true
}

func (c Client) IDTokenUserinfoClaimsAssertion() bool {
	return false
}

func (c Client) ClockSkew() time.Duration {
	return 10 * time.Second
}
