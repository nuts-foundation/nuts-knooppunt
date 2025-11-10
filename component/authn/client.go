package authn

import (
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type Client struct {
	ID      string `koanf:"id"`
	Secret  string `koanf:"secret"`
	devMode bool
}

func (c Client) GetID() string {
	return c.ID
}

func (c Client) RedirectURIs() []string {
	//TODO implement me
	panic("implement me")
}

func (c Client) PostLogoutRedirectURIs() []string {
	//TODO implement me
	panic("implement me")
}

func (c Client) ApplicationType() op.ApplicationType {
	//TODO implement me
	panic("implement me")
}

func (c Client) AuthMethod() oidc.AuthMethod {
	//TODO implement me
	panic("implement me")
}

func (c Client) ResponseTypes() []oidc.ResponseType {
	//TODO implement me
	panic("implement me")
}

func (c Client) GrantTypes() []oidc.GrantType {
	// Extend this list as we add support for new grant types
	return []oidc.GrantType{
		oidc.GrantTypeClientCredentials,
	}
}

func (c Client) LoginURL(s string) string {
	//TODO implement me
	panic("LoginURL(): implement me")
}

func (c Client) AccessTokenType() op.AccessTokenType {
	return op.AccessTokenTypeBearer
}

func (c Client) IDTokenLifetime() time.Duration {
	//TODO implement me
	panic("IDTokenLifetime(): implement me")
}

func (c Client) DevMode() bool {
	return c.devMode
}

func (c Client) RestrictAdditionalIdTokenScopes() func(scopes []string) []string {
	//TODO implement me
	panic("implement me")
}

func (c Client) RestrictAdditionalAccessTokenScopes() func(scopes []string) []string {
	//TODO implement me
	panic("implement me")
}

func (c Client) IsScopeAllowed(scope string) bool {
	//TODO implement me
	panic("implement me")
}

func (c Client) IDTokenUserinfoClaimsAssertion() bool {
	//TODO implement me
	panic("implement me")
}

func (c Client) ClockSkew() time.Duration {
	return 10 * time.Second
}
