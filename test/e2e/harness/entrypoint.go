package harness

import (
	"net/url"
	"os"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
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
	MockMitzXACML            *MockXACMLMitzServer
}

type MITZDetails struct {
	KnooppuntInternalBaseURL *url.URL
	MockMITZ                 *MockMITZServer
}

type PEPDetails struct {
	KnooppuntPDPBaseURL *url.URL
	HAPIBaseURL         *url.URL
	PEPBaseURL          *url.URL
	MockMitzXACML       *MockXACMLMitzServer
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
		PIPURL:  testData.PIP.FHIRBaseURL.String(),
	}

	mockMitz := NewMockXACMLMitzServer(t)
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
	mockMITZ := NewMockMITZServer(t)

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

// StartPEP starts a minimal harness for PEP e2e tests with HAPI, Knooppunt PDP, mock XACML Mitz, and PEP nginx.
func StartPEP(t *testing.T, pepConfig PEPConfig) PEPDetails {
	t.Helper()

	// Create mock XACML Mitz server
	mockMitz := NewMockXACMLMitzServer(t)

	// Start HAPI FHIR server
	hapiBaseURL := startHAPI(t, "")

	// Start Knooppunt with PDP and MITZ enabled
	knooppuntPDPURL := startKnooppunt(t, cmd.Config{
		HTTP: http.TestConfig(),
		PDP: pdp.Config{
			Enabled: true,
		},
		MITZ: mitz.Config{
			MitzBase:      mockMitz.GetURL(),
			GatewaySystem: "test-gateway",
			SourceSystem:  "test-source",
		},
	})

	// Configure PEP to point to HAPI and Knooppunt
	pepConfig.FHIRBackendHost = "host.docker.internal"
	pepConfig.FHIRBackendPort = hapiBaseURL.Port()
	pepConfig.KnooppuntPDPHost = "host.docker.internal"
	pepConfig.KnooppuntPDPPort = knooppuntPDPURL.Port()

	// Start PEP container
	pepBaseURL := startPEP(t, pepConfig)

	return PEPDetails{
		KnooppuntPDPBaseURL: knooppuntPDPURL,
		HAPIBaseURL:         hapiBaseURL,
		PEPBaseURL:          pepBaseURL,
		MockMitzXACML:       mockMitz,
	}
}
