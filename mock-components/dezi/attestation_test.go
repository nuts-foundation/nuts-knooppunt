package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/stretchr/testify/require"
)

const testJKU = "https://mock-dezi:8443/dezi/jwks.json"

// testKey is generated once per package run. 2048 bits keeps key generation off
// the critical path of every test.
var testKey = func() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return key
}()

func testSigner(t *testing.T) *Signer {
	t.Helper()
	return NewSigner(testKey, "sandbox-dezi-1", "https://abonnee.dezi.nl", testJKU)
}

func TestAttestationCarriesRequiredClaims(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	serialized, err := testSigner(t).Attestation(DrElAmrani, now, time.Hour)
	require.NoError(t, err)

	token, err := jwt.Parse([]byte(serialized), jwt.WithVerify(false), jwt.WithValidate(false))
	require.NoError(t, err)

	require.NotEmpty(t, token.JwtID(), "jti becomes the credential id")
	require.Equal(t, "https://abonnee.dezi.nl", token.Issuer())
	require.Equal(t, now.Unix(), token.NotBefore().Unix())
	require.Equal(t, now.Add(time.Hour).Unix(), token.Expiration().Unix())

	for claim, want := range map[string]string{
		"dezi_nummer":    "900001234",
		"voorletters":    "S.",
		"achternaam":     "el Amrani",
		"abonnee_nummer": "00000010",
		"abonnee_naam":   "Ziekenhuis De Plataan",
		"rol_code":       "01.022",
		"rol_naam":       "Klinisch geriater",
		"rol_code_bron":  "http://www.dezi.nl/rol_bron/big",
	} {
		got, ok := token.Get(claim)
		require.True(t, ok, "missing claim %s", claim)
		require.Equal(t, want, got, "claim %s", claim)
	}
}

func TestAttestationHeaderHasKidAndHTTPSJKU(t *testing.T) {
	serialized, err := testSigner(t).Attestation(DrElAmrani, time.Now(), time.Hour)
	require.NoError(t, err)

	msg, err := jws.Parse([]byte(serialized))
	require.NoError(t, err)
	require.Len(t, msg.Signatures(), 1)

	headers := msg.Signatures()[0].ProtectedHeaders()
	require.Equal(t, "sandbox-dezi-1", headers.KeyID(), "jwx requires kid when jku is used")
	require.Equal(t, testJKU, headers.JWKSetURL())
}

func TestJWKSetContainsTheSigningKey(t *testing.T) {
	raw, err := testSigner(t).JWKSet()
	require.NoError(t, err)

	set, err := jwk.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, 1, set.Len())

	key, ok := set.LookupKeyID("sandbox-dezi-1")
	require.True(t, ok)
	require.Equal(t, "RSA", key.KeyType().String())

	// Regression guard: a private RSA JWK also reports KeyType() == "RSA" under
	// the same kid, so the assertions above would still pass if JWKSet() were
	// changed to serialize s.key instead of s.key.Public(), leaking the private
	// key over the unauthenticated JWKS endpoint. Assert on the serialized JSON
	// itself, since that is what actually reaches the network, and confirm none
	// of the RSA private-key fields are present.
	var parsed struct {
		Keys []map[string]any `json:"keys"`
	}
	require.NoError(t, json.Unmarshal(raw, &parsed))
	require.Len(t, parsed.Keys, 1)
	for _, field := range []string{"d", "p", "q", "dp", "dq", "qi"} {
		_, present := parsed.Keys[0][field]
		require.False(t, present, "serialized JWK must not contain private field %q", field)
	}
}
