package mcsd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

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
	component := New(Config{
		RootDirectories: map[string]DirectoryConfig{
			"rootDir": {
				FHIRBaseURL: rootDirServer.URL,
			},
		},
		LocalDirectory: DirectoryConfig{
			FHIRBaseURL: "http://example.com/local/fhir",
		},
	})
	component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
		if baseURL.String() == rootDirServer.URL {
			return fhirclient.New(baseURL, http.DefaultClient, nil)
		}
		if baseURL.String() == "http://example.com/local/fhir" {
			return localClient
		}
		panic("unknown base URL: " + baseURL.String())
	}
	ctx := context.Background()

	report, err := component.update(ctx)

	require.NoError(t, err)
	require.NotNil(t, report)
	require.NoError(t, report[rootDirServer.URL].Error)
	require.Empty(t, report[rootDirServer.URL].Warnings)

	t.Run("check created resources", func(t *testing.T) {
		require.Len(t, localClient.CreatedResources["Bundle"], 1)
		bundle := localClient.CreatedResources["Bundle"][0].(fhir.Bundle)
		require.Len(t, bundle.Entry, 9)
	})
}

func TestComponent_incrementalUpdates(t *testing.T) {
	testDataJSON, err := os.ReadFile("test/test_bundle_with_directories.json")
	require.NoError(t, err)

	var sinceParams []string
	rootDirMux := http.NewServeMux()
	rootDirMux.HandleFunc("/_history", func(w http.ResponseWriter, r *http.Request) {
		// FHIR client uses POST by default, parameters are in form data
		r.ParseForm()
		since := r.Form.Get("_since")
		sinceParams = append(sinceParams, since)
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testDataJSON)
	})
	rootDirServer := httptest.NewServer(rootDirMux)

	config := Config{
		RootDirectories: map[string]DirectoryConfig{
			"rootDir": {
				FHIRBaseURL: rootDirServer.URL,
			},
		},
		LocalDirectory: DirectoryConfig{
			FHIRBaseURL: "http://example.com/local/fhir",
		},
	}
	component := New(config)
	component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
		if baseURL.String() == rootDirServer.URL {
			return fhirclient.New(baseURL, http.DefaultClient, nil)
		}
		if baseURL.String() == "http://example.com/local/fhir" {
			return &test.StubFHIRClient{}
		}
		panic("unknown base URL: " + baseURL.String())
	}
	ctx := context.Background()

	// First update - should have no _since parameter
	_, err = component.update(ctx)
	require.NoError(t, err)
	require.Len(t, sinceParams, 1, "Should have one request")
	require.Empty(t, sinceParams[0], "First update should not have _since parameter")
	
	// Verify timestamp was stored
	component.lastUpdateTimesMux.RLock()
	lastUpdate, exists := component.lastUpdateTimes[rootDirServer.URL]
	component.lastUpdateTimesMux.RUnlock()
	require.True(t, exists, "Last update time should be stored")
	require.WithinDuration(t, time.Now(), lastUpdate, 5*time.Second)

	// Second update - should include _since parameter
	_, err = component.update(ctx)
	require.NoError(t, err)
	require.Len(t, sinceParams, 2, "Should have two requests total")
	require.NotEmpty(t, sinceParams[1], "Second update should include _since parameter")
	
	// Verify _since parameter is a valid RFC3339 timestamp
	_, err = time.Parse(time.RFC3339, sinceParams[1])
	require.NoError(t, err, "_since parameter should be valid RFC3339 timestamp")
	
	// Verify _since parameter matches the stored timestamp
	expectedSince := lastUpdate.Format(time.RFC3339)
	require.Equal(t, expectedSince, sinceParams[1], "_since parameter should match the stored lastUpdate timestamp")
}
