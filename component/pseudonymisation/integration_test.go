package pseudonymisation

import (
	"os"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd/core"
	"github.com/nuts-foundation/nuts-knooppunt/component/authn"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tlsutil"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestIntegration(t *testing.T) {
	const (
		tokenEndpoint = "https://oauth.proeftuin.gf.irealisatie.nl/oauth/token"
		prsBaseURL    = "https://pseudoniemendienst.proeftuin.gf.irealisatie.nl"
		ura           = "90000311" // adjust this to your own
		uraNVI        = "90000901"
		certFile      = "../authn/cert.pem"
		keyFile       = "../authn/cert-key.pem"
	)

	// Only skip if certificate files don't exist
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		t.Skipf("Certificate file not found: %s", certFile)
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		t.Skipf("Key file not found: %s", keyFile)
	}

	authnComponent, err := authn.New(authn.Config{
		MinVWS: authn.MinistryAuthConfig{
			Config:        tlsutil.Config{TLSCertFile: certFile, TLSKeyFile: keyFile},
			TokenEndpoint: tokenEndpoint,
		},
	}, nil, core.Config{})
	require.NoError(t, err)

	prsComponent := New(Config{
		PRSBaseURL: prsBaseURL,
	}, authnComponent.MinVWSHTTPClient)
	bsn := fhir.Identifier{
		System: to.Ptr(coding.BSNNamingSystem),
		Value:  to.Ptr("123456789"),
	}
	token, err := prsComponent.IdentifierToToken(t.Context(), bsn, ura, uraNVI, "nationale-verwijsindex")
	require.NoError(t, err)
	t.Logf("Pseudonymized token: %s", *token.Value)
}
