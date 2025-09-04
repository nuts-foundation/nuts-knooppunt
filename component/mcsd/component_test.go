package mcsd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/stretchr/testify/require"
)

func TestComponent_update(t *testing.T) {
	t.Log("mCSD Component is tested limited here, as it requires running FHIR servers and a lot of data. The main logic is tested in the integration tests.")
	rootDirHistoryResponseBytes, err := os.ReadFile("test/root_dir_history_response.json")
	require.NoError(t, err)
	rootDirHistoryResponse := string(rootDirHistoryResponseBytes)

	rootDirMux := http.NewServeMux()
	rootDirMux.HandleFunc("/_history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(rootDirHistoryResponse))
	})
	rootDirServer := httptest.NewServer(rootDirMux)

	org1DirHistoryResponseBytes, err := os.ReadFile("test/org1_dir_history_response.json")
	require.NoError(t, err)
	org1DirHistoryResponse := string(org1DirHistoryResponseBytes)

	org1DirMux := http.NewServeMux()
	org1DirMux.HandleFunc("/fhir/_history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(org1DirHistoryResponse))
	})
	org1DirServer := httptest.NewServer(org1DirMux)

	orgDir1BaseURL := org1DirServer.URL + "/fhir"
	rootDirHistoryResponse = strings.ReplaceAll(rootDirHistoryResponse, "{{ORG1_DIR_BASEURL}}", orgDir1BaseURL)
	org1DirHistoryResponse = strings.ReplaceAll(org1DirHistoryResponse, "{{ORG1_DIR_BASEURL}}", orgDir1BaseURL)

	localClient := &test.StubFHIRClient{}
	component := New(Config{
		AdministrationDirectories: map[string]DirectoryConfig{
			"rootDir": {
				FHIRBaseURL: rootDirServer.URL,
			},
		},
		QueryDirectory: DirectoryConfig{
			FHIRBaseURL: "http://example.com/local/fhir",
		},
	})
	unknownFHIRServerClient := &test.StubFHIRClient{
		Error: errors.New("404 Not Found"),
	}
	component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
		if baseURL.String() == rootDirServer.URL ||
			baseURL.String() == orgDir1BaseURL {
			return fhirclient.New(baseURL, http.DefaultClient, nil)
		}
		if baseURL.String() == "http://example.com/local/fhir" {
			return localClient
		}
		t.Log("Using unknown FHIR server client for baseURL: " + baseURL.String())
		return unknownFHIRServerClient
	}
	ctx := context.Background()

	report, err := component.update(ctx)

	require.NoError(t, err)
	require.NotNil(t, report)
	t.Run("assert sync report from root directory", func(t *testing.T) {
		thisReport := report[rootDirServer.URL]
		require.Empty(t, thisReport.Errors)
		// Root directory: only mCSD directory endpoints should be synced, other resources should be filtered out
		t.Run("warnings", func(t *testing.T) {
			require.Len(t, thisReport.Warnings, 2)
			require.Contains(t, thisReport.Warnings[0], "failed to register discovered mCSD Directory at file:///etc/passwd: invalid FHIR base URL (url=file:///etc/passwd)")
		})
		require.Equal(t, 4, thisReport.CountCreated) // 4 mCSD directory endpoints should be created
		require.Equal(t, 0, thisReport.CountUpdated)
		require.Equal(t, 0, thisReport.CountDeleted)
	})
	t.Run("assert sync report from org1 directory", func(t *testing.T) {
		thisReport := report[orgDir1BaseURL]
		require.Empty(t, thisReport.Errors)
		require.Empty(t, thisReport.Warnings)
		require.Equal(t, 2, thisReport.CountCreated) // Now 2 resources: Organization + Endpoint
		require.Equal(t, 0, thisReport.CountUpdated)
		require.Equal(t, 0, thisReport.CountDeleted)
	})
	t.Run("assert sync report from non-existing FHIR server #1", func(t *testing.T) {
		thisReport := report["https://directory1.example.org"]
		require.Equal(t, "_history search failed: 404 Not Found", strings.Join(thisReport.Errors, ""))
		require.Empty(t, thisReport.Warnings)
		require.Equal(t, 0, thisReport.CountCreated)
		require.Equal(t, 0, thisReport.CountUpdated)
		require.Equal(t, 0, thisReport.CountDeleted)
	})
	t.Run("assert sync report from non-existing FHIR server #2", func(t *testing.T) {
		thisReport := report["https://directory2.example.org"]
		require.Equal(t, "_history search failed: 404 Not Found", strings.Join(thisReport.Errors, ""))
		require.Empty(t, thisReport.Warnings)
		require.Equal(t, 0, thisReport.CountCreated)
		require.Equal(t, 0, thisReport.CountUpdated)
		require.Equal(t, 0, thisReport.CountDeleted)
	})

	t.Run("check created resources", func(t *testing.T) {
		// Only mCSD directory endpoints from discoverable directories + all resources from non-discoverable directories
		require.Len(t, localClient.CreatedResources["Organization"], 1) // 1 organization from org1 directory
		require.Len(t, localClient.CreatedResources["Endpoint"], 5)     // 4 mCSD directory endpoints from root + 1 from org1 directory
	})
}

func parseJSON[T any](data []byte) (*T, error) {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}
