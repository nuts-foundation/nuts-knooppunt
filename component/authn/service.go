package authn

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/nuts-foundation/nuts-knooppunt/cmd/core"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/authn/html"
	httpComponent "github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

var _ component.Lifecycle = (*Component)(nil)

const (
	keysEndpointPath                  = "/auth/keys"
	tokenEndpointPath                 = "/auth/token"
	tokenIntrospectionEndpointPath    = "/auth/introspect"
	authorizationEndpointPath         = "/auth/authorize"
	authorizationCallbackEndpointPath = "/auth/authorize/callback"
	loginFormEndpointPath             = "/auth/login"
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
		authorizationCallbackEndpointPath,
	},
	internalEndpoints: []string{
		keysEndpointPath,
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
	config          Config
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
	publicMux.HandleFunc("GET "+loginFormEndpointPath, html.RenderLogin)
	publicMux.HandleFunc("POST "+loginFormEndpointPath, html.HandleLoginSubmit(op.AuthCallbackURL(c.provider), func(authRequestID string, deziToken string) error {
		return c.storage.AuthenticateUser(authRequestID, deziToken)
	}))
	for _, endpoint := range endpointConfig.internalEndpoints {
		internalMux.Handle(endpoint, c.provider)
	}
}

func New(config Config, httpInterfaces httpComponent.InterfaceInfo, coreConfig core.Config) (*Component, error) {
	var extraOptions []op.Option
	if !coreConfig.StrictMode {
		extraOptions = append(extraOptions, op.WithAllowInsecure())
	}

	// Generate signing key for the OpenID Provider, used to sign the id_token
	// TODO: Might want to change this to a configurable key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate private key: %w", err)
	}
	storage := &Storage{
		clients:      make(map[string]Client),
		authRequests: &sync.Map{},
		tokens:       &sync.Map{},
		signingKey: SigningKey{
			id:           uuid.NewString(),
			sigAlgorithm: "RS256",
			key:          privateKey,
		},
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
		config:          config,
	}, nil
}

func newOIDCProvider(storage op.Storage, httpInterfaces httpComponent.InterfaceInfo, extraOptions []op.Option) (*op.Provider, error) {
	config := &op.Config{
		CodeMethodS256: true,
		AuthMethodPost: true,
		// TODO: This depends on whatever is supported through GF AuthN
		SupportedScopes: []string{
			oidc.ScopeOpenID,
			oidc.ScopeProfile,
		},
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
		},
	}

	internalBaseURL := httpInterfaces.Internal().URL()
	publicBaseURL := httpInterfaces.Public().URL()
	op.DefaultEndpoints = &op.Endpoints{
		// Privately available endpoints
		Token:         op.NewEndpointWithURL(tokenEndpointPath, internalBaseURL.JoinPath(tokenEndpointPath).String()),
		Introspection: op.NewEndpointWithURL(tokenIntrospectionEndpointPath, internalBaseURL.JoinPath(tokenIntrospectionEndpointPath).String()),
		JwksURI:       op.NewEndpointWithURL(keysEndpointPath, internalBaseURL.JoinPath(keysEndpointPath).String()),
		// Publicly available endpoints
		Authorization: op.NewEndpointWithURL(authorizationEndpointPath, publicBaseURL.JoinPath(authorizationEndpointPath).String()),
		// Unsupported endpoints (for now)
		Revocation:          op.DefaultEndpoints.Revocation,
		Userinfo:            op.DefaultEndpoints.Userinfo,
		EndSession:          op.DefaultEndpoints.EndSession,
		DeviceAuthorization: op.DefaultEndpoints.DeviceAuthorization,
	}

	opts := append([]op.Option{
		// Do this when switched to slog
		//op.WithLogger(logrus.StandardLogger()),
	}, extraOptions...)
	handler, err := op.NewProvider(config, storage,
		func(insecure bool) (op.IssuerFromRequest, error) {
			return func(r *http.Request) string {
				return httpInterfaces.Internal().URL().JoinPath("auth").String()
			}, nil
		}, opts...)
	if err != nil {
		return nil, err
	}
	return handler, nil
}
