package harness

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/pem"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/nutsnode"
	"github.com/nuts-foundation/nuts-knooppunt/component/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
	"github.com/nuts-foundation/nuts-knooppunt/mock-components/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/care2cure"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/stretchr/testify/require"
)

type Details struct {
	Vectors                  vectors.Details
	KnooppuntInternalBaseURL *url.URL
	HAPIBaseURL              *url.URL
	MCSDQueryFHIRBaseURL     *url.URL
	LRZaFHIRBaseURL          *url.URL
	Care2CureFHIRBaseURL     *url.URL
	SunflowerFHIRBaseURL     *url.URL
	SunflowerURA             string
	Care2CureURA             string
	MockMitzXACML            *mitzmock.ClosedQuestionService
}

type MITZDetails struct {
	KnooppuntInternalBaseURL *url.URL
	MockMITZ                 *mitzmock.SubscriptionService
}

type PEPTestConfig struct {
	CertsDir string // Path to directory containing CA cert
}

type PEPDetails struct {
	KnooppuntURL  *url.URL                 // Internal interface URL (for PDP, etc.)
	NutsPublicURL *url.URL                 // Public interface URL for Nuts APIs (for OAuth authServer)
	HAPIBaseURL   *url.URL                 // HAPI FHIR base URL
	NutsAPI       func(path string) string // Helper to build internal Nuts API URLs
	MockMitz      *mitzmock.ClosedQuestionService
}

// Start starts the full test harness with all components (MCSD, NVI, MITZ).
// Start runs the components an exchange is discovered and addressed through: the
// directories, the NVI and the policy decision point, with no Nuts node. Tests
// that need a token ask for StartWithNuts instead.
func Start(t *testing.T) Details {
	t.Helper()
	return start(t, false)
}

// StartWithNuts is Start plus the embedded Nuts node, which is what makes an
// authorization server exist to ask a token of. It costs the node's startup on
// every call, so it is a separate entry point rather than the default.
func StartWithNuts(t *testing.T) Details {
	t.Helper()
	setupNutsEnvironment(t, filepath.Join(PEPCertsDir(t), "ca.pem"))
	return start(t, true)
}

// PEPCertsDir is where the committed test certificates live, resolved from this
// source file rather than the working directory so any package can ask for it.
func PEPCertsDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "test", "e2e", "pep", "certs")
}

func start(t *testing.T, withNuts bool) Details {
	t.Helper()

	// Delay container shutdown to improve container reusability
	os.Setenv("TESTCONTAINERS_RYUK_RECONNECTION_TIMEOUT", "5m")
	os.Setenv("TESTCONTAINERS_RYUK_CONNECTION_TIMEOUT", "5m")

	dockerNetwork, err := createDockerNetwork(t)
	require.NoError(t, err)
	hapiBaseURL := startHAPI(t, dockerNetwork.Name)

	// HAPI containers are reused across tests (see startHAPI). vectors.Load is
	// intentionally non-destructive (idempotent boot/reset), so expunge here to
	// give each harness a clean store and keep tests isolated.
	require.NoError(t, vectors.ExpungeAll(hapiBaseURL), "failed to expunge HAPI FHIR server")

	testData, err := vectors.Load(hapiBaseURL)
	require.NoError(t, err, "failed to load test data into HAPI FHIR server")

	config := cmd.DefaultConfig()
	config.HTTP = http.TestConfig()
	config.MCSD.AdministrationDirectories = map[string]mcsd.DirectoryConfig{
		"lrza": {
			FHIRBaseURL: testData.LRZa.FHIRBaseURL.String(),
		},
	}
	config.MCSD.QueryDirectory = mcsd.DirectoryConfig{
		FHIRBaseURL: testData.Knooppunt.MCSD.QueryFHIRBaseURL.String(),
	}
	config.NVI = nvi.Config{
		FHIRBaseURL: testData.NVI.FHIRBaseURL.String(),
		Audience:    "nvi",
	}
	config.PDP = pdp.Config{
		Enabled: true,
		PIP: pdp.PIPConfig{
			URL: testData.PIP.FHIRBaseURL.String(),
		},
	}

	config.Nuts = nutsnode.Config{Enabled: withNuts}

	mockMitz := mitzmock.NewClosedQuestionService(t)
	config.MITZ = mitz.Config{
		MitzBase:      mockMitz.GetURL(),
		GatewaySystem: "test-gateway",
		SourceSystem:  "test-source",
	}

	knooppuntInternalURL := startKnooppunt(t, config)

	return Details{
		KnooppuntInternalBaseURL: knooppuntInternalURL,
		HAPIBaseURL:              hapiBaseURL,
		MCSDQueryFHIRBaseURL:     testData.Knooppunt.MCSD.QueryFHIRBaseURL,
		LRZaFHIRBaseURL:          testData.LRZa.FHIRBaseURL,
		SunflowerFHIRBaseURL:     sunflower.AdminHAPITenant().BaseURL(hapiBaseURL),
		SunflowerURA:             *sunflower.Organization().Identifier[0].Value,
		Care2CureFHIRBaseURL:     care2cure.AdminHAPITenant().BaseURL(hapiBaseURL),
		Care2CureURA:             *care2cure.Organization().Identifier[0].Value,
		Vectors:                  *testData,
		MockMitzXACML:            mockMitz,
	}
}

// StartMITZ starts a minimal harness with just Knooppunt and mock MITZ server, for MITZ-specific e2e tests.
func StartMITZ(t *testing.T) MITZDetails {
	t.Helper()

	// Create mock MITZ server
	mockMITZ := mitzmock.NewSubscriptionService(t)

	// Start Knooppunt with minimal config (only MITZ enabled)
	knooppuntInternalURL := startKnooppunt(t, cmd.Config{
		HTTP: http.TestConfig(),
		MITZ: mitz.Config{
			MitzBase:       mockMITZ.GetURL(),
			NotifyEndpoint: "http://localhost:8080/consent/notify",
			GatewaySystem:  "test-gateway",
			SourceSystem:   "test-source",
		},
	})

	return MITZDetails{
		KnooppuntInternalBaseURL: knooppuntInternalURL,
		MockMITZ:                 mockMITZ,
	}
}

// StartPEP starts a harness for PEP e2e tests with embedded Nuts node, PDP, and mock MITZ.
func StartPEP(t *testing.T, config PEPTestConfig) PEPDetails {
	t.Helper()

	// Set up Nuts node environment variables
	setupNutsEnvironment(t, filepath.Join(config.CertsDir, "ca.pem"))

	// Create mock XACML Mitz server
	mockMitz := mitzmock.NewClosedQuestionService(t)

	// Start HAPI FHIR server (shared helper from hapi.go)
	dockerNetwork, err := createDockerNetwork(t)
	require.NoError(t, err)
	hapiBaseURL := startHAPI(t, dockerNetwork.Name)

	// Start Knooppunt with embedded Nuts node and PDP
	knooppuntURL := startKnooppunt(t, cmd.Config{
		HTTP: http.TestConfig(),
		Nuts: nutsnode.Config{Enabled: true},
		PDP: pdp.Config{
			Enabled: true,
			PIP: pdp.PIPConfig{
				URL: hapiBaseURL.String() + "/DEFAULT",
			},
		},
		MITZ: mitz.Config{
			MitzBase:      mockMitz.GetURL(),
			GatewaySystem: "test-gateway",
			SourceSystem:  "test-source",
		},
	})

	// The public Nuts URL is on port 8080 (from http.TestConfig().PublicInterface)
	nutsPublicURL, _ := url.Parse("http://localhost:8080/nuts")

	return PEPDetails{
		KnooppuntURL:  knooppuntURL,
		NutsPublicURL: nutsPublicURL,
		HAPIBaseURL:   hapiBaseURL,
		NutsAPI: func(path string) string {
			return knooppuntURL.JoinPath("/nuts", path).String()
		},
		MockMitz: mockMitz,
	}
}

// repoRoot returns the absolute path of the repository root, derived from this
// source file's location rather than the working directory, so helpers can read
// repository config regardless of which package's directory the test runs in.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "unable to determine harness source location")
	// this file lives at <root>/test/e2e/harness/entrypoint.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

// setupNutsEnvironment configures environment variables for the embedded Nuts node.
//
// The access policy and discovery definition are read from the repository's
// config/ directory — the same files docker compose serves — rather than from a
// testdata copy. Keeping a single source matters because the discovery definition
// pins the test CA's public key hash: two copies would drift, and the resulting
// authorization failures are opaque.
func setupNutsEnvironment(t *testing.T, caPath string) {
	t.Helper()

	repoConfigDir := filepath.Join(repoRoot(t), "config")

	// Create temp directories for Nuts node configuration
	tempDir := t.TempDir()
	policyDir := filepath.Join(tempDir, "policies")
	discoveryDir := filepath.Join(tempDir, "discovery")
	require.NoError(t, os.MkdirAll(policyDir, 0755))
	require.NoError(t, os.MkdirAll(discoveryDir, 0755))

	// Copy policy file
	policyData, err := os.ReadFile(filepath.Join(repoConfigDir, "policy", "accesspolicy.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "accesspolicy.json"), policyData, 0644))

	// The bgz scope is not in config/policy: its presentation definition pins a
	// certificate authority fingerprint, so it is rendered per deployment rather
	// than committed. renderBGZPolicy does for this harness what
	// sandbox/generate-demo-certs.sh does for compose, from the same template,
	// with the test CA these certificates descend from. Without it the node
	// serves no bgz definition and a token request fails with invalid_scope two
	// services away from the cause.
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "bgz.json"),
		renderBGZPolicy(t, caPath), 0644))

	// Copy discovery definition (must be named <service-id>.json)
	discoveryData, err := os.ReadFile(filepath.Join(repoConfigDir, "discovery", "bgz-test.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(discoveryDir, "bgz-test.json"), discoveryData, 0644))

	// Set Nuts node environment variables
	os.Setenv("NUTS_URL", "http://localhost:8080/nuts")
	os.Setenv("NUTS_AUTH_CONTRACTVALIDATORS", "dummy")
	os.Setenv("NUTS_POLICY_DIRECTORY", policyDir)
	os.Setenv("NUTS_DISCOVERY_DEFINITIONS_DIRECTORY", discoveryDir)
	os.Setenv("NUTS_DISCOVERY_SERVER_IDS", "bgz-test")
	os.Setenv("NUTS_VDR_DIDMETHODS", "web")
	os.Setenv("NUTS_INTERNALRATELIMITER", "false")
	os.Setenv("NUTS_NETWORK_ENABLEDISCOVERY", "false")
	os.Setenv("SSL_CERT_FILE", caPath)
}

// bgzFingerprintPlaceholder is what sandbox/policy/bgz.json.template carries
// where the trusted authority's fingerprint belongs. The template as committed
// denies everything, which is the point: a placeholder is not 43 base64url
// characters, so an unrendered definition cannot accidentally trust anyone.
const bgzFingerprintPlaceholder = "__GF_SANDBOX_DEMO_CA_FINGERPRINT__"

// renderBGZPolicy pins caPath's fingerprint into the bgz presentation
// definition. The fingerprint is the unpadded base64url SHA-256 of the
// certificate's DER form, which is how did:x509 names an authority.
func renderBGZPolicy(t *testing.T, caPath string) []byte {
	t.Helper()
	template, err := os.ReadFile(filepath.Join(repoRoot(t), "sandbox", "policy", "bgz.json.template"))
	require.NoError(t, err, "the bgz presentation definition template is missing")

	pemBytes, err := os.ReadFile(caPath)
	require.NoError(t, err)
	block, _ := pem.Decode(pemBytes)
	require.NotNil(t, block, "%s holds no PEM block", caPath)

	sum := sha256.Sum256(block.Bytes)
	fingerprint := base64.RawURLEncoding.EncodeToString(sum[:])
	require.Len(t, fingerprint, 43, "an unpadded base64url SHA-256 is 43 characters")

	return bytes.ReplaceAll(template, []byte(bgzFingerprintPlaceholder), []byte(fingerprint))
}
