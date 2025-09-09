package harness

import (
	"net/url"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/stretchr/testify/require"
)

type Details struct {
	KnooppuntInternalBaseURL *url.URL
	MCSDQueryFHIRBaseURL     *url.URL
	LRZaFHIRBaseURL          *url.URL
	Care2CureFHIRBaseURL     *url.URL
	SunflowerFHIRBaseURL     *url.URL
	SunflowerURA             string
	Care2CureURA             string
}

func Start(t *testing.T) Details {
	t.Helper()

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
	})
	care2CureTenant := vectors.HAPITenant{Name: "care2cure-admin", ID: 4}
	sunflowerTenant := vectors.HAPITenant{Name: "sunflower-admin", ID: 5}
	return Details{
		KnooppuntInternalBaseURL: knooppuntInternalURL,
		MCSDQueryFHIRBaseURL:     testData.Knooppunt.MCSD.QueryFHIRBaseURL,
		LRZaFHIRBaseURL:          testData.LRZa.FHIRBaseURL,
		Care2CureFHIRBaseURL:     care2CureTenant.BaseURL(hapiBaseURL),
		SunflowerFHIRBaseURL:     sunflowerTenant.BaseURL(hapiBaseURL),
		SunflowerURA:             "00000020",
		Care2CureURA:             "00000030",
	}
}
