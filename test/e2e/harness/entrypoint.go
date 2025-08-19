package harness

import (
	"net/url"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata"
	"github.com/stretchr/testify/require"
)

type Details struct {
	KnooppuntInternalBaseURL *url.URL
	MCSDCacheFHIRBaseURL     *url.URL
	SunflowerURA             string
	Care2CureURA             string
}

func Start(t *testing.T) Details {
	t.Helper()

	dockerNetwork, err := createDockerNetwork(t)
	require.NoError(t, err)
	hapiBaseURL := startHAPI(t, dockerNetwork.Name)

	testData, err := testdata.Load(hapiBaseURL)
	require.NoError(t, err, "failed to load test data into HAPI FHIR server")

	knooppuntInternalURL := startKnooppunt(t, cmd.Config{
		MCSD: mcsd.Config{
			RootDirectories: map[string]mcsd.DirectoryConfig{
				"lrza": {
					FHIRBaseURL: testData.LRZa.FHIRBaseURL.String(),
				},
			},
			LocalDirectory: mcsd.DirectoryConfig{
				FHIRBaseURL: testData.Knooppunt.MCSD.CacheFHIRBaseURL.String(),
			},
		},
	})
	return Details{
		KnooppuntInternalBaseURL: knooppuntInternalURL,
		MCSDCacheFHIRBaseURL:     testData.Knooppunt.MCSD.CacheFHIRBaseURL,
		SunflowerURA:             "00000020",
		Care2CureURA:             "00000030",
	}
}
