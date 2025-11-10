package authn

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

var _ op.Storage = (*Storage)(nil)
var _ op.ClientCredentialsStorage = (*Storage)(nil)

const TokenLifetime = 5 * time.Minute

type Storage struct {
	clients map[string]Client
	// TODO: Change to GF AuthN tokens
	tokens *sync.Map
}

func (o Storage) ClientCredentials(ctx context.Context, clientID, clientSecret string) (op.Client, error) {
	client, err := o.getClientByID(clientID)
	if err != nil {
		return nil, err
	}
	err = o.AuthorizeClientIDSecret(ctx, clientID, clientSecret)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (o Storage) ClientCredentialsTokenRequest(ctx context.Context, clientID string, scopes []string) (op.TokenRequest, error) {
	client, err := o.getClientByID(clientID)
	if err != nil {
		return nil, err
	}
	// TODO: Subject should be related to GF AuthN subject
	return &ClientCredentialsTokenRequest{
		Subject: client.ID,
		// TODO: Audience
		Audience: []string{"TODO(audience)"},
		Scopes:   scopes,
	}, nil
}

func (o Storage) CreateAuthRequest(ctx context.Context, request *oidc.AuthRequest, _ string) (op.AuthRequest, error) {
	return nil, errors.New("CreateAuthRequest(): not implemented")
}

func (o Storage) AuthRequestByID(ctx context.Context, id string) (op.AuthRequest, error) {
	return nil, errors.New("AuthRequestByID(): not implemented")
}

func (o Storage) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	return nil, errors.New("AuthRequestByCode(): not implemented")
}

func (o Storage) SaveAuthCode(ctx context.Context, id string, code string) error {
	return errors.New("SaveAuthCode(): not implemented")
}

func (o Storage) DeleteAuthRequest(ctx context.Context, id string) error {
	return errors.New("DeleteAuthRequest(): not implemented")
}

func (o Storage) CreateAccessToken(ctx context.Context, request op.TokenRequest) (accessTokenID string, expiration time.Time, err error) {
	_, ok := request.(*ClientCredentialsTokenRequest)
	if !ok {
		return "", time.Time{}, fmt.Errorf("invalid token request: %T", request)
	}

	// TODO: Use Nuts/GF AuthN here instead
	token := &Token{
		ID:             uuid.NewString(),
		Audience:       request.GetAudience(),
		Scopes:         request.GetScopes(),
		ExpirationTime: time.Now().Add(TokenLifetime),
	}
	o.tokens.Store(token.ID, token)
	return token.ID, token.ExpirationTime, nil
}

func (o Storage) SigningKey(ctx context.Context) (op.SigningKey, error) {
	// TODO
	return nil, errors.New("SigningKey(): not implemented")
}

func (o Storage) SignatureAlgorithms(ctx context.Context) ([]jose.SignatureAlgorithm, error) {
	// TODO
	return []jose.SignatureAlgorithm{
		//o.signingKey.SignatureAlgorithm(),
	}, nil
}

func (o Storage) KeySet(ctx context.Context) ([]op.Key, error) {
	return []op.Key{
		// TODO
		//o.signingKey.Public(),
	}, nil
}

func (o Storage) GetClientByClientID(ctx context.Context, clientID string) (op.Client, error) {
	return o.getClientByID(clientID)
}

func (o Storage) getClientByID(clientID string) (*Client, error) {
	client, ok := o.clients[clientID]
	if !ok {
		return nil, errors.New("client not found")
	}
	return &client, nil
}

func (o Storage) AuthorizeClientIDSecret(ctx context.Context, clientID, clientSecret string) error {
	client, err := o.getClientByID(clientID)
	if err != nil {
		return err
	}
	// TODO: Do we want to use a hashed secret?
	if client.Secret != clientSecret {
		return errors.New("invalid client secret")
	}
	return nil
}

// SetUserinfoFromScopes sets the userinfo claims based on the requested scopes and user ID.
// Since we don't want to store the userinfo in the database, we just return nil here.
// User info should then be set through SetUserinfoFromRequest
func (o Storage) SetUserinfoFromScopes(ctx context.Context, userinfo *oidc.UserInfo, userID, clientID string, scopes []string) error {
	return errors.New("SetUserinfoFromScopes(): not implemented")
}

func (o Storage) SetUserinfoFromRequest(ctx context.Context, userinfo *oidc.UserInfo, request op.IDTokenRequest, scopes []string) error {
	return errors.New("SetUserinfoFromRequest(): not implemented")
}

func (o Storage) SetUserinfoFromToken(ctx context.Context, userInfo *oidc.UserInfo, tokenID, subject, origin string) error {
	return errors.New("SetUserinfoFromToken(): not implemented")
}

func (o Storage) GetKeyByIDAndClientID(ctx context.Context, keyID, clientID string) (*jose.JSONWebKey, error) {
	//TODO implement me
	panic("GetKeyByIDAndClientID")
}

func (o Storage) ValidateJWTProfileScopes(ctx context.Context, userID string, scopes []string) ([]string, error) {
	//TODO implement me
	panic("ValidateJWTProfileScopes")
}

func (o Storage) GetPrivateClaimsFromScopes(ctx context.Context, userID, clientID string, scopes []string) (map[string]any, error) {
	// No private claims
	return nil, nil
}

func (o Storage) Health(ctx context.Context) error {
	// OK
	return nil
}

func (o Storage) SetIntrospectionFromToken(ctx context.Context, userinfo *oidc.IntrospectionResponse, tokenID, subject, clientID string) error {
	// TODO: change to GF AuthN token introspection
	tokenRaw, ok := o.tokens.Load(tokenID)
	if !ok {
		return errors.New("token not found")
	}
	token, _ := tokenRaw.(*Token)
	userinfo.Active = time.Now().Before(token.ExpirationTime)
	userinfo.Audience = token.Audience
	userinfo.Scope = token.Scopes
	return nil
}

func (o Storage) CreateAccessAndRefreshTokens(ctx context.Context, request op.TokenRequest, currentRefreshToken string) (accessTokenID string, newRefreshTokenID string, expiration time.Time, err error) {
	return "", "", time.Time{}, errors.New("refresh tokens not supported")
}

func (o Storage) TokenRequestByRefreshToken(ctx context.Context, refreshTokenID string) (op.RefreshTokenRequest, error) {
	return nil, errors.New("refresh tokens not supported")
}

func (o Storage) TerminateSession(ctx context.Context, userID string, clientID string) error {
	return errors.New("logout not supported")
}

func (o Storage) RevokeToken(ctx context.Context, tokenOrTokenID string, userID string, clientID string) *oidc.Error {
	return &oidc.Error{
		ErrorType:   "invalid_request",
		Description: "token revocation is not supported",
	}
}

func (o Storage) GetRefreshTokenInfo(ctx context.Context, clientID string, token string) (userID string, tokenID string, err error) {
	return "", "", errors.New("refresh tokens not supported")
}

type Token struct {
	ID       string
	Audience []string
	Scopes   []string

	ExpirationTime time.Time
}
