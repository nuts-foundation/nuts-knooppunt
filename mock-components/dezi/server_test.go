package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	s := NewSigner(testKey, "sandbox-dezi-1", "https://abonnee.dezi.nl", testJKU)
	srv := httptest.NewServer(NewMux(s, DrElAmrani))
	t.Cleanup(srv.Close)
	return srv
}

func challengeFor(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// noRedirect stops the client following the consent redirect so the test can
// inspect the Location header.
func noRedirect(srv *httptest.Server) *http.Client {
	c := srv.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

func authorize(t *testing.T, srv *httptest.Server, verifier string) string {
	t.Helper()
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {"gf-sandbox"},
		"redirect_uri":          {"http://localhost:8091/demo/auth/callback"},
		"state":                 {"state-abc"},
		"code_challenge":        {challengeFor(verifier)},
		"code_challenge_method": {"S256"},
	}
	res, err := noRedirect(srv).Get(srv.URL + "/authorize?" + query.Encode())
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode, "authorize renders the consent screen")

	body := new(strings.Builder)
	_, err = io.Copy(body, res.Body)
	require.NoError(t, err)
	handle := extractHandle(t, body.String())

	consent, err := noRedirect(srv).PostForm(srv.URL+"/authorize", url.Values{"request": {handle}})
	require.NoError(t, err)
	defer consent.Body.Close()
	require.Equal(t, http.StatusSeeOther, consent.StatusCode)

	location, err := url.Parse(consent.Header.Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "state-abc", location.Query().Get("state"))
	code := location.Query().Get("code")
	require.NotEmpty(t, code)
	return code
}

func extractHandle(t *testing.T, page string) string {
	t.Helper()
	const marker = `name="request" value="`
	start := strings.Index(page, marker)
	require.GreaterOrEqual(t, start, 0, "consent page must carry the request handle")
	rest := page[start+len(marker):]
	end := strings.Index(rest, `"`)
	require.GreaterOrEqual(t, end, 0)
	return rest[:end]
}

func exchange(t *testing.T, srv *httptest.Server, code, verifier string) *http.Response {
	t.Helper()
	res, err := srv.Client().PostForm(srv.URL+"/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"client_id":     {"gf-sandbox"},
		"redirect_uri":  {"http://localhost:8091/demo/auth/callback"},
	})
	require.NoError(t, err)
	return res
}

func TestFullFlowReturnsAnAttestation(t *testing.T) {
	srv := testServer(t)
	const verifier = "a-sufficiently-long-code-verifier-value-1234567890"

	code := authorize(t, srv, verifier)

	tokenRes := exchange(t, srv, code, verifier)
	defer tokenRes.Body.Close()
	require.Equal(t, http.StatusOK, tokenRes.StatusCode)

	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	require.NoError(t, json.NewDecoder(tokenRes.Body).Decode(&token))
	require.NotEmpty(t, token.AccessToken)
	require.Equal(t, "Bearer", token.TokenType)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/userinfo", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	infoRes, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer infoRes.Body.Close()
	require.Equal(t, http.StatusOK, infoRes.StatusCode)

	var envelope struct {
		VerklaringID string `json:"verklaring_id"`
		Verklaring   string `json:"verklaring"`
	}
	require.NoError(t, json.NewDecoder(infoRes.Body).Decode(&envelope))
	require.NotEmpty(t, envelope.VerklaringID)
	require.Equal(t, 3, strings.Count(envelope.Verklaring, ".")+1, "verklaring is a compact JWS")
}

func TestTokenRejectsWrongVerifier(t *testing.T) {
	srv := testServer(t)
	const verifier = "a-sufficiently-long-code-verifier-value-1234567890"
	code := authorize(t, srv, verifier)

	res := exchange(t, srv, code, "the-wrong-verifier-entirely-0987654321")
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestAuthorizationCodeIsSingleUse(t *testing.T) {
	srv := testServer(t)
	const verifier = "a-sufficiently-long-code-verifier-value-1234567890"
	code := authorize(t, srv, verifier)

	first := exchange(t, srv, code, verifier)
	first.Body.Close()
	require.Equal(t, http.StatusOK, first.StatusCode)

	replay := exchange(t, srv, code, verifier)
	defer replay.Body.Close()
	require.Equal(t, http.StatusBadRequest, replay.StatusCode, "a code must not be redeemable twice")
}

func TestUserinfoRequiresBearerToken(t *testing.T) {
	srv := testServer(t)
	res, err := srv.Client().Get(srv.URL + "/userinfo")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusUnauthorized, res.StatusCode)
}

func TestJWKSIsServed(t *testing.T) {
	srv := testServer(t)
	res, err := srv.Client().Get(srv.URL + "/dezi/jwks.json")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var set struct {
		Keys []map[string]any `json:"keys"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&set))
	require.Len(t, set.Keys, 1)
}
