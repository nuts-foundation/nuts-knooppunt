package harness

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	dezimock "github.com/nuts-foundation/nuts-knooppunt/mock-components/dezi"
	"github.com/stretchr/testify/require"
)

// MockDezi is the signing half of the mock Dezi: a key for this run.
type MockDezi struct {
	Signer *dezimock.Signer
}

// NewMockDezi returns a signer for the attestations a full-chain test signs in
// with. Nothing in the chain fetches the jku they name: the sandbox reads the
// practitioner from the attestation without verifying it (sandbox/app/dezi.go).
func NewMockDezi(t *testing.T) MockDezi {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return MockDezi{Signer: dezimock.NewSigner(key, "sandbox-dezi-1", "https://abonnee.dezi.nl", "https://mock-dezi:8443/dezi/jwks.json")}
}
