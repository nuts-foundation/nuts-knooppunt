package authn

import "github.com/zitadel/oidc/v3/pkg/op"

var _ op.TokenRequest = (*ClientCredentialsTokenRequest)(nil)

type ClientCredentialsTokenRequest struct {
	Subject  string
	Audience []string
	Scopes   []string
}

func (c ClientCredentialsTokenRequest) GetSubject() string {
	return c.Subject
}

func (c ClientCredentialsTokenRequest) GetAudience() []string {
	return c.Audience
}

func (c ClientCredentialsTokenRequest) GetScopes() []string {
	return c.Scopes
}
