package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/nuts-foundation/go-did/vc"
	"github.com/nuts-foundation/nuts-node/vcr/pe"
	"github.com/stretchr/testify/require"
)

func TestOrganizationContextCredentialShape(t *testing.T) {
	// Z9 must differ from nutsConfigFromEnv's default (Z3): otherwise a
	// hardcoded facilityType in the credential would pass this test too,
	// and nothing would prove the configured value is actually forwarded.
	cred := organizationContextCredential(nutsConfig{FacilityType: "Z9"})

	require.Equal(t, []string{"https://www.w3.org/2018/credentials/v1"}, cred["@context"])
	require.Equal(t, []string{"VerifiableCredential", "SandboxOrganizationContextCredential"}, cred["type"])
	require.Equal(t, map[string]any{"facilityType": "Z9"}, cred["credentialSubject"])
}

// The node rejects a request credential that carries either field and fills
// both in itself, so setting them turns a working request into a 400 whose
// message points at the credential rather than at us.
func TestOrganizationContextCredentialOmitsIssuerAndSubjectID(t *testing.T) {
	cred := organizationContextCredential(nutsConfig{FacilityType: "Z3"})

	require.NotContains(t, cred, "issuer", "the node rejects a caller-supplied issuer")
	subject, ok := cred["credentialSubject"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, subject, "id", "the node rejects a caller-supplied credentialSubject.id")
}

func TestNutsConfigDefaultsAreDevelopmentLocal(t *testing.T) {
	// Isolate from the ambient environment: without this, a developer who
	// happens to have e.g. SANDBOX_FACILITY_TYPE exported gets a spurious
	// failure here, since envOr would then read the real value instead of
	// falling back to the default this test asserts.
	for _, key := range []string{
		"NUTS_INTERNAL_BASE_URL",
		"SANDBOX_NUTS_SUBJECT",
		"SANDBOX_BGZ_SCOPE",
		"SANDBOX_AUTH_SERVER",
		"SANDBOX_FACILITY_TYPE",
	} {
		t.Setenv(key, "")
	}

	cfg := nutsConfigFromEnv()

	require.Equal(t, "http://localhost:8081", cfg.InternalBaseURL)
	require.Equal(t, "plataan", cfg.Subject)
	require.Equal(t, "bgz", cfg.Scope)
	// The audience check compares this string to the node's advertised issuer
	// byte for byte, so a Docker hostname here fails even though it resolves.
	require.Equal(t, "http://localhost:8080/nuts/oauth2/plataan", cfg.AuthorizationServer)
	require.Equal(t, "Z3", cfg.FacilityType)
}

// The defaults test above never sets an environment variable, so it only
// exercises envOr's fallback branch. These five names are the contract with
// the compose file a later task writes: a renamed key, or a field reading
// another key while keeping its own default, would leave the default value
// in place and go uncaught there. Distinct sentinel values per key catch
// both.
func TestNutsConfigFromEnvReadsEachVariable(t *testing.T) {
	t.Setenv("NUTS_INTERNAL_BASE_URL", "http://internal-base-url.example")
	t.Setenv("SANDBOX_NUTS_SUBJECT", "sentinel-subject")
	t.Setenv("SANDBOX_BGZ_SCOPE", "sentinel-scope")
	t.Setenv("SANDBOX_AUTH_SERVER", "http://auth-server.example")
	t.Setenv("SANDBOX_FACILITY_TYPE", "sentinel-facility-type")

	cfg := nutsConfigFromEnv()

	require.Equal(t, "http://internal-base-url.example", cfg.InternalBaseURL)
	require.Equal(t, "sentinel-subject", cfg.Subject)
	require.Equal(t, "sentinel-scope", cfg.Scope)
	require.Equal(t, "http://auth-server.example", cfg.AuthorizationServer)
	require.Equal(t, "sentinel-facility-type", cfg.FacilityType)
}

// config/policy/bgz_test.go checks the id_sandbox_organization_context
// descriptor against its own hand-written credential fixture, not this
// function's output. Editing bgz.json and that fixture together — the
// obvious way to change what the descriptor selects — leaves both packages
// green while the credential this code actually produces no longer matches
// it. This test runs the real matcher (vcr/pe) over the real output of
// organizationContextCredential instead of a fixture.
func TestOrganizationContextCredentialMatchesBGZPolicy(t *testing.T) {
	policyRaw, err := os.ReadFile("../../config/policy/bgz.json")
	require.NoError(t, err)

	var mapping map[string]pe.WalletOwnerMapping
	require.NoError(t, json.Unmarshal(policyRaw, &mapping))
	definition := mapping["bgz"]["organization"]

	var descriptor *pe.InputDescriptor
	for _, d := range definition.InputDescriptors {
		if d.Id == "id_sandbox_organization_context" {
			descriptor = d
		}
	}
	require.NotNil(t, descriptor, "id_sandbox_organization_context must still be defined in bgz.json")
	definition.InputDescriptors = []*pe.InputDescriptor{descriptor}

	const configuredFacilityType = "outpatient-clinic"
	cred := organizationContextCredential(nutsConfig{FacilityType: configuredFacilityType})
	credRaw, err := json.Marshal(cred)
	require.NoError(t, err)
	parsed, err := vc.ParseVerifiableCredential(string(credRaw))
	require.NoError(t, err)

	matched, descriptors, err := definition.Match([]vc.VerifiableCredential{*parsed})
	require.NoError(t, err, "the credential organizationContextCredential produces must satisfy the descriptor bgz.json selects it with")
	require.Len(t, matched, 1)

	credentialMap := map[string]vc.VerifiableCredential{descriptors[0].Id: matched[0]}
	fields, err := definition.ResolveConstraintsFields(credentialMap)
	require.NoError(t, err)
	require.Equal(t, configuredFacilityType, fields["organization_facility_type"])
}
