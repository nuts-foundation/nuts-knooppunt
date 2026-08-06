package policy

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"os"
	"testing"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/nuts-foundation/go-did/vc"
	"github.com/nuts-foundation/nuts-node/vcr/pe"
	v2 "github.com/nuts-foundation/nuts-node/vcr/pe/schema/v2"
	"github.com/stretchr/testify/require"
)

// The node validates every policy file against this schema while loading the
// directory, so an invalid definition is a startup failure of the whole stack.
// Catching it here turns that into a test failure.
//
// The node validates each scope's mapping (e.g. {"organization": {...}}) on its
// own; see validatingWalletOwnerMapping.UnmarshalJSON in nuts-node's
// policy/local.go. Validating the whole file (as this test used to) is
// vacuous: the WalletOwnerMapping schema only $refs the PresentationDefinition
// schema under the literal "organization" and "user" keys, and otherwise falls
// back to "additionalProperties": {"type": "object"}, which never looks inside
// "bgz". So this test validates raw["bgz"] instead, matching what the node does.
func TestBGZPolicyIsValid(t *testing.T) {
	raw, err := os.ReadFile("bgz.json")
	require.NoError(t, err)

	var scopes map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &scopes))
	scopeRaw, ok := scopes["bgz"]
	require.True(t, ok, "the scope key is what the token request asks for")
	require.NoError(t, v2.Validate(scopeRaw, v2.WalletOwnerMapping))

	var mapping pe.WalletOwnerMapping
	require.NoError(t, json.Unmarshal(scopeRaw, &mapping))

	definition, ok := mapping["organization"]
	require.True(t, ok, "the organization wallet owner must have a definition")

	ids := map[string]bool{}
	for _, d := range definition.InputDescriptors {
		ids[d.Id] = true
	}
	require.Equal(t, map[string]bool{
		"id_uzicert_uracredential":        true,
		"id_dezicredential":               true,
		"id_sandbox_organization_context": true,
	}, ids, "all three credentials must be selected by one definition")
}

// The derived Dezi credential is an LDP VC whose proof type is DeziIDJWT07 for
// the mock's v0.7 attestation. Presentation matching checks the format, so a
// definition that only allows JsonWebSignature2020 silently excludes it.
func TestBGZPolicyAcceptsTheDeziProofType(t *testing.T) {
	raw, err := os.ReadFile("bgz.json")
	require.NoError(t, err)
	var mapping map[string]pe.WalletOwnerMapping
	require.NoError(t, json.Unmarshal(raw, &mapping))

	format := mapping["bgz"]["organization"].Format
	require.NotNil(t, format, "the definition must pin its accepted formats")
	proofTypes := (*format)["ldp_vc"]["proof_type"]
	require.Contains(t, proofTypes, "DeziIDJWT07")
	require.Contains(t, (*format)["jwt_vc"]["alg"], "PS256", "the X509 credential is a JWT VC")
}

// Field ids become introspection claim names verbatim. The PDP reads user_id,
// user_role, organization_ura, organization_name and organization_facility_type
// (component/pdp/shared.go), so a definition emitting user_uzi (as the
// nuts-node exemplar does) would leave PolicySubject.User.Id empty and send an
// empty ProviderID to Mitz.
func TestBGZPolicyEmitsTheClaimNamesThePDPReads(t *testing.T) {
	raw, err := os.ReadFile("bgz.json")
	require.NoError(t, err)
	var mapping map[string]pe.WalletOwnerMapping
	require.NoError(t, json.Unmarshal(raw, &mapping))

	fields := map[string]bool{}
	for _, d := range mapping["bgz"]["organization"].InputDescriptors {
		for _, f := range d.Constraints.Fields {
			if f.Id != nil {
				fields[*f.Id] = true
			}
		}
	}
	for _, want := range []string{"user_id", "user_role", "organization_ura", "organization_name", "organization_facility_type"} {
		require.True(t, fields[want], "the PDP reads %q; see component/pdp/shared.go", want)
	}
	require.False(t, fields["user_uzi"], "user_uzi is the nuts-node exemplar's name and this PDP does not read it")
}

// The tests above only inspect field ids and JSON shape. None of them would
// notice a broken JSON path, or a regex pattern the matcher rejects outright,
// even though either breaks the flow at runtime: the whole input descriptor
// stops matching, the token request fails with missing credentials, and none
// of the tests above warn. This test runs the real matcher (vcr/pe) against
// three credentials shaped like what the sandbox flow presents, and checks
// the resolved claim values instead of the definition's JSON shape.
func TestBGZPolicyMatchesRealCredentials(t *testing.T) {
	raw, err := os.ReadFile("bgz.json")
	require.NoError(t, err)
	var mapping map[string]pe.WalletOwnerMapping
	require.NoError(t, json.Unmarshal(raw, &mapping))
	definition := mapping["bgz"]["organization"]

	// X509 credential: a jwt_vc signed with PS256; the matcher parses the raw
	// JWS and compares the alg header against the definition's jwt_vc.alg.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	x509Token, err := jwt.NewBuilder().
		Subject("did:web:example.com:org").
		Claim("vc", map[string]interface{}{
			"@context": []string{"https://www.w3.org/2018/credentials/v1"},
			"type":     []string{vc.VerifiableCredentialType, "X509Credential"},
			"credentialSubject": map[string]interface{}{
				"id":      "did:web:example.com:org",
				"subject": map[string]interface{}{"O": "Test Care Org"},
				"san":     map[string]interface{}{"otherName": "2.16.528.1.1007.99.2110-1-0-S-00000010-00.000-0"},
			},
		}).Build()
	require.NoError(t, err)
	signedX509, err := jwt.Sign(x509Token, jwt.WithKey(jwa.PS256, rsaKey))
	require.NoError(t, err)
	x509Cred, err := vc.ParseVerifiableCredential(string(signedX509))
	require.NoError(t, err)

	// Dezi credential: an ldp_vc carrying a proof of type DeziIDJWT07.
	deziCred, err := vc.ParseVerifiableCredential(`{
		"@context": ["https://www.w3.org/2018/credentials/v1"],
		"type": ["VerifiableCredential", "DeziUserCredential"],
		"issuer": "did:web:example.com:dezi",
		"issuanceDate": "2024-01-01T00:00:00Z",
		"credentialSubject": {"id": "did:web:example.com:practitioner", "employee": {"identifier": "12345", "role": "nurse"}},
		"proof": {"type": "DeziIDJWT07", "proofPurpose": "assertionMethod", "verificationMethod": "did:web:example.com:dezi#key-1", "created": "2024-01-01T00:00:00Z"}
	}`)
	require.NoError(t, err)

	// Organization context credential: an ldp_vc with no proof at all, admitted
	// by matchFormat's self-attestation early return, so its (nonexistent)
	// proof type is never consulted.
	contextCred, err := vc.ParseVerifiableCredential(`{
		"@context": ["https://www.w3.org/2018/credentials/v1"],
		"type": ["VerifiableCredential", "SandboxOrganizationContextCredential"],
		"issuer": "did:web:example.com:org",
		"issuanceDate": "2024-01-01T00:00:00Z",
		"credentialSubject": {"id": "did:web:example.com:org", "facilityType": "hospital"}
	}`)
	require.NoError(t, err)

	creds, descriptors, err := definition.Match([]vc.VerifiableCredential{*x509Cred, *deziCred, *contextCred})
	require.NoError(t, err, "all three credentials must be selected by the definition")
	require.Len(t, creds, 3)

	credentialMap := make(map[string]vc.VerifiableCredential, len(descriptors))
	for i, cred := range creds {
		credentialMap[descriptors[i].Id] = cred
	}
	fields, err := definition.ResolveConstraintsFields(credentialMap)
	require.NoError(t, err)

	require.Equal(t, map[string]interface{}{
		"organization_id":            "did:web:",
		"organization_name":          "Test Care Org",
		"organization_ura":           "00000010",
		"user_id":                    "12345",
		"user_role":                  "nurse",
		"organization_facility_type": "hospital",
	}, fields, "organization_ura must be extracted from the SAN's URA capture group")
}
