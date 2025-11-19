package authn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const AuthRequestLifetime = 5 * time.Minute

const subjectTokenType = "urn:ietf:params:oauth:token-type:id_token"
const nutsSubjectIDTokenType = "nuts-subject-id"

// var _ op.Storage = (*Storage)(nil)
// var _ op.ClientCredentialsStorage = (*Storage)(nil) // for client credentials grant
var _ op.TokenExchangeStorage = (*Storage)(nil)

const TokenLifetime = 5 * time.Minute

// Storage implements the op.Storage interface using in-memory maps.
// TODO: Change to persistent token storage
type Storage struct {
	clients      map[string]Client
	authRequests *sync.Map
	tokens       *sync.Map
	signingKey   SigningKey
}

func (o Storage) AuthenticateUser(authRequestID string, deziToken string) error {
	authRequestRaw, ok := o.authRequests.Load(authRequestID)
	if !ok {
		return errors.New("auth request not found")
	}
	authRequest := authRequestRaw.(AuthRequest)
	if err := authRequest.Authenticate(deziToken); err != nil {
		return err
	}
	o.authRequests.Store(authRequestID, authRequest)
	log.Ctx(context.Background()).Info().Msgf("OIDC: AuthRequest %s authenticated", authRequestID)
	return nil
}

func (o Storage) ValidateTokenExchangeRequest(ctx context.Context, request op.TokenExchangeRequest) error {
	if request.GetExchangeSubjectTokenType() != "" && request.GetExchangeSubjectTokenType() != subjectTokenType {
		return fmt.Errorf("unsupported subject token type: %s (expected '%s')", request.GetExchangeSubjectTokenType(), subjectTokenType)
	}
	if request.GetExchangeActorTokenType() != nutsSubjectIDTokenType {
		return fmt.Errorf("unsupported actor token type: %s (expected '%s')", request.GetExchangeActorTokenType(), nutsSubjectIDTokenType)
	}
	if len(request.GetAudience()) != 1 {
		return fmt.Errorf("exactly one audience must be specified")
	}
	if request.GetRequestedTokenType() != oidc.AccessTokenType {
		return fmt.Errorf("unsupported requested token type: %s (expected '%s')", request.GetRequestedTokenType(), oidc.AccessTokenType)
	}
	return nil
}

func (o Storage) CreateTokenExchangeRequest(ctx context.Context, request op.TokenExchangeRequest) error {
	// Auditing could happen here
	return nil
}

func (o Storage) GetPrivateClaimsFromTokenExchangeRequest(ctx context.Context, request op.TokenExchangeRequest) (claims map[string]any, err error) {
	// Token request is forwarded to GF Authentication, so no private claims to return
	return nil, nil
}

func (o Storage) SetUserinfoFromTokenExchangeRequest(ctx context.Context, userinfo *oidc.UserInfo, request op.TokenExchangeRequest) error {
	panic("SetUserinfoFromTokenExchangeRequest(): implement me")
}

func (o Storage) CreateAuthRequest(ctx context.Context, request *oidc.AuthRequest, userID string) (op.AuthRequest, error) {
	if len(userID) != 0 {
		return nil, errors.New("token refresh not supported")
	}
	log.Ctx(ctx).Info().Msgf("OIDC: AuthRequest received (client: %s)", request.ClientID)
	authRequestID := uuid.NewString()
	req := AuthRequest{
		ID:             authRequestID,
		AuthRequest:    *request,
		ExpirationTime: time.Now().Add(AuthRequestLifetime),
	}
	o.authRequests.Store(authRequestID, req)
	return &req, nil
}

func (o Storage) AuthRequestByID(ctx context.Context, id string) (op.AuthRequest, error) {
	authRequestRaw, ok := o.authRequests.Load(id)
	if !ok {
		return nil, errors.New("auth request not found")
	}
	authRequest, _ := authRequestRaw.(AuthRequest)
	if time.Now().After(authRequest.ExpirationTime) {
		o.authRequests.Delete(id)
		return nil, errors.New("auth request expired")
	}
	return &authRequest, nil
}

func (o Storage) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	var authRequest *AuthRequest
	o.authRequests.Range(func(key, value any) bool {
		curr := value.(AuthRequest)
		if curr.Code == code {
			authRequest = &curr
			return false
		}
		return true
	})
	if authRequest == nil {
		return nil, errors.New("auth request not found")
	}
	if time.Now().After(authRequest.ExpirationTime) {
		return nil, errors.New("auth request expired")
	}
	return authRequest, nil
}

func (o Storage) SaveAuthCode(ctx context.Context, id string, code string) error {
	reqRaw, err := o.AuthRequestByID(ctx, id)
	if err != nil {
		return err
	}
	req := reqRaw.(*AuthRequest)
	req.Code = code
	o.authRequests.Store(id, *req)
	return nil
}

func (o Storage) DeleteAuthRequest(ctx context.Context, id string) error {
	o.authRequests.Delete(id)
	return nil
}

func (o Storage) CreateAccessToken(ctx context.Context, request op.TokenRequest) (accessTokenID string, expiration time.Time, err error) {
	_, ok := request.(*AuthRequest)
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
	return o.signingKey, nil
}

func (o Storage) SignatureAlgorithms(ctx context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{
		o.signingKey.SignatureAlgorithm(),
	}, nil
}

func (o Storage) KeySet(ctx context.Context) ([]op.Key, error) {
	return []op.Key{
		o.signingKey.Public(),
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
	return nil
}

func (o Storage) SetUserinfoFromRequest(ctx context.Context, userinfo *oidc.UserInfo, request op.IDTokenRequest, scopes []string) error {
	authRequest := request.(*AuthRequest)
	deziTokenClaims := (*authRequest.ParsedDEZIToken).PrivateClaims()
	userinfo.Subject = authRequest.Subject
	userinfo.FamilyName = formatName(deziTokenClaims["voorvoegsel"].(string), deziTokenClaims["achternaam"].(string))
	userinfo.GivenName = deziTokenClaims["voorletters"].(string)
	userinfo.Name = formatName(userinfo.GivenName, userinfo.FamilyName)
	// copy all DEZI token claims into userinfo claims
	userinfo.AppendClaims("dezi_token", authRequest.DEZIToken)
	userinfo.AppendClaims("dezi_claims", deziTokenClaims)
	return nil
}

func formatName(parts ...string) string {
	nameParts := []string{}
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			nameParts = append(nameParts, part)
		}
	}
	return strings.Join(nameParts, " ")
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
