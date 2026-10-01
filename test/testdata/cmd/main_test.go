package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeJWT builds a compact JWS with the given iss claim and an arbitrary,
// unverifiable signature segment. walletHoldsFakeUZICredential never
// verifies a wallet entry's signature, so this is enough to exercise it
// without real signing material.
func fakeJWT(t *testing.T, issuer string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]string{"iss": issuer})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return header + "." + base64.RawURLEncoding.EncodeToString(payload) + ".placeholder"
}

func walletServer(t *testing.T, entries []any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(entries); err != nil {
			t.Fatalf("encode wallet response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestWalletHoldsFakeUZICredentialEmptyWallet(t *testing.T) {
	srv := walletServer(t, []any{})
	has, err := walletHoldsFakeUZICredential(srv.URL, "00000010")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Fatal("expected no Fake UZI credential in an empty wallet")
	}
}

func TestWalletHoldsFakeUZICredentialOtherCAOnly(t *testing.T) {
	// The shape a wallet holds after sandbox/bootstrap-nuts.sh issues a
	// credential from the local demo CA, not the Fake UZI CA this function
	// looks for.
	srv := walletServer(t, []any{fakeJWT(t, "did:x509:0:sha256:SOME-OTHER-CA-FINGERPRINT::subject:O:Ziekenhuis%20De%20Plataan")})
	has, err := walletHoldsFakeUZICredential(srv.URL, "00000010")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Fatal("a credential from a different CA must not count as a Fake UZI credential")
	}
}

func TestWalletHoldsFakeUZICredentialMatch(t *testing.T) {
	issuer := fmt.Sprintf("did:x509:0:sha256:%s::subject:O:Ziekenhuis%%20De%%20Plataan", fakeUZIFingerprint)
	srv := walletServer(t, []any{fakeJWT(t, issuer)})
	has, err := walletHoldsFakeUZICredential(srv.URL, "00000010")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Fatal("expected a Fake UZI credential to be detected")
	}
}

func TestWalletHoldsFakeUZICredentialSkipsNonJWTEntries(t *testing.T) {
	// A JSON-LD credential is a JSON object, not a bare string - this
	// codebase never issues one, but the wallet endpoint's response shape
	// allows it, and it must be skipped rather than erroring the lookup.
	jsonLD := map[string]any{"type": []string{"VerifiableCredential", "X509Credential"}}
	srv := walletServer(t, []any{jsonLD})
	has, err := walletHoldsFakeUZICredential(srv.URL, "00000010")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Fatal("a non-JWT entry must not be mistaken for a Fake UZI credential")
	}
}
