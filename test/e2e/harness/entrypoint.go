package harness

import (
	"context"
	"net/url"
	"os"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/component/mcsd"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/care2cure"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/stretchr/testify/require"
)

type Details struct {
	MCSDQueryFHIRBaseURL *url.URL
	LRZaFHIRBaseURL      *url.URL
	Care2CureFHIRBaseURL *url.URL
	SunflowerFHIRBaseURL *url.URL
	SunflowerURA         string
	Care2CureURA         string

	KnooppuntInternalBaseURL *url.URL
	knooppuntShutdownFunc    context.CancelFunc
	knooppuntShutdownChan    chan struct{}
	testData                 *vectors.Details
}

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

	details := Details{
		testData: testData,
	}
	details.start(t)

	return Details{
		KnooppuntInternalBaseURL: knooppuntInternalURL,
		knooppuntShutdownFunc:    knooppuntShutdownFunc,
		knooppuntShutdownChan:    knooppuntShutdownChan,
		testData:                 testData,
		MCSDQueryFHIRBaseURL:     testData.Knooppunt.MCSD.QueryFHIRBaseURL,
		LRZaFHIRBaseURL:          testData.LRZa.FHIRBaseURL,
		SunflowerFHIRBaseURL:     sunflower.HAPITenant().BaseURL(hapiBaseURL),
		SunflowerURA:             *sunflower.Organization().Identifier[0].Value,
		Care2CureFHIRBaseURL:     care2cure.HAPITenant().BaseURL(hapiBaseURL),
		Care2CureURA:             *care2cure.Organization().Identifier[0].Value,
	}
}

func (d *Details) Restart(t *testing.T) Details {
	// Stop Knooppunt and wait for it to exit
	d.knooppuntShutdownFunc()
	_ = <-d.knooppuntShutdownChan
	newDetails := *d
	newDetails.start(t)
	return newDetails
}

func (d *Details) start(t *testing.T) {
	ctx, knooppuntShutdownFunc := context.WithCancel(t.Context())
	knooppuntInternalURL, knooppuntShutdownChan := startKnooppunt(t, ctx, cmd.Config{
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
	d.knooppuntShutdownFunc = knooppuntShutdownFunc
	d.knooppuntShutdownChan = knooppuntShutdownChan
	d.KnooppuntInternalBaseURL = knooppuntInternalURL
}
