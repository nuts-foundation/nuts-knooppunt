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
	"sync"
	"testing"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func mockHistoryEndpoints(mux *http.ServeMux, responses map[string]*string) {
	for endpoint, responsePtr := range responses {
		responsePtr := responsePtr // Capture the pointer in the loop scope
		mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/fhir+json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(*responsePtr))
		})
	}
}

func TestComponent_update_regression(t *testing.T) {
	organizationHistoryResponse, err := os.ReadFile("test/regression_lrza_organization_history_response.json")
	require.NoError(t, err)
	endpointHistoryResponse, err := os.ReadFile("test/regression_lrza_endpoint_history_response.json")
	require.NoError(t, err)
	locationHistoryResponse, err := os.ReadFile("test/regression_lrza_location_history_response.json")
	require.NoError(t, err)
	emptyResponse, err := os.ReadFile("test/regression_lrza_empty_history_response.json")
	require.NoError(t, err)

	mux := http.NewServeMux()
	// Convert []byte responses to strings for pointer approach
	endpointHistoryResponseStr := string(endpointHistoryResponse)
	locationHistoryResponseStr := string(locationHistoryResponse)
	organizationHistoryResponseStr := string(organizationHistoryResponse)
	emptyResponseStr := string(emptyResponse)

	mockHistoryEndpoints(mux, map[string]*string{
		"/Endpoint/_history":          &endpointHistoryResponseStr,
		"/Location/_history":          &locationHistoryResponseStr,
		"/Organization/_history":      &organizationHistoryResponseStr,
		"/HealthcareService/_history": &emptyResponseStr,
		"/PractitionerRole/_history":  &emptyResponseStr,
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
	// Root directories only query Organization and Endpoint resource types
	// Location history is provided in test data but should not be queried (and thus no warnings about it)
	// The test verifies the regression data can be processed without errors
	assert.Empty(t, report[server.URL].Warnings, "should have no warnings since Location is not queried for root directories")
	assert.Empty(t, report[server.URL].Errors)
	assert.NotNil(t, report[server.URL].Errors, "expected an empty slice")
}

func TestComponent_update(t *testing.T) {
	t.Log("mCSD Component is tested limited here, as it requires running FHIR servers and a lot of data. The main logic is tested in the integration tests.")

	rootDirEndpointHistoryResponseBytes, err := os.ReadFile("test/root_dir_endpoint_history_response.json")
	require.NoError(t, err)
	rootDirOrganizationHistoryResponseBytes, err := os.ReadFile("test/root_dir_organization_history_response.json")
	require.NoError(t, err)
	emptyResponse, err := os.ReadFile("test/regression_lrza_empty_history_response.json")
	require.NoError(t, err)

	require.NoError(t, err)
	rootDirEndpointHistoryResponse := string(rootDirEndpointHistoryResponseBytes)
	rootDirOrganizationHistoryResponse := string(rootDirOrganizationHistoryResponseBytes)

	rootDirMux := http.NewServeMux()

	// Convert []byte responses to strings for pointer approach
	emptyResponseStr := string(emptyResponse)

	mockHistoryEndpoints(rootDirMux, map[string]*string{
		"/Endpoint/_history":          &rootDirEndpointHistoryResponse,
		"/Organization/_history":      &rootDirOrganizationHistoryResponse,
		"/HealthcareService/_history": &emptyResponseStr,
		"/Location/_history":          &emptyResponseStr,
		"/PractitionerRole/_history":  &emptyResponseStr,
	})

	rootDirServer := httptest.NewServer(rootDirMux)

	// page 1
	org1DirEndpointHistoryResponsePage1Bytes, err := os.ReadFile("test/org1_dir_endpoint_history_response-page1.json")
	require.NoError(t, err)
	org1DirEndpointHistoryPage1Response := string(org1DirEndpointHistoryResponsePage1Bytes)

	org1DirOrganizationHistoryResponsePage1Bytes, err := os.ReadFile("test/org1_dir_organization_history_response-page1.json")
	require.NoError(t, err)
	org1DirOrganizationHistoryPage1Response := string(org1DirOrganizationHistoryResponsePage1Bytes)

	// page 2
	org1DirEndpointHistoryResponsePage2Bytes, err := os.ReadFile("test/org1_dir_endpoint_history_response-page2.json")
	require.NoError(t, err)
	org1DirEndpointHistoryPage2Response := string(org1DirEndpointHistoryResponsePage2Bytes)
	org1DirOrganizationHistoryResponsePage2Bytes, err := os.ReadFile("test/org1_dir_organization_history_response-page2.json")
	require.NoError(t, err)
	org1DirOrganizationHistoryPage2Response := string(org1DirOrganizationHistoryResponsePage2Bytes)

	org1DirMux := http.NewServeMux()

	mockHistoryEndpoints(org1DirMux, map[string]*string{
		"/fhir/Endpoint/_history":           &org1DirEndpointHistoryPage1Response,
		"/fhir/Organization/_history":       &org1DirOrganizationHistoryPage1Response,
		"/fhir/Endpoint/_history_page2":     &org1DirEndpointHistoryPage2Response,
		"/fhir/Organization/_history_page2": &org1DirOrganizationHistoryPage2Response,
		"/fhir/Location/_history":           &emptyResponseStr,
		"/fhir/HealthcareService/_history":  &emptyResponseStr,
		"/fhir/PractitionerRole/_history":   &emptyResponseStr,
	})

	org1DirServer := httptest.NewServer(org1DirMux)

	orgDir1BaseURL := org1DirServer.URL + "/fhir"
	rootDirEndpointHistoryResponse = strings.ReplaceAll(rootDirEndpointHistoryResponse, "{{ORG1_DIR_BASEURL}}", orgDir1BaseURL)
	org1DirEndpointHistoryPage1Response = strings.ReplaceAll(org1DirEndpointHistoryPage1Response, "{{ORG1_DIR_BASEURL}}", orgDir1BaseURL)

	rootDirOrganizationHistoryResponse = strings.ReplaceAll(rootDirOrganizationHistoryResponse, "{{ORG1_DIR_BASEURL}}", orgDir1BaseURL)
	org1DirOrganizationHistoryPage1Response = strings.ReplaceAll(org1DirOrganizationHistoryPage1Response, "{{ORG1_DIR_BASEURL}}", orgDir1BaseURL)

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
		require.Equal(t, 3, thisReport.CountCreated) // 3 resources: Organization + 2 Endpoints
		require.Equal(t, 0, thisReport.CountUpdated)
		require.Equal(t, 0, thisReport.CountDeleted)
		t.Run("assert meta.source", func(t *testing.T) {
			var endpoint fhir.Endpoint
			for _, resource := range localClient.CreatedResources["Endpoint"] {
				err := json.Unmarshal(resource.(json.RawMessage), &endpoint)
				require.NoError(t, err)
				if *endpoint.Name == "FHIR-2" {
					break
				}
			}
			assert.Equal(t, orgDir1BaseURL+"/Endpoint/ep-2", *endpoint.Meta.Source)
		})
	})
	t.Run("assert sync report from non-existing FHIR server #1", func(t *testing.T) {
		thisReport := report["https://directory1.example.org"]
		require.Equal(t, "failed to query Organization history: _history search failed: 404 Not Found", strings.Join(thisReport.Errors, ""))
		require.Empty(t, thisReport.Warnings)
		require.Equal(t, 0, thisReport.CountCreated)
		require.Equal(t, 0, thisReport.CountUpdated)
		require.Equal(t, 0, thisReport.CountDeleted)
	})
	t.Run("assert sync report from non-existing FHIR server #2", func(t *testing.T) {
		thisReport := report["https://directory2.example.org"]
		require.Equal(t, "failed to query Organization history: _history search failed: 404 Not Found", strings.Join(thisReport.Errors, ""))
		require.Empty(t, thisReport.Warnings)
		require.Equal(t, 0, thisReport.CountCreated)
		require.Equal(t, 0, thisReport.CountUpdated)
		require.Equal(t, 0, thisReport.CountDeleted)
	})

	t.Run("check created resources", func(t *testing.T) {
		// Only mCSD directory endpoints from discoverable directories + all resources from non-discoverable directories
		require.Len(t, localClient.CreatedResources["Organization"], 1) // 1 organization from org1 directory
		require.Len(t, localClient.CreatedResources["Endpoint"], 6)     // 4 mCSD directory endpoints from root + 2 from org1 directory
	})
}

func TestComponent_incrementalUpdates(t *testing.T) {
	testDataJSONOrg, err := os.ReadFile("test/root_dir_organization_history_response.json")
	require.NoError(t, err)
	testDataJSONEndpoint, err := os.ReadFile("test/root_dir_endpoint_history_response.json")
	require.NoError(t, err)
	emptyResponse, err := os.ReadFile("test/regression_lrza_empty_history_response.json")
	require.NoError(t, err)

	require.NoError(t, err)

	var sinceParams []string
	rootDirMux := http.NewServeMux()
	// For incremental updates test, we need custom handlers to capture _since parameters
	rootDirMux.HandleFunc("/Organization/_history", func(w http.ResponseWriter, r *http.Request) {
		// FHIR client configured to use GET, parameters are in query string
		since := r.URL.Query().Get("_since")
		sinceParams = append(sinceParams, since)
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testDataJSONOrg)
	})
	rootDirMux.HandleFunc("/Endpoint/_history", func(w http.ResponseWriter, r *http.Request) {
		// FHIR client configured to use GET, parameters are in query string
		since := r.URL.Query().Get("_since")
		sinceParams = append(sinceParams, since)
		w.Header().Set("Content-Type", "application/fhir+json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(testDataJSONEndpoint)
	})

	// Convert []byte responses to strings for pointer approach
	emptyResponseStr2 := string(emptyResponse)

	mockHistoryEndpoints(rootDirMux, map[string]*string{
		"/Location/_history":          &emptyResponseStr2,
		"/HealthcareService/_history": &emptyResponseStr2,
		"/PractitionerRole/_history":  &emptyResponseStr2,
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
	require.Len(t, sinceParams, 2, "Should have two requests")
	require.Empty(t, sinceParams[0], "First update should not have _since parameter")

	// Verify timestamp was stored
	lastUpdate, exists := component.lastUpdateTimes[rootDirServer.URL]
	require.True(t, exists, "Last update time should be stored")
	require.NotEmpty(t, lastUpdate, "Last update time should not be empty")

	// Second update - should include _since parameter
	_, err = component.update(ctx)
	require.NoError(t, err)
	require.Len(t, sinceParams, 4, "Should have four requests total")
	require.NotEmpty(t, sinceParams[2], "Third update should include _since parameter")
	require.NotEmpty(t, sinceParams[3], "Fourth update should include _since parameter")

	// Verify _since parameter is a valid RFC3339 timestamp
	_, err = time.Parse(time.RFC3339, sinceParams[2])
	require.NoError(t, err, "_since parameter should be valid RFC3339 timestamp")
	_, err = time.Parse(time.RFC3339Nano, sinceParams[2])
	require.NoError(t, err, "_since parameter should be valid RFC3339Nano timestamp")
	_, err = time.Parse(time.RFC3339, sinceParams[3])
	require.NoError(t, err, "_since parameter should be valid RFC3339 timestamp")
	_, err = time.Parse(time.RFC3339Nano, sinceParams[3])
	require.NoError(t, err, "_since parameter should be valid RFC3339Nano timestamp")

	// Verify _since parameter matches the stored timestamp
	require.Equal(t, lastUpdate, sinceParams[2], "_since parameter should match the stored lastUpdate timestamp")
}

func TestComponent_noDuplicateResourcesInTransactionBundle(t *testing.T) {
	// This test verifies that when _history returns multiple versions of the same resource,
	// the transaction bundle sent to the query directory contains no duplicates.
	// This addresses the HAPI error: "Transaction bundle contains multiple resources with ID: urn:uuid:..."
	emptyResponse, err := os.ReadFile("test/regression_lrza_empty_history_response.json")
	require.NoError(t, err)
	historyWithDuplicatesBytes, err := os.ReadFile("test/history_with_duplicates.json")
	require.NoError(t, err)

	mockMux := http.NewServeMux()
	// Convert []byte responses to strings for pointer approach
	historyWithDuplicatesStr := string(historyWithDuplicatesBytes)
	emptyResponseStr3 := string(emptyResponse)

	mockHistoryEndpoints(mockMux, map[string]*string{
		"/Organization/_history":      &historyWithDuplicatesStr,
		"/Location/_history":          &emptyResponseStr3,
		"/Endpoint/_history":          &emptyResponseStr3,
		"/HealthcareService/_history": &emptyResponseStr3,
		"/PractitionerRole/_history":  &emptyResponseStr3,
	})
	mockServer := httptest.NewServer(mockMux)
	defer mockServer.Close()

	capturingClient := &test.StubFHIRClient{}
	component, err := New(Config{
		QueryDirectory: DirectoryConfig{FHIRBaseURL: "http://example.com/local/fhir"},
	})
	require.NoError(t, err)

	// Register as discovered directory to avoid Organization filtering
	err = component.registerAdministrationDirectory(context.Background(), mockServer.URL, []string{"Organization", "Endpoint"}, false, "")
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

func TestComponent_updateFromDirectory(t *testing.T) {
	ctx := context.Background()

	t.Run("#233: no entry.Request in _history results", func(t *testing.T) {
		t.Log("See https://github.com/nuts-foundation/nuts-knooppunt/issues/233")
		server := startMockServer(t, map[string]string{
			"/fhir/Organization/_history": "test/bugs/233-no-bundle-request/organization_response.json",
		})
		component, err := New(Config{})
		require.NoError(t, err)
		report, err := component.updateFromDirectory(ctx, server.URL+"/fhir", []string{"Organization"}, false)
		require.NoError(t, err)
		require.NotNil(t, report)
		require.Len(t, report.Warnings, 1)
		assert.Equal(t, report.Warnings[0], "Skipping entry with no request: #0")
		assert.Empty(t, report.Errors)
		assert.Equal(t, 0, report.CountCreated)
		assert.Equal(t, 0, report.CountUpdated)
		assert.Equal(t, 0, report.CountDeleted)
	})

	t.Run("no duplicate resources in transaction bundle", func(t *testing.T) {
		// This test verifies that when _history returns multiple versions of the same resource,
		// the transaction bundle sent to the query directory contains no duplicates.
		// This addresses the HAPI error: "Transaction bundle contains multiple resources with ID: urn:uuid:..."
		server := startMockServer(t, map[string]string{
			"/fhir/Organization/_history": "test/history_with_duplicates.json",
		})
		defer server.Close()

		capturingClient := &test.StubFHIRClient{}
		component, err := New(Config{
			QueryDirectory: DirectoryConfig{
				FHIRBaseURL: "http://example.com/local/fhir",
			},
		})
		require.NoError(t, err)

		component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
			if baseURL.String() == server.URL+"/fhir" {
				return fhirclient.New(baseURL, http.DefaultClient, &fhirclient.Config{UsePostSearch: false})
			}
			if baseURL.String() == "http://example.com/local/fhir" {
				return capturingClient
			}
			return &test.StubFHIRClient{Error: errors.New("unknown URL")}
		}

		report, err := component.updateFromDirectory(ctx, server.URL+"/fhir", []string{"Organization", "Endpoint"}, false)

		require.NoError(t, err)
		require.Empty(t, report.Errors, "Should not have errors after deduplication")

		// Should have 0 Organizations because the DELETE operation is the most recent
		orgs := capturingClient.CreatedResources["Organization"]
		require.Len(t, orgs, 0, "Should have 0 Organizations after deduplication (DELETE is most recent operation)")
	})

	t.Run("handles DELETE operations for Endpoints and unregisters from administrationDirectories", func(t *testing.T) {
		// This test verifies that when an Endpoint is deleted (DELETE operation in _history),
		// it is properly removed from the query directory and unregistered from administrationDirectories.
		// This fixes issue #241 where deleted Endpoints were cached indefinitely.

		ctx := context.Background()

		// Create test data with an Endpoint that will be deleted
		initialBundle := `{
			"resourceType": "Bundle",
			"type": "history",
			"entry": [{
				"fullUrl": "http://test.example.org/fhir/Endpoint/test-endpoint",
				"resource": {
					"resourceType": "Endpoint",
					"id": "test-endpoint",
					"status": "active",
					"payloadType": [{
						"coding": [{
							"system": "http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-data-exchange-capabilities",
							"code": "http://nuts-foundation.github.io/nl-generic-functions-ig/CapabilityStatement/nl-gf-admin-directory-update-client"
						}]
					}],
					"address": "https://directory.example.org/fhir"
				},
				"request": {
					"method": "POST",
					"url": "Endpoint/test-endpoint"
				}
			}]
		}`

		// Create bundle with DELETE operation for the same Endpoint
		deleteBundle := `{
			"resourceType": "Bundle",
			"type": "history",
			"entry": [{
				"fullUrl": "http://test.example.org/fhir/Endpoint/test-endpoint",
				"request": {
					"method": "DELETE",
					"url": "Endpoint/test-endpoint"
				}
			}]
		}`

		// Create a mock server that returns the initial bundle first, then the delete bundle
		callCount := 0
		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		defer server.Close()

		mux.HandleFunc("/fhir/Endpoint/_history", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if callCount == 0 {
				w.Write([]byte(initialBundle))
			} else {
				w.Write([]byte(deleteBundle))
			}
			callCount++
		})
		mux.HandleFunc("/fhir/Organization/_history", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"resourceType": "Bundle", "type": "history", "entry": []}`))
		})
		mux.HandleFunc("/fhir/Location/_history", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"resourceType": "Bundle", "type": "history", "entry": []}`))
		})
		mux.HandleFunc("/fhir/HealthcareService/_history", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"resourceType": "Bundle", "type": "history", "entry": []}`))
		})

		component, err := New(Config{
			QueryDirectory: DirectoryConfig{
				FHIRBaseURL: "http://example.com/local/fhir",
			},
			AdministrationDirectories: map[string]DirectoryConfig{
				"test-dir": {
					FHIRBaseURL: server.URL + "/fhir",
				},
			},
		})
		require.NoError(t, err)

		// Mock FHIR client that tracks operations
		capturingClient := &test.StubFHIRClient{}
		component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
			if baseURL.String() == server.URL+"/fhir" {
				return fhirclient.New(baseURL, http.DefaultClient, &fhirclient.Config{UsePostSearch: false})
			}
			if baseURL.String() == "http://example.com/local/fhir" {
				return capturingClient
			}
			return &test.StubFHIRClient{Error: errors.New("unknown URL")}
		}

		// First update - should discover and register the Endpoint
		report1, err := component.updateFromDirectory(ctx, server.URL+"/fhir", []string{"Endpoint"}, true)
		require.NoError(t, err)
		require.Empty(t, report1.Errors)
		require.Equal(t, 1, report1.CountCreated, "Should have created 1 Endpoint")

		// Verify Endpoint was created in query directory
		require.NotNil(t, capturingClient.CreatedResources)
		require.Len(t, capturingClient.CreatedResources["Endpoint"], 1, "Endpoint should be created in query directory")

		// Verify Endpoint was discovered and registered with correct fullUrl
		initialAdminDirCount := len(component.administrationDirectories)
		foundEndpoint := false
		var registeredFullUrl string
		for _, dir := range component.administrationDirectories {
			if dir.fhirBaseURL == "https://directory.example.org/fhir" {
				foundEndpoint = true
				registeredFullUrl = dir.sourceURL
				break
			}
		}
		assert.True(t, foundEndpoint, "Endpoint should be registered as administration directory")
		assert.Equal(t, "http://test.example.org/fhir/Endpoint/test-endpoint", registeredFullUrl, "Registered Endpoint should have fullUrl from Bundle entry")

		// Second update - should process DELETE and unregister the Endpoint
		report2, err := component.updateFromDirectory(ctx, server.URL+"/fhir", []string{"Endpoint"}, true)
		require.NoError(t, err)
		require.Empty(t, report2.Errors)

		// Verify DELETE was processed and Endpoint was unregistered
		afterDeleteCount := len(component.administrationDirectories)
		assert.Less(t, afterDeleteCount, initialAdminDirCount, "Deleted Endpoint should be unregistered")

		deletedEndpointStillExists := false
		for _, dir := range component.administrationDirectories {
			if dir.fhirBaseURL == "https://directory.example.org/fhir" {
				deletedEndpointStillExists = true
				break
			}
		}
		assert.False(t, deletedEndpointStillExists, "Deleted Endpoint should not remain in administrationDirectories")

		// Verify DELETE was sent to query directory
		assert.Equal(t, 1, report2.CountDeleted, "Should have 1 deleted resource")
	})

	t.Run("respects allowedResourceTypes parameter and only queries specified resource types", func(t *testing.T) {
		// This test verifies that updateFromDirectory only queries the resource types
		// specified in the allowedResourceTypes parameter, not all resource types.
		// This prevents 404 errors when the FHIR server doesn't support certain resource types.

		ctx := context.Background()

		// Track which resource type endpoints were called
		calledEndpoints := make(map[string]bool)
		var mu sync.Mutex

		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		defer server.Close()

		// Empty bundle response
		emptyBundle := `{
			"resourceType": "Bundle",
			"type": "history",
			"entry": []
		}`

		// Set up handlers that track which endpoints are called
		resourceTypes := []string{"Organization", "Endpoint", "Location", "HealthcareService", "PractitionerRole"}
		for _, rt := range resourceTypes {
			resourceType := rt // capture for closure
			mux.HandleFunc("/fhir/"+resourceType+"/_history", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calledEndpoints[resourceType] = true
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(emptyBundle))
			})
		}

		component, err := New(Config{
			QueryDirectory: DirectoryConfig{
				FHIRBaseURL: "http://example.com/local/fhir",
			},
		})
		require.NoError(t, err)

		capturingClient := &test.StubFHIRClient{}
		component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
			if baseURL.String() == server.URL+"/fhir" {
				return fhirclient.New(baseURL, http.DefaultClient, &fhirclient.Config{UsePostSearch: false})
			}
			if baseURL.String() == "http://example.com/local/fhir" {
				return capturingClient
			}
			return &test.StubFHIRClient{Error: errors.New("unknown URL")}
		}

		// Call updateFromDirectory with only Organization and Endpoint
		allowedTypes := []string{"Organization", "Endpoint"}
		report, err := component.updateFromDirectory(ctx, server.URL+"/fhir", allowedTypes, false)

		require.NoError(t, err)
		require.Empty(t, report.Errors)

		// Verify only the allowed resource types were queried
		mu.Lock()
		defer mu.Unlock()

		assert.True(t, calledEndpoints["Organization"], "Organization/_history should have been called")
		assert.True(t, calledEndpoints["Endpoint"], "Endpoint/_history should have been called")
		assert.False(t, calledEndpoints["Location"], "Location/_history should NOT have been called (not in allowedResourceTypes)")
		assert.False(t, calledEndpoints["HealthcareService"], "HealthcareService/_history should NOT have been called (not in allowedResourceTypes)")
		assert.False(t, calledEndpoints["PractitionerRole"], "PractitionerRole/_history should NOT have been called (not in allowedResourceTypes)")

		// Verify exactly 2 resource types were queried
		assert.Equal(t, 2, len(calledEndpoints), "Should have queried exactly 2 resource types")
	})
}

func startMockServer(t *testing.T, filesToServe map[string]string) *httptest.Server {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	emptyBundleData, err := os.ReadFile("test/empty_bundle_response.json")
	require.NoError(t, err)
	emptyResponseStr := string(emptyBundleData)
	pathsToServe := map[string]*string{
		"/fhir/Endpoint/_history":          &emptyResponseStr,
		"/fhir/Organization/_history":      &emptyResponseStr,
		"/fhir/Location/_history":          &emptyResponseStr,
		"/fhir/HealthcareService/_history": &emptyResponseStr,
	}
	for path, filename := range filesToServe {
		data, err := os.ReadFile(filename)
		require.NoError(t, err)
		dataStr := string(data)
		pathsToServe[path] = &dataStr
	}

	mockHistoryEndpoints(mux, pathsToServe)
	return server
}

func TestComponent_registerAdministrationDirectory(t *testing.T) {
	t.Run("excludes administration directory by exact URL match", func(t *testing.T) {
		component, err := New(Config{
			ExcludeAdminDirectories: []string{
				"http://example.com/fhir",
			},
		})
		require.NoError(t, err)

		err = component.registerAdministrationDirectory(context.Background(), "http://example.com/fhir", []string{"Organization"}, false, "")

		require.NoError(t, err, "Should not error when URL is excluded, just skip registration")
		assert.Len(t, component.administrationDirectories, 0, "No directories should be registered")
	})

	t.Run("excludes administration directory with trailing slash trimmed", func(t *testing.T) {
		component, err := New(Config{
			ExcludeAdminDirectories: []string{
				"http://example.com/fhir",
			},
		})
		require.NoError(t, err)

		// Try to register with trailing slash - should still be excluded
		err = component.registerAdministrationDirectory(context.Background(), "http://example.com/fhir/", []string{"Organization"}, false, "")

		require.NoError(t, err, "Should not error when URL is excluded, just skip registration")
		assert.Len(t, component.administrationDirectories, 0, "No directories should be registered")
	})

	t.Run("matches exclusion list entries with trailing slashes", func(t *testing.T) {
		component, err := New(Config{
			ExcludeAdminDirectories: []string{
				"http://example.com/fhir/", // Exclusion list has trailing slash
			},
		})
		require.NoError(t, err)

		// Try to register without trailing slash - should still be excluded due to trimming
		err = component.registerAdministrationDirectory(context.Background(), "http://example.com/fhir", []string{"Organization"}, false, "")

		require.NoError(t, err, "Should not error when URL is excluded, just skip registration")
		assert.Len(t, component.administrationDirectories, 0, "No directories should be registered")
	})

	t.Run("matches with both having trailing slashes", func(t *testing.T) {
		component, err := New(Config{
			ExcludeAdminDirectories: []string{
				"http://example.com/fhir/", // Both have trailing slash
			},
		})
		require.NoError(t, err)

		err = component.registerAdministrationDirectory(context.Background(), "http://example.com/fhir/", []string{"Organization"}, false, "")

		require.NoError(t, err, "Should not error when URL is excluded, just skip registration")
		assert.Len(t, component.administrationDirectories, 0, "No directories should be registered")
	})

	t.Run("allows administration directory not in exclusion list", func(t *testing.T) {
		component, err := New(Config{
			ExcludeAdminDirectories: []string{
				"http://excluded.com/fhir",
			},
		})
		require.NoError(t, err)

		err = component.registerAdministrationDirectory(context.Background(), "http://allowed.com/fhir", []string{"Organization"}, false, "")

		require.NoError(t, err)
		assert.Len(t, component.administrationDirectories, 1, "Directory should be registered")
		assert.Equal(t, "http://allowed.com/fhir", component.administrationDirectories[0].fhirBaseURL)
	})

	t.Run("excludes own query directory from being registered as admin directory", func(t *testing.T) {
		ownFHIRBaseURL := "http://localhost:8080/fhir"
		component, err := New(Config{
			QueryDirectory: DirectoryConfig{
				FHIRBaseURL: ownFHIRBaseURL,
			},
			ExcludeAdminDirectories: []string{
				ownFHIRBaseURL,
			},
		})
		require.NoError(t, err)

		// Try to register the same URL as admin directory - should be excluded
		err = component.registerAdministrationDirectory(context.Background(), ownFHIRBaseURL, []string{"Organization"}, true, "")

		require.NoError(t, err, "Should not error when URL is excluded, just skip registration")
		assert.Len(t, component.administrationDirectories, 0, "Own directory should not be registered as admin directory")
	})

	t.Run("excludes multiple directories", func(t *testing.T) {
		component, err := New(Config{
			ExcludeAdminDirectories: []string{
				"http://excluded1.com/fhir",
				"http://excluded2.com/fhir",
				"http://excluded3.com/fhir",
			},
		})
		require.NoError(t, err)

		// Try to register excluded directories
		err1 := component.registerAdministrationDirectory(context.Background(), "http://excluded1.com/fhir", []string{"Organization"}, false, "")
		err2 := component.registerAdministrationDirectory(context.Background(), "http://excluded2.com/fhir", []string{"Organization"}, false, "")
		err3 := component.registerAdministrationDirectory(context.Background(), "http://excluded3.com/fhir", []string{"Organization"}, false, "")

		// Register an allowed directory
		err4 := component.registerAdministrationDirectory(context.Background(), "http://allowed.com/fhir", []string{"Organization"}, false, "")

		require.NoError(t, err1, "Should not error when URL is excluded, just skip registration")
		require.NoError(t, err2, "Should not error when URL is excluded, just skip registration")
		require.NoError(t, err3, "Should not error when URL is excluded, just skip registration")
		require.NoError(t, err4)
		assert.Len(t, component.administrationDirectories, 1, "Only the allowed directory should be registered")
	})

	t.Run("empty exclusion list allows all directories", func(t *testing.T) {
		component, err := New(Config{
			ExcludeAdminDirectories: []string{},
		})
		require.NoError(t, err)

		err = component.registerAdministrationDirectory(context.Background(), "http://example.com/fhir", []string{"Organization"}, false, "")

		require.NoError(t, err)
		assert.Len(t, component.administrationDirectories, 1, "Directory should be registered when exclusion list is empty")
	})

	t.Run("invalid URL returns error even if in exclusion list", func(t *testing.T) {
		component, err := New(Config{
			ExcludeAdminDirectories: []string{
				"not-a-valid-url",
			},
		})
		require.NoError(t, err)

		// Invalid URL should return error, not silently skip
		err = component.registerAdministrationDirectory(context.Background(), "not-a-valid-url", []string{"Organization"}, false, "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid FHIR base URL")
		assert.Len(t, component.administrationDirectories, 0, "Invalid URL should not be registered")
	})
}
