package harness

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/nutsnode"
	"github.com/nuts-foundation/nuts-knooppunt/component/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
	"github.com/nuts-foundation/nuts-knooppunt/test/mitzmock"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/care2cure"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/stretchr/testify/require"
)

type Details struct {
	Vectors                  vectors.Details
	KnooppuntInternalBaseURL *url.URL
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
	CertsDir    string // Path to directory containing CA cert
	TestDataDir string // Path to directory containing accesspolicy.json and discovery.json
}

type PEPDetails struct {
	KnooppuntURL  *url.URL                          // Internal interface URL (for PDP, etc.)
	NutsPublicURL *url.URL                          // Public interface URL for Nuts APIs (for OAuth authServer)
	HAPIBaseURL   *url.URL                          // HAPI FHIR base URL
	NutsAPI       func(path string) string          // Helper to build internal Nuts API URLs
	MockMitz      *mitzmock.ClosedQuestionService
}


// Start starts the full test harness with all components (MCSD, NVI, MITZ).
func Start(t *testing.T) Details {
	t.Helper()

	// Delay container shutdown to improve container reusability
	os.Setenv("TESTCONTAINERS_RYUK_RECONNECTION_TIMEOUT", "5m")
	os.Setenv("TESTCONTAINERS_RYUK_CONNECTION_TIMEOUT", "5m")

	dockerNetwork, err := createDockerNetwork(t)
	require.NoError(t, err)
	hapiBaseURL := startHAPI(t, dockerNetwork.Name)

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

	mockMitz := mitzmock.NewClosedQuestionService(t)
	config.MITZ = mitz.Config{
		MitzBase:      mockMitz.GetURL(),
		GatewaySystem: "test-gateway",
		SourceSystem:  "test-source",
	}

	knooppuntInternalURL := startKnooppunt(t, config)

	return Details{
		KnooppuntInternalBaseURL: knooppuntInternalURL,
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
	setupNutsEnvironment(t, config.TestDataDir, filepath.Join(config.CertsDir, "ca.pem"))

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

// setupNutsEnvironment configures environment variables for the embedded Nuts node.
func setupNutsEnvironment(t *testing.T, testdataDir, caPath string) {
	t.Helper()

	// Create temp directories for Nuts node configuration
	tempDir := t.TempDir()
	policyDir := filepath.Join(tempDir, "policies")
	discoveryDir := filepath.Join(tempDir, "discovery")
	require.NoError(t, os.MkdirAll(policyDir, 0755))
	require.NoError(t, os.MkdirAll(discoveryDir, 0755))

	// Copy policy file
	policyData, err := os.ReadFile(filepath.Join(testdataDir, "accesspolicy.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(policyDir, "accesspolicy.json"), policyData, 0644))

	// Copy discovery definition (must be named <service-id>.json)
	discoveryData, err := os.ReadFile(filepath.Join(testdataDir, "discovery.json"))
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

