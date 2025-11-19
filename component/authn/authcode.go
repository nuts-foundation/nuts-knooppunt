package authn

import (
	"errors"
	"fmt"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

var _ op.AuthRequest = (*AuthRequest)(nil)

type AuthRequest struct {
	oidc.AuthRequest
	ID string

	Subject         string
	DEZIToken       string
	ParsedDEZIToken *jwt.Token
	AuthTime        time.Time
	AuthDone        bool
	Code            string
	ApplicationID   string

	ExpirationTime time.Time
}

func (a *AuthRequest) Authenticate(deziToken string) error {
	if a.AuthDone {
		return errors.New("already authenticated")
	}
	// Parse DEZI token
	// TODO: Need to actually verify when really supporting DEZI
	parsedToken, err := jwt.Parse([]byte(deziToken), jwt.WithVerify(false))
	if err != nil {
		return fmt.Errorf("parse DEZI token: %w", err)
	}
	// Resolve subject from DEZI token
	var ok bool
	a.Subject, ok = parsedToken.PrivateClaims()["dezi_nummer"].(string)
	if !ok {
		return errors.New("dezi_nummer claim missing or invalid in DEZI token")
	}

	a.ParsedDEZIToken = &parsedToken
	a.DEZIToken = deziToken
	a.AuthDone = true
	a.AuthTime = time.Now()
	return nil
}

func (a AuthRequest) GetID() string {
	return a.ID
}

func (a AuthRequest) GetACR() string {
	return ""
}

func (a AuthRequest) GetAMR() []string {
	return nil
}

func (a AuthRequest) GetAudience() []string {
	return []string{a.ClientID}
}

func (a AuthRequest) GetAuthTime() time.Time {
	return a.AuthTime
}

func (a AuthRequest) GetClientID() string {
	return a.ClientID
}

func (a AuthRequest) GetCodeChallenge() *oidc.CodeChallenge {
	if a.CodeChallenge == "" && a.CodeChallengeMethod == "" {
		return nil
	}
	return &oidc.CodeChallenge{
		Challenge: a.CodeChallenge,
		Method:    a.CodeChallengeMethod,
	}
}

func (a AuthRequest) GetNonce() string {
	return a.Nonce
}

func (a AuthRequest) GetScopes() []string {
	return a.Scopes
}

func (a AuthRequest) GetSubject() string {
	return a.Subject
}

func (a AuthRequest) Done() bool {
	return a.AuthDone
}
