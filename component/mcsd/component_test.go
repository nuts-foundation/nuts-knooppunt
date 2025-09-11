package mcsd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_update_regression(t *testing.T) {
	historyResponse, err := os.ReadFile("test/regression_lrza_history_response.json")
	require.NoError(t, err)

	mux := http.NewServeMux()
	mux.HandleFunc("/_history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(historyResponse)
	})
	server := httptest.NewServer(mux)

	localClient := &test.StubFHIRClient{}
	component, err := New(Config{
		AdministrationDirectories: map[string]DirectoryConfig{
			"lrza": {
				FHIRBaseURL: server.URL,
			},
		},
	})
	require.NoError(t, err)
	component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
		if baseURL.String() == server.URL {
			return fhirclient.New(baseURL, http.DefaultClient, nil)
		} else {
			return localClient
		}
	}
	ctx := context.Background()

	report, err := component.update(ctx)

	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Empty(t, report[server.URL].Warnings)
	assert.Empty(t, report[server.URL].Errors)
}

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
	component, err := New(Config{
		AdministrationDirectories: map[string]DirectoryConfig{
			"rootDir": {
				FHIRBaseURL: rootDirServer.URL,
			},
		},
		QueryDirectory: DirectoryConfig{
			FHIRBaseURL: "http://example.com/local/fhir",
		},
	})
	require.NoError(t, err)

	unknownFHIRServerClient := &test.StubFHIRClient{
		Error: errors.New("404 Not Found"),
	}
	component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
		if baseURL.String() == rootDirServer.URL ||
			baseURL.String() == orgDir1BaseURL {
			return fhirclient.New(baseURL, http.DefaultClient, &fhirclient.Config{
				UsePostSearch: false,
			})
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
			// Check that both expected warnings are present (order may vary due to deduplication)
			warnings := strings.Join(thisReport.Warnings, " ")
			require.Contains(t, warnings, "failed to register discovered mCSD Directory at file:///etc/passwd: invalid FHIR base URL (url=file:///etc/passwd)")
			require.Contains(t, warnings, "resource type Something-else not allowed")
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

func TestComponent_incrementalUpdates(t *testing.T) {
	testDataJSON, err := os.ReadFile("test/root_dir_history_response.json")
	require.NoError(t, err)

	var sinceParams []string
	rootDirMux := http.NewServeMux()
	rootDirMux.HandleFunc("/_history", func(w http.ResponseWriter, r *http.Request) {
		// FHIR client configured to use GET, parameters are in query string
		since := r.URL.Query().Get("_since")
		sinceParams = append(sinceParams, since)
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testDataJSON)
	})
	rootDirServer := httptest.NewServer(rootDirMux)

	localClient := &test.StubFHIRClient{}
	component, err := New(Config{
		AdministrationDirectories: map[string]DirectoryConfig{
			"rootDir": {
				FHIRBaseURL: rootDirServer.URL,
			},
		},
		QueryDirectory: DirectoryConfig{
			FHIRBaseURL: "http://example.com/local/fhir",
		},
	})
	require.NoError(t, err)

	component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
		if baseURL.String() == rootDirServer.URL {
			return fhirclient.New(baseURL, http.DefaultClient, &fhirclient.Config{
				UsePostSearch: false,
			})
		}
		if baseURL.String() == "http://example.com/local/fhir" {
			return localClient
		}
		return &test.StubFHIRClient{Error: errors.New("unknown URL")}
	}
	ctx := context.Background()

	// First update - should have no _since parameter
	_, err = component.update(ctx)
	require.NoError(t, err)
	require.Len(t, sinceParams, 1, "Should have one request")
	require.Empty(t, sinceParams[0], "First update should not have _since parameter")

	// Verify timestamp was stored
	lastUpdate, exists := component.lastUpdateTimes[rootDirServer.URL]
	require.True(t, exists, "Last update time should be stored")
	require.NotEmpty(t, lastUpdate, "Last update time should not be empty")

	// Second update - should include _since parameter
	_, err = component.update(ctx)
	require.NoError(t, err)
	require.Len(t, sinceParams, 2, "Should have two requests total")
	require.NotEmpty(t, sinceParams[1], "Second update should include _since parameter")

	// Verify _since parameter is a valid RFC3339 timestamp
	_, err = time.Parse(time.RFC3339, sinceParams[1])
	require.NoError(t, err, "_since parameter should be valid RFC3339 timestamp")

	// Verify _since parameter matches the stored timestamp
	require.Equal(t, lastUpdate, sinceParams[1], "_since parameter should match the stored lastUpdate timestamp")
}

func TestComponent_noDuplicateResourcesInTransactionBundle(t *testing.T) {
	// This test verifies that when _history returns multiple versions of the same resource,
	// the transaction bundle sent to the query directory contains no duplicates.
	// This addresses the HAPI error: "Transaction bundle contains multiple resources with ID: urn:uuid:..."

	historyWithDuplicatesBytes, err := os.ReadFile("test/history_with_duplicates.json")
	require.NoError(t, err)

	mockMux := http.NewServeMux()
	mockMux.HandleFunc("/_history", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(historyWithDuplicatesBytes)
	})
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	capturingClient := &test.StubFHIRClient{}
	component, err := New(Config{
		QueryDirectory: DirectoryConfig{FHIRBaseURL: "http://example.com/local/fhir"},
	})
	require.NoError(t, err)

	// Register as discovered directory to avoid Organization filtering
	err = component.registerAdministrationDirectory(context.Background(), mockServer.URL, []string{"Organization", "Endpoint"}, false)
	require.NoError(t, err)

	component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
		if baseURL.String() == mockServer.URL {
			return fhirclient.New(baseURL, http.DefaultClient, &fhirclient.Config{UsePostSearch: false})
		}
		if baseURL.String() == "http://example.com/local/fhir" {
			return capturingClient
		}
		return &test.StubFHIRClient{Error: errors.New("unknown URL")}
	}

	ctx := context.Background()
	report, err := component.update(ctx)

	require.NoError(t, err)
	require.Empty(t, report[mockServer.URL].Errors, "Should not have errors after deduplication")

	// Should have 0 Organizations because the DELETE operation is the most recent
	orgs := capturingClient.CreatedResources["Organization"]
	require.Len(t, orgs, 0, "Should have 0 Organizations after deduplication (DELETE is most recent operation)")
}

func TestExtractResourceIDFromURL(t *testing.T) {
	tests := []struct {
		name     string
		entry    fhir.BundleEntry
		expected string
	}{
		{
			name: "extract from Request.Url with auto increment FHIR ID",
			entry: fhir.BundleEntry{
				Request: &fhir.BundleEntryRequest{
					Url: "Organization/123",
				},
			},
			expected: "123",
		},
		{
			name: "extract from Request.Url with UUID-format ID",
			entry: fhir.BundleEntry{
				Request: &fhir.BundleEntryRequest{
					Url: "Organization/fd3524f9-705e-453c-8130-71cdf51cfcb9",
				},
			},
			expected: "fd3524f9-705e-453c-8130-71cdf51cfcb9",
		},
		{
			name: "extract from fullUrl when Request.Url is empty",
			entry: fhir.BundleEntry{
				FullUrl: to.Ptr("http://example.org/fhir/Organization/abc123"),
				Request: &fhir.BundleEntryRequest{
					Url: "",
				},
			},
			expected: "abc123",
		},
		{
			name: "extract from fullUrl with UUID-format ID",
			entry: fhir.BundleEntry{
				FullUrl: to.Ptr("http://example.org/fhir/Organization/fd3524f9-705e-453c-8130-71cdf51cfcb9"),
			},
			expected: "fd3524f9-705e-453c-8130-71cdf51cfcb9",
		},
		{
			name: "return empty string when no ID can be extracted",
			entry: fhir.BundleEntry{
				Request: &fhir.BundleEntryRequest{
					Url: "Organization",
				},
			},
			expected: "",
		},
		{
			name: "return empty string when both Request.Url and fullUrl are empty",
			entry: fhir.BundleEntry{
				FullUrl: to.Ptr(""),
				Request: &fhir.BundleEntryRequest{
					Url: "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractResourceIDFromURL(tt.entry)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestIsMoreRecent(t *testing.T) {
	tests := []struct {
		name     string
		entry1   fhir.BundleEntry
		entry2   fhir.BundleEntry
		expected bool
	}{
		{
			name: "entry1 is more recent with timestamps",
			entry1: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"lastUpdated":"2025-08-01T11:00:00.000+00:00"}}`),
			},
			entry2: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"lastUpdated":"2025-08-01T10:00:00.000+00:00"}}`),
			},
			expected: true,
		},
		{
			name: "entry2 is more recent with timestamps",
			entry1: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"lastUpdated":"2025-08-01T10:00:00.000+00:00"}}`),
			},
			entry2: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"lastUpdated":"2025-08-01T11:00:00.000+00:00"}}`),
			},
			expected: false,
		},
		{
			name: "same timestamps",
			entry1: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"lastUpdated":"2025-08-01T10:00:00.000+00:00"}}`),
			},
			entry2: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"lastUpdated":"2025-08-01T10:00:00.000+00:00"}}`),
			},
			expected: false,
		},
		{
			name: "entry1 has no timestamp, entry2 has timestamp",
			entry1: fhir.BundleEntry{
				Resource: []byte(`{}`),
			},
			entry2: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"lastUpdated":"2025-08-01T10:00:00.000+00:00"}}`),
			},
			expected: false,
		},
		{
			name: "both entries have no timestamps (fallback)",
			entry1: fhir.BundleEntry{
				Resource: []byte(`{}`),
			},
			entry2: fhir.BundleEntry{
				Resource: []byte(`{}`),
			},
			expected: false,
		},
		{
			name: "DELETE entry (no resource) vs entry with timestamp",
			entry1: fhir.BundleEntry{
				Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbDELETE},
			},
			entry2: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"lastUpdated":"2025-08-01T10:00:00.000+00:00"}}`),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMoreRecent(tt.entry1, tt.entry2)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetLastUpdated(t *testing.T) {
	tests := []struct {
		name     string
		entry    fhir.BundleEntry
		expected string // Using string for easier comparison, will parse to time.Time
	}{
		{
			name: "valid lastUpdated timestamp",
			entry: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"lastUpdated":"2025-08-01T10:00:00.000+00:00"}}`),
			},
			expected: "2025-08-01T10:00:00.000+00:00",
		},
		{
			name: "no meta field",
			entry: fhir.BundleEntry{
				Resource: []byte(`{"resourceType":"Organization"}`),
			},
			expected: "",
		},
		{
			name: "no lastUpdated field in meta",
			entry: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"versionId":"1"}}`),
			},
			expected: "",
		},
		{
			name: "invalid timestamp format",
			entry: fhir.BundleEntry{
				Resource: []byte(`{"meta":{"lastUpdated":"invalid-date"}}`),
			},
			expected: "",
		},
		{
			name: "no resource (DELETE operation)",
			entry: fhir.BundleEntry{
				Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbDELETE},
			},
			expected: "",
		},
		{
			name: "invalid JSON resource",
			entry: fhir.BundleEntry{
				Resource: []byte(`{invalid json}`),
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getLastUpdated(tt.entry)
			if tt.expected == "" {
				require.True(t, result.IsZero(), "Expected zero time")
			} else {
				expectedTime, err := time.Parse(time.RFC3339, tt.expected)
				require.NoError(t, err, "Test setup error parsing expected time")
				require.Equal(t, expectedTime, result)
			}
		})
	}
}
