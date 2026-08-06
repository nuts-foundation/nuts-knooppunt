package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// nutsConfig separates the URL this process calls from the URL the node
// advertises as its own issuer. In compose they differ: the sandbox reaches
// the node by service name, while the issuer must stay exactly what the node
// publishes, because the audience check compares the two strings verbatim.
type nutsConfig struct {
	InternalBaseURL     string
	Subject             string
	Scope               string
	AuthorizationServer string
	FacilityType        string
}

func nutsConfigFromEnv() nutsConfig {
	return nutsConfig{
		InternalBaseURL:     envOr("NUTS_INTERNAL_BASE_URL", "http://localhost:8081"),
		Subject:             envOr("SANDBOX_NUTS_SUBJECT", "plataan"),
		Scope:               envOr("SANDBOX_BGZ_SCOPE", "bgz"),
		AuthorizationServer: envOr("SANDBOX_AUTH_SERVER", "http://localhost:8080/nuts/oauth2/plataan"),
		FacilityType:        envOr("SANDBOX_FACILITY_TYPE", "Z3"),
	}
}

// organizationContextCredential carries the one claim nothing else on this
// path supplies. The UZI certificate's SAN holds the URA and the Dezi
// attestation holds the practitioner, but neither knows a facility type.
//
// It deliberately sets no issuer and no credentialSubject.id: the token
// endpoint rejects a request credential carrying either, and the node's client
// fills both in from the requester's DID before the credential is verified.
//
// This is a caller assertion. It demonstrates the plumbing, not organizational
// assurance, and nothing ties its facility type to the URA the X509 credential
// carries.
func organizationContextCredential(cfg nutsConfig) map[string]any {
	return map[string]any{
		"@context":          []string{"https://www.w3.org/2018/credentials/v1"},
		"type":              []string{"VerifiableCredential", "SandboxOrganizationContextCredential"},
		"credentialSubject": map[string]any{"facilityType": cfg.FacilityType},
	}
}

type nutsClient struct {
	cfg  nutsConfig
	http *http.Client
}

func newNutsClient(cfg nutsConfig) *nutsClient {
	return &nutsClient{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second}}
}

// requestToken exchanges the practitioner's session for a service access
// token. The attestation goes in as id_token, from which the node builds the
// Dezi credential itself; the practitioner and role are therefore not
// restated here.
func (c *nutsClient) requestToken(ctx context.Context, session authSession) (string, error) {
	body, err := json.Marshal(map[string]any{
		"authorization_server": c.cfg.AuthorizationServer,
		"scope":                c.cfg.Scope,
		// Bearer is explicit: the node defaults to DPoP, which would need a
		// possession proof bound to method, URL and token.
		"token_type":  "Bearer",
		"id_token":    session.Attestation,
		"credentials": []map[string]any{organizationContextCredential(c.cfg)},
	})
	if err != nil {
		return "", fmt.Errorf("request service access token: %w", err)
	}

	url := c.cfg.InternalBaseURL + "/nuts/internal/auth/v2/" + c.cfg.Subject + "/request-service-access-token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("request service access token: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request service access token: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return "", fmt.Errorf("request service access token: status %d: %s", res.StatusCode, detail)
	}

	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&token); err != nil {
		return "", fmt.Errorf("request service access token: decode response: %w", err)
	}
	if token.AccessToken == "" {
		return "", fmt.Errorf("request service access token: response carried no access_token")
	}
	return token.AccessToken, nil
}

// introspect resolves the token into the claims a PEP would put in the PDP's
// input.subject. The token response itself carries only the opaque token, so
// this is the only way to see what the authorization flow will consume.
func (c *nutsClient) introspect(ctx context.Context, token string) (map[string]any, error) {
	form := url.Values{"token": {token}}
	endpoint := c.cfg.InternalBaseURL + "/nuts/internal/auth/v2/accesstoken/introspect"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("introspect access token: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("introspect access token: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspect access token: status %d", res.StatusCode)
	}

	var claims map[string]any
	if err := json.NewDecoder(res.Body).Decode(&claims); err != nil {
		return nil, fmt.Errorf("introspect access token: decode response: %w", err)
	}
	return claims, nil
}
