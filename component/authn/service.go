package authn

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/nuts-foundation/nuts-knooppunt/cmd/core"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	httpComponent "github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

var _ component.Lifecycle = (*Component)(nil)

const (
	tokenEndpointPath              = "/auth/token"
	tokenIntrospectionEndpointPath = "/auth/introspect"
	authorizationEndpointPath      = "/auth/authorize"
)

type Config struct {
	Clients []Client `koanf:"clients"`
}

var endpointConfig = struct {
	publicEndpoints   []string
	internalEndpoints []string
}{
	publicEndpoints: []string{
		authorizationEndpointPath,
	},
	internalEndpoints: []string{
		tokenEndpointPath,
		tokenIntrospectionEndpointPath,
		"/.well-known/openid-configuration",
	},
}

// Component implements an OpenID Connect Provider using the zitadel/oidc library.
// Since its clients (the EHR) are internal to the Nuts Knooppunt, endpoints intended for the clients are registered on the internal mux.
// Endpoints intended for end-users (like the authorization endpoint) are registered on the public mux, so they can be accessed through the browser.
// This also means only confidential clients (clients capable of keeping a secret) are supported (https://oauth.net/2/client-types/).
type Component struct {
	provider        *op.Provider
	callbackURLFunc func(context.Context, string) string
	storage         *Storage
	strictMode      bool
}

func (c *Component) Start() error {
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	return nil
}

func (c *Component) RegisterHttpHandlers(publicMux *http.ServeMux, internalMux *http.ServeMux) {
	for _, endpoint := range endpointConfig.publicEndpoints {
		publicMux.Handle(endpoint, c.provider)
	}
	for _, endpoint := range endpointConfig.internalEndpoints {
		internalMux.Handle(endpoint, c.provider)
	}
}

func New(config Config, httpInterfaces httpComponent.InterfaceInfo, coreConfig core.Config) (*Component, error) {
	var extraOptions []op.Option
	if !coreConfig.StrictMode {
		extraOptions = append(extraOptions, op.WithAllowInsecure())
	}

	storage := &Storage{
		clients: make(map[string]Client),
		tokens:  &sync.Map{},
	}
	for _, client := range config.Clients {
		if _, exists := storage.clients[client.ID]; exists {
			return nil, fmt.Errorf("duplicate client_id: %s", client.ID)
		}
		client.devMode = !coreConfig.StrictMode
		storage.clients[client.ID] = client
	}

	provider, err := newOIDCProvider(storage, httpInterfaces, extraOptions)
	if err != nil {
		return nil, err
	}
	return &Component{
		provider:        provider,
		storage:         storage,
		callbackURLFunc: op.AuthCallbackURL(provider),
	}, nil
}

func newOIDCProvider(storage op.Storage, httpInterfaces httpComponent.InterfaceInfo, extraOptions []op.Option) (*op.Provider, error) {
	config := &op.Config{
		// enable code_challenge_method S256 for PKCE (and therefore PKCE in general)
		CodeMethodS256: true,

		// enables additional client_id/client_secret authentication by form post (not only HTTP Basic Auth)
		AuthMethodPost: true,

		// enables use of the `request` Object parameter
		RequestObjectSupported: true,

		// TODO: This depends on whatever is supported through GF AuthN
		SupportedScopes: []string{
			oidc.ScopeOpenID,
			oidc.ScopeProfile,
			oidc.ScopeEmail,
		},

		// TODO: This depends on whatever is supported through GF AuthN
		SupportedClaims: []string{
			"sub",
			"aud",
			"exp",
			"iat",
			"iss",
			"auth_time",
			"nonce",
			"c_hash",
			"at_hash",
			"scopes",
			"client_id",
			"name",
			"email",
		},
	}

	internalBaseURL := httpInterfaces.Internal().URL()
	op.DefaultEndpoints = &op.Endpoints{
		// Privately available endpoints
		Token:         op.NewEndpointWithURL(tokenEndpointPath, internalBaseURL.JoinPath(tokenEndpointPath).String()),
		Introspection: op.NewEndpointWithURL(tokenIntrospectionEndpointPath, internalBaseURL.JoinPath(tokenIntrospectionEndpointPath).String()),
		// Publicly available endpoints
		Authorization: op.NewEndpointWithURL(authorizationEndpointPath, internalBaseURL.JoinPath(authorizationEndpointPath).String()),
		// Unsupported endpoints (for now)
		Revocation:          op.DefaultEndpoints.Revocation,
		Userinfo:            op.DefaultEndpoints.Userinfo,
		EndSession:          op.DefaultEndpoints.EndSession,
		JwksURI:             op.DefaultEndpoints.JwksURI,
		DeviceAuthorization: op.DefaultEndpoints.DeviceAuthorization,
	}

	opts := append([]op.Option{
		// TODO
		//op.WithLogger(logrus.StandardLogger()),
	}, extraOptions...)
	handler, err := op.NewProvider(config, storage,
		func(insecure bool) (op.IssuerFromRequest, error) {
			return func(r *http.Request) string {
				return httpInterfaces.Public().URL().JoinPath("auth").String()
			}, nil
		}, opts...)
	if err != nil {
		return nil, err
	}
	return handler, nil
}
