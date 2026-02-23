package pseudonimization

import (
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/authn"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tlsutil"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestIntegration(t *testing.T) {
	t.Skip()
	const (
		tokenEndpoint = "https://oauth.proeftuin.gf.irealisatie.nl/oauth/token"
		prsBaseURL    = "https://pseudoniemendienst.proeftuin.gf.irealisatie.nl"
		ura           = "90000311"
		uraNVI        = "90000002"
	)
	httpClient, err := authn.HTTPClient(t.Context(), []string{"prs:read"}, ura, prsBaseURL, authn.MinistryAuthConfig{
		Config:        tlsutil.Config{TLSCertFile: "../authn/cert.pem", TLSKeyFile: "../authn/cert-key.pem"},
		TokenEndpoint: tokenEndpoint,
	})
	require.NoError(t, err)

	service := Component{
		httpClient: httpClient,
		prsURL:     prsBaseURL,
	}
	bsn := fhir.Identifier{
		System: to.Ptr(coding.BSNNamingSystem),
		Value:  to.Ptr("123456789"),
	}
	token, err := service.IdentifierToToken(t.Context(), bsn, uraNVI)
	require.NoError(t, err)
	t.Logf("Pseudonymized token: %s", *token.Value)
}
