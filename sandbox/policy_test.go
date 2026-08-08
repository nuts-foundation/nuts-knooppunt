package sandbox

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"github.com/nuts-foundation/go-did/vc"
	"github.com/nuts-foundation/nuts-node/vcr/pe"
	v2 "github.com/nuts-foundation/nuts-node/vcr/pe/schema/v2"
	"github.com/stretchr/testify/require"
)

// The definition lives here as a template rather than in config/policy because
// it has to pin the demo CA's did:x509 fingerprint, and that CA is regenerated
// on every rotation. A committed file cannot carry a value that changes, and a
// committed file without the pin is worse than none: the did:x509 resolver
// builds its trust store out of the credential's own chain
// (nuts-node vdr/didx509/resolver.go), so a definition that does not constrain
// $.issuer accepts a chain the requester minted itself. config/policy is
// mounted into a knooppunt that is not profile-gated, so leaving one there
// would arm that on every plain `docker compose up`.
//
// sandbox/generate-demo-certs.sh renders sandbox/.certs/policy/bgz.json from
// this file with the real fingerprint substituted in, and the sandbox overlay
// points NUTS_POLICY_DIRECTORY at a mount of that directory. The .template
// suffix is what keeps this file inert: loadFromDirectory loads only names
// ending in .json (nuts-node policy/local.go), so even dropped into a policy
// directory it is skipped.
const bgzPolicyTemplate = "policy/bgz.json.template"

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
	raw, err := os.ReadFile(bgzPolicyTemplate)
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
	raw, err := os.ReadFile(bgzPolicyTemplate)
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
	raw, err := os.ReadFile(bgzPolicyTemplate)
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
	fingerprint := mintCAFingerprint(t)
	definition := bgzDefinition(t, fingerprint)

	fields := resolveClaims(t, definition,
		x509Credential(t, didX509(fingerprint), "did:web:example.com:org", "Test Care Org", plataanOtherName),
		deziCredential(t, "00000010", "12345", "nurse"),
		contextCredential(t, "hospital"),
	)

	require.Equal(t, map[string]interface{}{
		"organization_id":            "did:web:example.com:org",
		"organization_name":          "Test Care Org",
		"organization_ura":           "00000010",
		"organization_ura_dezi":      "00000010",
		"user_id":                    "12345",
		"user_role":                  "nurse",
		"organization_facility_type": "hospital",
	}, fields, "organization_ura must be extracted from the SAN's URA capture group")
}

// The Dezi attestation carries its own organization URA: the node's conversion
// puts the required abonnee_nummer claim in credentialSubject.identifier
// (nuts-node vcr/credential/dezi.go). Dropping it leaves the token asserting
// one organization, taken entirely from the X509 credential, while the
// practitioner was authenticated for another. The node's own Dezi end-to-end
// policy keeps this as organization_ura_dezi, and that name is what this
// follows (nuts-node e2e-tests/oauth-flow/dezi_idtoken/accesspolicy.json).
//
// The claim lands in the introspection response's unmapped properties
// (component/pdp/shared.go keeps everything it does not name in OtherProps and
// marshals it back), so a PDP can read it without a change here.
func TestBGZPolicyCarriesTheDeziOrganization(t *testing.T) {
	fingerprint := mintCAFingerprint(t)
	definition := bgzDefinition(t, fingerprint)

	fields := resolveClaims(t, definition,
		x509Credential(t, didX509(fingerprint), "did:web:example.com:org", "Test Care Org", plataanOtherName),
		deziCredential(t, "87654321", "12345", "nurse"),
		contextCredential(t, "hospital"),
	)

	require.Equal(t, "87654321", fields["organization_ura_dezi"],
		"the organization the practitioner was authenticated for must survive into the token")
}

// The limitation, asserted rather than described, because describing it in a
// comment alone invites the next reader to assume the descriptor above rejects
// what it merely reports.
//
// A presentation definition cannot compare two fields against each other. Its
// filters are JSON Schema fragments evaluated against one JSON path in one
// credential (nuts-node vcr/pe/presentation_definition.go, matchField), so
// there is no expressible constraint of the form "$.credentialSubject.identifier
// on the Dezi credential must equal the URA in the X509 credential's SAN".
//
// So this is what carrying organization_ura_dezi buys: a token in which the
// mismatch is visible. It does not buy rejection. A practitioner attestation
// for organization B combined with the Plataan wallet credential still yields a
// token, and it is the policy decision point that has to compare the two and
// refuse. That comparison does not exist yet; it belongs to the PDP epic. Until
// it does, any consumer treating organization_ura as "the organization this
// practitioner acts for" is trusting something nothing has checked.
//
// If this test ever starts failing because Match returned an error, the
// comparison has been implemented somewhere and this comment is what needs
// rewriting.
func TestBGZPolicyCannotRejectAnOrganizationMismatch(t *testing.T) {
	fingerprint := mintCAFingerprint(t)
	definition := bgzDefinition(t, fingerprint)

	fields := resolveClaims(t, definition,
		x509Credential(t, didX509(fingerprint), "did:web:example.com:org", "Test Care Org", plataanOtherName),
		deziCredential(t, "87654321", "12345", "nurse"),
		contextCredential(t, "hospital"),
	)

	require.NotEqual(t, fields["organization_ura"], fields["organization_ura_dezi"],
		"the fixture is only meaningful while the two organizations differ")
	require.Equal(t, "00000010", fields["organization_ura"],
		"the X509 credential's URA is emitted unchanged, mismatch or not")
	require.Equal(t, "87654321", fields["organization_ura_dezi"],
		"and so is the Dezi credential's, which is what makes the mismatch visible downstream")
}

// The property that would have caught the defect this file was moved for.
//
// A did:x509 proof establishes only that the chain travelling inside the
// credential is self-consistent: the resolver takes chain[0] as the certificate
// to validate and builds its trust store out of chain[1:], both from the
// credential's own x5c header (nuts-node vdr/didx509/resolver.go). Nothing
// outside the credential is consulted, and the resolver says so where it
// declines the check: "this check is already performed by Verifiable Credential
// interaction using PEX: Presentation Definitions should limit the accepted CAs
// through ca-fingerprint constraining".
//
// So without the $.issuer constraint a requester generates its own CA, issues
// itself a leaf with any organization name and any URA, and satisfies every
// other field here. The second subtest is that requester. The third is what
// makes the second mean something: the same credential, unchanged, against a
// definition rendered for the CA that did issue it. The pin is the only
// difference between them.
func TestBGZPolicyPinsTheCAItWasRenderedFor(t *testing.T) {
	demoCA := mintCAFingerprint(t)
	attackerCA := mintCAFingerprint(t)
	require.NotEqual(t, demoCA, attackerCA, "two throwaway CAs must not collide")

	const attackerURA = "2.16.528.1.1007.99.2110-1-0-S-99999999-00.000-0"
	attackersCredentials := []vc.VerifiableCredential{
		x509Credential(t, didX509(attackerCA), "did:web:attacker.example:org", "Ziekenhuis De Plataan", attackerURA),
		deziCredential(t, "99999999", "12345", "nurse"),
		contextCredential(t, "hospital"),
	}

	t.Run("a credential issued under the pinned CA is accepted", func(t *testing.T) {
		fields := resolveClaims(t, bgzDefinition(t, demoCA),
			x509Credential(t, didX509(demoCA), "did:web:example.com:org", "Ziekenhuis De Plataan", plataanOtherName),
			deziCredential(t, "00000010", "12345", "nurse"),
			contextCredential(t, "hospital"),
		)
		require.Equal(t, "00000010", fields["organization_ura"])
	})

	t.Run("a credential issued under a CA the requester minted is rejected", func(t *testing.T) {
		_, _, err := bgzDefinition(t, demoCA).Match(attackersCredentials)
		require.ErrorIs(t, err, pe.ErrNoCredentials,
			"a self-minted chain satisfies every other field; the issuer pin is what has to stop it")
	})

	t.Run("and is rejected for the CA and nothing else", func(t *testing.T) {
		_, _, err := bgzDefinition(t, attackerCA).Match(attackersCredentials)
		require.NoError(t, err,
			"the same credentials against their own CA must pass, or the subtest above proves nothing")
	})
}

// Fail-closed by construction, asserted rather than argued. The committed file
// carries the placeholder, and a real fingerprint is the 43 base64url
// characters of an unpadded 32-byte digest, so no placeholder can ever equal
// one. This is what makes it safe for the definition to exist in the repository
// at all: dropped into a policy directory by accident, with the .json.template
// suffix removed, it denies everything rather than accepting anything.
func TestBGZPolicyTemplateAsCommittedAcceptsNothing(t *testing.T) {
	demoCA := mintCAFingerprint(t)

	_, _, err := bgzDefinition(t, caFingerprintPlaceholder).Match([]vc.VerifiableCredential{
		x509Credential(t, didX509(demoCA), "did:web:example.com:org", "Ziekenhuis De Plataan", plataanOtherName),
		deziCredential(t, "00000010", "12345", "nurse"),
		contextCredential(t, "hospital"),
	})

	require.ErrorIs(t, err, pe.ErrNoCredentials,
		"the unrendered template must be unsatisfiable, not permissive")
}

// The capture-group rule cuts both ways in this file, and the two conclusions
// look like a contradiction to anyone who reads only one of them.
//
// The rule: the node resolves a field to its single capture group when the
// pattern has one, to the whole match when it has none, and errors out on more
// than one (nuts-node vcr/pe/presentation_definition.go, matchField's pattern
// branch). Only a field with an id has its resolved value kept, as the claim of
// that name (matchConstraint, the `if field.Id != nil` branch).
//
// The issuer field therefore has no id and no capture group. No id, because it
// pins a CA rather than describing the requester, and the node's own exemplar
// leaves it unnamed too. No group, because a group would be the thing the field
// resolves to, and the day someone gives this field an id, "the tail after the
// fingerprint" is not what they will mean by it. organization_id is the
// opposite call for the same reason: it has an id, so it needs its pattern to
// resolve to the whole DID, and it wraps the DID in a group to say so.
//
// TestBGZPolicyMatchesRealCredentials asserts the resolved claims exactly, so
// an id appearing on this field fails there. This test is the other half.
func TestBGZPolicyIssuerFieldIsUnnamedAndGroupFree(t *testing.T) {
	definition := bgzDefinition(t, caFingerprintPlaceholder)

	var descriptor *pe.InputDescriptor
	for _, d := range definition.InputDescriptors {
		if d.Id == "id_uzicert_uracredential" {
			descriptor = d
		}
	}
	require.NotNil(t, descriptor)

	var issuer *pe.Field
	for i, field := range descriptor.Constraints.Fields {
		if len(field.Path) == 1 && field.Path[0] == "$.issuer" {
			issuer = &descriptor.Constraints.Fields[i]
		}
	}
	require.NotNil(t, issuer, "the X509 descriptor must constrain $.issuer, or the CA is not pinned at all")

	require.Nil(t, issuer.Id, "a field with an id emits an introspection claim; this one pins a CA")
	require.NotNil(t, issuer.Filter)
	require.NotNil(t, issuer.Filter.Pattern)
	require.Equal(t,
		"^did:x509:0:sha256:"+caFingerprintPlaceholder+"::.*$", *issuer.Filter.Pattern,
		"the committed pattern must pin the placeholder and end in a wildcard, since the policy string the toolkit emits for the leaf is not this file's business")
	require.NotContains(t, *issuer.Filter.Pattern, "(",
		"no capture group: see this test's comment for why organization_id needs the opposite")
}

// caFingerprintPlaceholder is the token generate-demo-certs.sh substitutes for
// the real fingerprint when it renders the live definition. Named once here so
// a test cannot drift from the script by editing only one of them.
const caFingerprintPlaceholder = "__GF_SANDBOX_DEMO_CA_FINGERPRINT__"

// bgzDefinition renders the template the way the generator does and returns the
// organization definition out of it. Passing caFingerprintPlaceholder yields
// the file exactly as committed.
func bgzDefinition(t *testing.T, fingerprint string) pe.PresentationDefinition {
	t.Helper()
	raw, err := os.ReadFile(bgzPolicyTemplate)
	require.NoError(t, err)

	var mapping map[string]pe.WalletOwnerMapping
	require.NoError(t, json.Unmarshal(
		[]byte(strings.ReplaceAll(string(raw), caFingerprintPlaceholder, fingerprint)), &mapping))
	return mapping["bgz"]["organization"]
}

// mintCAFingerprint generates a throwaway CA and returns the fingerprint a
// did:x509 issued under it would carry: the unpadded base64url of the SHA-256
// of the certificate's DER, which is the recipe findCertificateByHash decodes
// and compares against (nuts-node vdr/didx509/x509_utils.go).
func mintCAFingerprint(t *testing.T) string {
	t.Helper()
	key := newECKey(t)
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return caFingerprintOf(der)
}

func caFingerprintOf(der []byte) string {
	digest := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// didX509 builds the issuer a credential minted under this CA carries. The
// policy suffix is whatever the didx509 toolkit emits for the leaf; the
// descriptor's pattern ends in a wildcard precisely so it does not have to
// know, and the fixture keeps a realistic one so the wildcard is exercised
// rather than assumed.
func didX509(fingerprint string) string {
	return "did:x509:0:sha256:" + fingerprint + "::subject:O:Ziekenhuis%20De%20Plataan"
}

// x509Credential builds a jwt_vc signed with PS256, which is what the sandbox
// presents: the matcher parses the raw JWS and compares the alg header against
// the definition's jwt_vc.alg.
func x509Credential(t *testing.T, issuer, subjectDID, organizationName, otherName string) vc.VerifiableCredential {
	t.Helper()
	token, err := jwt.NewBuilder().
		Issuer(issuer).
		Subject(subjectDID).
		Claim("vc", map[string]interface{}{
			"@context": []string{"https://www.w3.org/2018/credentials/v1"},
			"type":     []string{vc.VerifiableCredentialType, "X509Credential"},
			"credentialSubject": map[string]interface{}{
				"id":      subjectDID,
				"subject": map[string]interface{}{"O": organizationName},
				"san":     map[string]interface{}{"otherName": otherName},
			},
		}).Build()
	require.NoError(t, err)
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.PS256, credentialSigningKey()))
	require.NoError(t, err)
	parsed, err := vc.ParseVerifiableCredential(string(signed))
	require.NoError(t, err)
	return *parsed
}

// credentialSigningKey is generated once. Nothing here verifies a signature,
// only that the JWS parses and names PS256, and a 2048-bit RSA key per fixture
// is most of what these tests would otherwise spend their time on.
var credentialSigningKey = sync.OnceValue(func() *rsa.PrivateKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return key
})

// deziCredential builds the ldp_vc the node derives from a v0.7 attestation:
// credentialSubject.identifier carries the organization's URA (abonnee_nummer)
// and employee.identifier the practitioner's, with a DeziIDJWT07 proof.
func deziCredential(t *testing.T, organizationURA, userID, role string) vc.VerifiableCredential {
	t.Helper()
	cred, err := vc.ParseVerifiableCredential(fmt.Sprintf(`{
		"@context": ["https://www.w3.org/2018/credentials/v1"],
		"type": ["VerifiableCredential", "DeziUserCredential"],
		"issuer": "did:web:example.com:dezi",
		"issuanceDate": "2024-01-01T00:00:00Z",
		"credentialSubject": {
			"id": "did:web:example.com:practitioner",
			"identifier": %q,
			"employee": {"identifier": %q, "role": %q}
		},
		"proof": {"type": "DeziIDJWT07", "proofPurpose": "assertionMethod", "verificationMethod": "did:web:example.com:dezi#key-1", "created": "2024-01-01T00:00:00Z"}
	}`, organizationURA, userID, role))
	require.NoError(t, err)
	return *cred
}

// contextCredential builds the caller's self-asserted organization context: an
// ldp_vc with no proof at all, admitted by matchFormat's self-attestation early
// return, so its (nonexistent) proof type is never consulted.
func contextCredential(t *testing.T, facilityType string) vc.VerifiableCredential {
	t.Helper()
	cred, err := vc.ParseVerifiableCredential(fmt.Sprintf(`{
		"@context": ["https://www.w3.org/2018/credentials/v1"],
		"type": ["VerifiableCredential", "SandboxOrganizationContextCredential"],
		"issuer": "did:web:example.com:org",
		"issuanceDate": "2024-01-01T00:00:00Z",
		"credentialSubject": {"id": "did:web:example.com:org", "facilityType": %q}
	}`, facilityType))
	require.NoError(t, err)
	return *cred
}

// resolveClaims runs the real matcher and returns the claims the node would put
// in the introspection response.
func resolveClaims(t *testing.T, definition pe.PresentationDefinition, credentials ...vc.VerifiableCredential) map[string]interface{} {
	t.Helper()
	matched, descriptors, err := definition.Match(credentials)
	require.NoError(t, err, "every credential presented must be selected by the definition")
	require.Len(t, matched, len(definition.InputDescriptors))

	credentialMap := make(map[string]vc.VerifiableCredential, len(descriptors))
	for i, cred := range matched {
		credentialMap[descriptors[i].Id] = cred
	}
	fields, err := definition.ResolveConstraintsFields(credentialMap)
	require.NoError(t, err)
	return fields
}
