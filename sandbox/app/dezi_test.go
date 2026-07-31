package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeDezi stands in for mock-components/dezi at the HTTP boundary.
func fakeDezi(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		if r.PostFormValue("code_verifier") == "" || r.PostFormValue("code") != "the-code" {
			http.Error(w, "invalid_grant", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer"})
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at" {
			http.Error(w, "invalid_token", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"verklaring_id": "v-1",
			"verklaring":    fixtureAttestation,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAuthorizeURLCarriesPKCEAndState(t *testing.T) {
	client := newDeziClient(deziConfig{
		PublicAuthorizeURL: "http://localhost:8092/authorize",
		RedirectURI:        "http://localhost:8091/demo/auth/callback",
		ClientID:           "gf-sandbox",
	})

	raw := client.authorizeURL("state-1", "verifier-value-that-is-long-enough-000000")
	parsed, err := url.Parse(raw)
	require.NoError(t, err)

	q := parsed.Query()
	require.Equal(t, "code", q.Get("response_type"))
	require.Equal(t, "gf-sandbox", q.Get("client_id"))
	require.Equal(t, "state-1", q.Get("state"))
	require.Equal(t, "S256", q.Get("code_challenge_method"))
	require.NotEmpty(t, q.Get("code_challenge"))
	require.NotEqual(t, "verifier-value-that-is-long-enough-000000", q.Get("code_challenge"),
		"the verifier itself must never leave the backend")
}

func TestExchangeBuildsASessionFromTheAttestation(t *testing.T) {
	dezi := fakeDezi(t)
	client := newDeziClient(deziConfig{
		InternalBaseURL: dezi.URL,
		RedirectURI:     "http://localhost:8091/demo/auth/callback",
		ClientID:        "gf-sandbox",
	})

	session, err := client.exchange(context.Background(), "the-code", "verifier-value-that-is-long-enough-000000")
	require.NoError(t, err)

	require.Equal(t, "900001234", session.DeziNumber)
	require.Equal(t, "S.", session.Initials)
	require.Equal(t, "el Amrani", session.Surname)
	require.Equal(t, "00000010", session.OrgURA)
	require.Equal(t, "Ziekenhuis De Plataan", session.OrgName)
	require.Equal(t, "01.022", session.RoleCode)
	require.Equal(t, fixtureAttestation, session.Attestation, "the raw attestation is kept for E4")
	require.False(t, session.ExpiresAt.IsZero())
}

func TestExchangeFailsOnBadCode(t *testing.T) {
	dezi := fakeDezi(t)
	client := newDeziClient(deziConfig{InternalBaseURL: dezi.URL, ClientID: "gf-sandbox"})

	_, err := client.exchange(context.Background(), "wrong-code", "verifier-value-that-is-long-enough-000000")
	// The message pins the failure to the /token endpoint's rejection (HTTP
	// 400), not just any error: a bare require.Error would also pass if the
	// status check were deleted, since decoding "invalid_grant" as JSON fails
	// too, for an unrelated reason.
	require.ErrorContains(t, err, "status 400")
}
