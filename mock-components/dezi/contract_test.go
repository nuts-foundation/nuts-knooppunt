package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-node/vcr/credential"
	"github.com/stretchr/testify/require"
)

// serveJWKS starts an HTTPS JWK Set server and points http.DefaultClient at it.
// The Nuts node fetches the jku with no custom client, so httprc falls back to
// http.DefaultClient (httprc@v1.0.6/fetcher.go:143). Swapping it is the only way
// to make the fetch trust a test certificate. Tests using this must not run in
// parallel.
func serveJWKS(t *testing.T, s *Signer) string {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		set, err := s.JWKSet()
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(set)
	}))
	t.Cleanup(srv.Close)

	previous := http.DefaultClient
	http.DefaultClient = srv.Client()
	t.Cleanup(func() { http.DefaultClient = previous })

	return srv.URL + "/dezi/jwks.json"
}

// signedFor mints an attestation whose jku is the running test JWKS server.
// It reuses testKey from attestation_test.go, so the served JWK Set and the
// signature belong to the same key.
func signedFor(t *testing.T, jku string) string {
	t.Helper()
	s := NewSigner(testKey, "sandbox-dezi-1", "https://abonnee.dezi.nl", jku)
	serialized, err := s.Attestation(DrElAmrani, time.Now().Add(-time.Minute), time.Hour)
	require.NoError(t, err)
	return serialized
}

// publisher serves the JWK Set. Its own jku is irrelevant; only the key matters.
func publisher() *Signer {
	return NewSigner(testKey, "sandbox-dezi-1", "https://abonnee.dezi.nl", "")
}

func TestNutsNodeValidatesOurAttestation(t *testing.T) {
	jku := serveJWKS(t, publisher())
	serialized := signedFor(t, jku)

	cred, err := credential.CreateDeziUserCredential(serialized)
	require.NoError(t, err)

	err = credential.DeziUserCredentialValidator{AllowedJKU: []string{jku}}.Validate(*cred)
	require.NoError(t, err, "the Nuts node must accept the mock's attestation")
}

func TestCredentialSubjectMatchesThePersona(t *testing.T) {
	jku := serveJWKS(t, publisher())
	serialized := signedFor(t, jku)

	cred, err := credential.CreateDeziUserCredential(serialized)
	require.NoError(t, err)

	var subjects []credential.DeziIDTokenSubject
	require.NoError(t, cred.UnmarshalCredentialSubject(&subjects))
	require.Len(t, subjects, 1)

	require.Equal(t, "00000010", subjects[0].Identifier)
	require.Equal(t, "Ziekenhuis De Plataan", subjects[0].Name)
	require.Equal(t, "900001234", subjects[0].Employee.Identifier)
	require.Equal(t, "S.", subjects[0].Employee.Initials)
	require.Equal(t, "el Amrani", subjects[0].Employee.Surname)
	require.Equal(t, "01.022", subjects[0].Employee.Role)
}

func TestValidatorRejectsBadAttestations(t *testing.T) {
	jku := serveJWKS(t, publisher())

	t.Run("jku not on the allowlist", func(t *testing.T) {
		serialized := signedFor(t, jku)
		cred, err := credential.CreateDeziUserCredential(serialized)
		require.NoError(t, err)
		err = credential.DeziUserCredentialValidator{
			AllowedJKU: []string{"https://elsewhere.example.com/jwks.json"},
		}.Validate(*cred)
		require.Error(t, err)
	})

	t.Run("credential subject altered after issue", func(t *testing.T) {
		serialized := signedFor(t, jku)
		cred, err := credential.CreateDeziUserCredential(serialized)
		require.NoError(t, err)

		// Change a field the validator re-derives from the id_token and compares.
		var subjects []credential.DeziIDTokenSubject
		require.NoError(t, cred.UnmarshalCredentialSubject(&subjects))
		subjects[0].Employee.Role = "01.047"
		raw, err := json.Marshal(subjects)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &cred.CredentialSubject))

		err = credential.DeziUserCredentialValidator{AllowedJKU: []string{jku}}.Validate(*cred)
		require.Error(t, err, "credential subject must match the id_token claims")
	})

	t.Run("expired attestation", func(t *testing.T) {
		s := NewSigner(testKey, "sandbox-dezi-1", "https://abonnee.dezi.nl", jku)
		serialized, err := s.Attestation(DrElAmrani, time.Now().Add(-2*time.Hour), time.Hour)
		require.NoError(t, err)
		cred, err := credential.CreateDeziUserCredential(serialized)
		require.NoError(t, err)
		err = credential.DeziUserCredentialValidator{AllowedJKU: []string{jku}}.Validate(*cred)
		require.Error(t, err, "an expired attestation must not validate")
	})

	t.Run("wrong kid", func(t *testing.T) {
		s := NewSigner(testKey, "unknown-kid", "https://abonnee.dezi.nl", jku)
		serialized, err := s.Attestation(DrElAmrani, time.Now().Add(-time.Minute), time.Hour)
		require.NoError(t, err)
		cred, err := credential.CreateDeziUserCredential(serialized)
		require.NoError(t, err)
		err = credential.DeziUserCredentialValidator{AllowedJKU: []string{jku}}.Validate(*cred)
		require.Error(t, err, "a kid absent from the JWK Set must not validate")
	})
}
