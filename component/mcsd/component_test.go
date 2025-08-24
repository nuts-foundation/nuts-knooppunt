package mcsd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_update(t *testing.T) {
	t.Log("mCSD Component is tested limited here, as it requires running FHIR servers and a lot of data. The main logic is tested in the integration tests.")
	testDataJSON, err := os.ReadFile("test/test_bundle_with_directories.json")
	require.NoError(t, err)

	rootDirMux := http.NewServeMux()
	rootDirMux.HandleFunc("/_history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testDataJSON)
	})
	rootDirServer := httptest.NewServer(rootDirMux)

	localClient := &test.StubFHIRClient{}
	component := Component{
		config: Config{
			RootDirectories: map[string]DirectoryConfig{
				"rootDir": {
					FHIRBaseURL: rootDirServer.URL,
				},
			},
			LocalDirectory: DirectoryConfig{
				FHIRBaseURL: "http://example.com/local/fhir",
			},
		},
		fhirClientFn: func(baseURL *url.URL) fhirclient.Client {
			if baseURL.String() == rootDirServer.URL {
				return fhirclient.New(baseURL, http.DefaultClient, nil)
			}
			if baseURL.String() == "http://example.com/local/fhir" {
				return localClient
			}
			panic("unknown base URL: " + baseURL.String())
		},
	}
	ctx := context.Background()

	report, err := component.update(ctx)

	require.NoError(t, err)
	require.NotNil(t, report)
	require.Nil(t, report[rootDirServer.URL].Error)
	require.Empty(t, report[rootDirServer.URL].Warnings)

	t.Run("check created resources", func(t *testing.T) {
		require.Len(t, localClient.CreatedResources["Bundle"], 1)
		bundle := localClient.CreatedResources["Bundle"][0].(fhir.Bundle)
		require.Len(t, bundle.Entry, 9)
	})
}

func parseJSON[T any](data []byte) (*T, error) {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
