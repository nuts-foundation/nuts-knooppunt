package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// deziConfig separates the URL the browser is redirected to from the URL the
// backend calls. In compose they differ: the browser cannot resolve the service
// name, and the container must not call localhost.
type deziConfig struct {
	PublicAuthorizeURL string
	InternalBaseURL    string
	RedirectURI        string
	ClientID           string
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func deziConfigFromEnv() deziConfig {
	return deziConfig{
		PublicAuthorizeURL: envOr("DEZI_PUBLIC_AUTHORIZE_URL", "http://localhost:8092/authorize"),
		InternalBaseURL:    envOr("DEZI_INTERNAL_BASE_URL", "http://localhost:8092"),
		// Trailing slashes are trimmed because the result is compared for
		// exact equality: this mock binds redirect_uri at /authorize and
		// checks it again at /token, and a real provider matches it against a
		// registered value. A configured "https://host/" would otherwise
		// produce "https://host//demo/auth/callback".
		RedirectURI: strings.TrimRight(envOr("SANDBOX_PUBLIC_URL", "http://localhost:8091"), "/") + "/demo/auth/callback",
		ClientID:    "gf-sandbox",
	}
}

type deziClient struct {
	cfg  deziConfig
	http *http.Client
}

func newDeziClient(cfg deziConfig) *deziClient {
	return &deziClient{cfg: cfg, http: &http.Client{Timeout: 10 * time.Second}}
}

func randomURLSafe() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (c *deziClient) authorizeURL(state, verifier string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {c.cfg.ClientID},
		"redirect_uri":          {c.cfg.RedirectURI},
		"state":                 {state},
		"code_challenge":        {challenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	separator := "?"
	if strings.Contains(c.cfg.PublicAuthorizeURL, "?") {
		separator = "&"
	}
	return c.cfg.PublicAuthorizeURL + separator + q.Encode()
}

// exchange redeems the authorization code and reads the attestation. Nothing
// here is logged: the token and the attestation are credentials.
func (c *deziClient) exchange(ctx context.Context, code, verifier string) (authSession, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"client_id":     {c.cfg.ClientID},
		"redirect_uri":  {c.cfg.RedirectURI},
	}
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.InternalBaseURL+"/token", strings.NewReader(form.Encode()))
	if err != nil {
		return authSession{}, err
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	tokenRes, err := c.http.Do(tokenReq)
	if err != nil {
		return authSession{}, fmt.Errorf("token request: %w", err)
	}
	defer tokenRes.Body.Close()
	if tokenRes.StatusCode != http.StatusOK {
		return authSession{}, fmt.Errorf("token request: status %d", tokenRes.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenRes.Body).Decode(&token); err != nil {
		return authSession{}, fmt.Errorf("decode token response: %w", err)
	}

	infoReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.InternalBaseURL+"/userinfo", nil)
	if err != nil {
		return authSession{}, err
	}
	infoReq.Header.Set("Authorization", "Bearer "+token.AccessToken)

	infoRes, err := c.http.Do(infoReq)
	if err != nil {
		return authSession{}, fmt.Errorf("userinfo request: %w", err)
	}
	defer infoRes.Body.Close()
	if infoRes.StatusCode != http.StatusOK {
		return authSession{}, fmt.Errorf("userinfo request: status %d", infoRes.StatusCode)
	}
	var envelope struct {
		Verklaring string `json:"verklaring"`
	}
	if err := json.NewDecoder(infoRes.Body).Decode(&envelope); err != nil {
		return authSession{}, fmt.Errorf("decode userinfo response: %w", err)
	}
	return sessionFromAttestation(envelope.Verklaring)
}

// sessionFromAttestation reads the claims the interface and E4 need. The
// signature is verified by the Nuts node when the attestation is presented, not
// here; this backend is not the trust boundary.
func sessionFromAttestation(attestation string) (authSession, error) {
	parts := strings.Split(attestation, ".")
	if len(parts) != 3 {
		return authSession{}, fmt.Errorf("attestation is not a compact JWS")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return authSession{}, fmt.Errorf("decode attestation payload: %w", err)
	}
	var claims struct {
		DeziNumber    string `json:"dezi_nummer"`
		Initials      string `json:"voorletters"`
		SurnamePrefix string `json:"voorvoegsel"`
		Surname       string `json:"achternaam"`
		OrgURA        string `json:"abonnee_nummer"`
		OrgName       string `json:"abonnee_naam"`
		RoleCode      string `json:"rol_code"`
		RoleName      string `json:"rol_naam"`
		RoleRegistry  string `json:"rol_code_bron"`
		Expiry        int64  `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		return authSession{}, fmt.Errorf("parse attestation claims: %w", err)
	}
	if claims.DeziNumber == "" || claims.OrgURA == "" {
		return authSession{}, fmt.Errorf("attestation is missing dezi_nummer or abonnee_nummer")
	}
	expires := time.Unix(claims.Expiry, 0)
	if claims.Expiry == 0 {
		expires = time.Now().Add(time.Hour)
	}
	return authSession{
		DeziNumber:    claims.DeziNumber,
		Initials:      claims.Initials,
		SurnamePrefix: claims.SurnamePrefix,
		Surname:       claims.Surname,
		OrgURA:        claims.OrgURA,
		OrgName:       claims.OrgName,
		RoleCode:      claims.RoleCode,
		RoleName:      claims.RoleName,
		RoleRegistry:  claims.RoleRegistry,
		Attestation:   attestation,
		ExpiresAt:     expires,
	}, nil
}
