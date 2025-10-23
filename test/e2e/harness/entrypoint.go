package harness

import (
	"net/url"
	"os"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/nvi"
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
}

type MITZDetails struct {
	KnooppuntInternalBaseURL *url.URL
	MockMITZ                 *MockMITZServer
}

// Start starts the full test harness with all components (MCSD, NVI, MITZ).
func Start(t *testing.T) Details {
	t.Helper()

	// Delay container shutdown to improve container reusability
	os.Setenv("TESTCONTAINERS_RYUK_RECONNECTION_TIMEOUT", "5m")
	os.Setenv("TESTCONTAINERS_RYUK_CONNECTION_TIMEOUT", "5m")

	// Create mock MITZ server
	mockMITZ := NewMockMITZServer(t)

	dockerNetwork, err := createDockerNetwork(t)
	require.NoError(t, err)
	hapiBaseURL := startHAPI(t, dockerNetwork.Name)

	testData, err := vectors.Load(hapiBaseURL)
	require.NoError(t, err, "failed to load test data into HAPI FHIR server")

	knooppuntInternalURL := startKnooppunt(t, cmd.Config{
		MCSD: mcsd.Config{
			AdministrationDirectories: map[string]mcsd.DirectoryConfig{
				"lrza": {
					FHIRBaseURL: testData.LRZa.FHIRBaseURL.String(),
				},
			},
			QueryDirectory: mcsd.DirectoryConfig{
				FHIRBaseURL: testData.Knooppunt.MCSD.QueryFHIRBaseURL.String(),
			},
		},
		NVI: nvi.DefaultConfig(),
		MITZ: mitz.Config{
			MitzBase:       mockMITZ.GetURL(),
			NotifyEndpoint: "http://localhost:8080/consent/notify",
			GatewaySystem:  "test-gateway",
			SourceSystem:   "test-source",
		},
	})

	return Details{
		KnooppuntInternalBaseURL: knooppuntInternalURL,
		MCSDQueryFHIRBaseURL:     testData.Knooppunt.MCSD.QueryFHIRBaseURL,
		LRZaFHIRBaseURL:          testData.LRZa.FHIRBaseURL,
		SunflowerFHIRBaseURL:     sunflower.HAPITenant().BaseURL(hapiBaseURL),
		SunflowerURA:             *sunflower.Organization().Identifier[0].Value,
		Care2CureFHIRBaseURL:     care2cure.HAPITenant().BaseURL(hapiBaseURL),
		Care2CureURA:             *care2cure.Organization().Identifier[0].Value,
		Vectors:                  *testData,
	}
}

// StartMITZ starts a minimal harness with just Knooppunt and mock MITZ server, for MITZ-specific e2e tests.
func StartMITZ(t *testing.T) MITZDetails {
	t.Helper()

	// Create mock MITZ server
	mockMITZ := NewMockMITZServer(t)

	// Start Knooppunt with minimal config (only MITZ enabled)
	knooppuntInternalURL := startKnooppunt(t, cmd.Config{
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
