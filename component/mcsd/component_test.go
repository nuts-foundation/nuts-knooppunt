package mcsd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func mockEndpoints(mux *http.ServeMux, responses map[string]*string) {
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
	organizationResponse, err := os.ReadFile("test/regression_lrza_organization_response.json")
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
	organizationResponseStr := string(organizationResponse)
	emptyResponseStr := string(emptyResponse)

	mockEndpoints(mux, map[string]*string{
		"/Endpoint/_history":          &endpointHistoryResponseStr,
		"/Location/_history":          &locationHistoryResponseStr,
		"/Organization/_history":      &organizationHistoryResponseStr,
		"/Organization":               &organizationResponseStr,
		"/HealthcareService/_history": &emptyResponseStr,
		"/PractitionerRole/_history":  &emptyResponseStr,
	})
	server := httptest.NewServer(mux)

	localClient := &test.StubFHIRClient{}
	config := DefaultConfig()
	config.AdministrationDirectories = map[string]DirectoryConfig{
		"lrza": {
			FHIRBaseURL: server.URL,
		},
	}
	component, err := New(config)
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

	mockEndpoints(rootDirMux, map[string]*string{
		"/Endpoint/_history":          &rootDirEndpointHistoryResponse,
		"/Organization/_history":      &rootDirOrganizationHistoryResponse,
		"/Organization":               &rootDirOrganizationHistoryResponse,
		"/HealthcareService/_history": &emptyResponseStr,
		"/Location/_history":          &emptyResponseStr,
		"/PractitionerRole/_history":  &emptyResponseStr,
		"/Practitioner/_history":      &emptyResponseStr,
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

	mockEndpoints(org1DirMux, map[string]*string{
		"/fhir/Endpoint/_history":           &org1DirEndpointHistoryPage1Response,
		"/fhir/Organization/_history":       &org1DirOrganizationHistoryPage1Response,
		"/fhir/Organization":                &org1DirOrganizationHistoryPage1Response,
		"/fhir/Endpoint/_history_page2":     &org1DirEndpointHistoryPage2Response,
		"/fhir/Organization/_history_page2": &org1DirOrganizationHistoryPage2Response,
		"/fhir/Location/_history":           &emptyResponseStr,
		"/fhir/HealthcareService/_history":  &emptyResponseStr,
		"/fhir/PractitionerRole/_history":   &emptyResponseStr,
		"/fhir/Practitioner/_history":       &emptyResponseStr,
	})

	org1DirServer := httptest.NewServer(org1DirMux)

	orgDir1BaseURL := org1DirServer.URL + "/fhir"
	rootDirEndpointHistoryResponse = strings.ReplaceAll(rootDirEndpointHistoryResponse, "{{ORG1_DIR_BASEURL}}", orgDir1BaseURL)
	org1DirEndpointHistoryPage1Response = strings.ReplaceAll(org1DirEndpointHistoryPage1Response, "{{ORG1_DIR_BASEURL}}", orgDir1BaseURL)

	rootDirOrganizationHistoryResponse = strings.ReplaceAll(rootDirOrganizationHistoryResponse, "{{ORG1_DIR_BASEURL}}", orgDir1BaseURL)
	org1DirOrganizationHistoryPage1Response = strings.ReplaceAll(org1DirOrganizationHistoryPage1Response, "{{ORG1_DIR_BASEURL}}", orgDir1BaseURL)

	localClient := &test.StubFHIRClient{}
	config := DefaultConfig()
	config.AdministrationDirectories = map[string]DirectoryConfig{
		"rootDir": {
			FHIRBaseURL: rootDirServer.URL,
		},
	}
	config.QueryDirectory = DirectoryConfig{
		FHIRBaseURL: "http://example.com/local/fhir",
	}
	component, err := New(config)
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
			require.Len(t, thisReport.Warnings, 3)
			// Check that both expected warnings are present (order may vary due to deduplication)
			warnings := strings.Join(thisReport.Warnings, " ")
			require.Contains(t, warnings, "failed to register discovered mCSD Directory at file:///etc/passwd: invalid FHIR base URL (url=file:///etc/passwd)")
			require.Contains(t, warnings, "resource type Something-else not allowed")
			require.Contains(t, warnings, "endpoint must be referenced in at least one organization's endpoint field (endpoint ID: non-dir-endpoint)")
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
	rootDirMux.HandleFunc("/Organization", func(w http.ResponseWriter, r *http.Request) {
		// FHIR client configured to use GET, parameters are in query string
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

	mockEndpoints(rootDirMux, map[string]*string{
		"/Location/_history":          &emptyResponseStr2,
		"/HealthcareService/_history": &emptyResponseStr2,
		"/PractitionerRole/_history":  &emptyResponseStr2,
	})

	rootDirServer := httptest.NewServer(rootDirMux)

	localClient := &test.StubFHIRClient{}
	config := DefaultConfig()
	config.AdministrationDirectories = map[string]DirectoryConfig{
		"rootDir": {
			FHIRBaseURL: rootDirServer.URL,
		},
	}
	config.QueryDirectory = DirectoryConfig{
		FHIRBaseURL: "http://example.com/local/fhir",
	}
	component, err := New(config)
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
		component, err := New(DefaultConfig())
		require.NoError(t, err)
		report, err := component.updateFromDirectory(ctx, server.URL+"/fhir", []string{"Organization"}, false, "")
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
		config := DefaultConfig()
		config.QueryDirectory = DirectoryConfig{
			FHIRBaseURL: "http://example.com/local/fhir",
		}
		component, err := New(config)
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

		report, err := component.updateFromDirectory(ctx, server.URL+"/fhir", []string{"Organization", "Endpoint"}, false, "")

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

		orgBundle := `{
			"resourceType": "Bundle",
			"type": "history",
			"entry": [{
				"fullUrl": "http://test.example.org/fhir/Organization/org",
				 "resource": {
					"resourceType": "Organization",
					"id": "org-4",
					"meta": {
					  "versionId": "1",
					  "lastUpdated": "2025-08-01T14:31:31.987+00:00"
					},
					"identifier": [
					  {
						"use": "official",
						"system": "http://fhir.nl/fhir/NamingSystem/ura",
						"value": "444"
					  }
					],
					"active": true,
					"endpoint": [
					  {
						"reference": "Endpoint/test-endpoint"
					  }
					],
					"name": "Organization 4"
				 },
				 "request": {
					"method": "POST",
					"url": "Organization/org"
				 },
				 "response": {
					"status": "201 Created",
					"etag": "W/\"1\""
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
			w.Write([]byte(orgBundle))
		})
		mux.HandleFunc("/fhir/Organization", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(orgBundle))
		})
		mux.HandleFunc("/fhir/Location/_history", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"resourceType": "Bundle", "type": "history", "entry": []}`))
		})
		mux.HandleFunc("/fhir/HealthcareService/_history", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"resourceType": "Bundle", "type": "history", "entry": []}`))
		})
		mux.HandleFunc("/fhir/PractitionerRole/_history", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"resourceType": "Bundle", "type": "history", "entry": []}`))
		})
		mux.HandleFunc("/fhir/Practitioner/_history", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"resourceType": "Bundle", "type": "history", "entry": []}`))
		})

		config := DefaultConfig()
		config.QueryDirectory = DirectoryConfig{
			FHIRBaseURL: "http://example.com/local/fhir",
		}
		config.AdministrationDirectories = map[string]DirectoryConfig{
			"test-dir": {
				FHIRBaseURL: server.URL + "/fhir",
			},
		}
		component, err := New(config)
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
		report1, err := component.updateFromDirectory(ctx, server.URL+"/fhir", []string{"Endpoint", "Organization"}, true, "")
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
		report2, err := component.updateFromDirectory(ctx, server.URL+"/fhir", []string{"Endpoint", "Organization"}, true, "")
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

		mux.HandleFunc("/fhir/Organization", func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(emptyBundle))
		})

		config := DefaultConfig()
		config.QueryDirectory = DirectoryConfig{
			FHIRBaseURL: "http://example.com/local/fhir",
		}
		component, err := New(config)
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
		report, err := component.updateFromDirectory(ctx, server.URL+"/fhir", allowedTypes, false, "")

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

	t.Run("uses configured DirectoryResourceTypes for discovered endpoints", func(t *testing.T) {
		// This test verifies that when DirectoryResourceTypes is configured,
		// those resource types are used when discovering and registering new Endpoints.

		ctx := context.Background()

		// Track which resource type endpoints were called
		calledEndpoints := make(map[string]bool)
		var mu sync.Mutex

		mux := http.NewServeMux()
		server := httptest.NewServer(mux)
		defer server.Close()

		// Response with an Endpoint that should be discovered
		// Must have the correct payloadType coding to be recognized as an mCSD directory
		discoveredEndpointBundle := `{
			"resourceType": "Bundle",
			"type": "history",
			"entry": [{
				"fullUrl": "http://example.com/Endpoint/discovered-endpoint",
				"resource": {
					"resourceType": "Endpoint",
					"id": "discovered-endpoint",
					"status": "active",
					"connectionType": {
						"system": "http://terminology.hl7.org/CodeSystem/endpoint-connection-type",
						"code": "hl7-fhir-rest"
					},
					"address": "` + server.URL + `/fhir/discovered",
					"payloadType": [{
						"coding": [{
							"system": "http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-data-exchange-capabilities",
							"code": "http://nuts-foundation.github.io/nl-generic-functions-ig/CapabilityStatement/nl-gf-admin-directory-update-client"
						}]
					}]
				},
				"request": {
					"method": "POST",
					"url": "Endpoint"
				}
			}]
		}`
		discoveredOrganizationBundle := `{
			"resourceType": "Bundle",
			"type": "history",
			"entry": [{
				"fullUrl": "http://test.example.org/fhir/Organization/org",
				 "resource": {
					"resourceType": "Organization",
					"id": "org-4",
					"meta": {
					  "versionId": "1",
					  "lastUpdated": "2025-08-01T14:31:31.987+00:00"
					},
					"identifier": [
					  {
						"use": "official",
						"system": "http://fhir.nl/fhir/NamingSystem/ura",
						"value": "444"
					  }
					],
					"active": true,
					"endpoint": [
					  {
						"reference": "Endpoint/discovered-endpoint"
					  }
					],
					"name": "Organization 4"
				 },
				 "request": {
					"method": "POST",
					"url": "Organization/org"
				 },
				 "response": {
					"status": "201 Created",
					"etag": "W/\"1\""
				 }
			}]
		}`

		emptyBundle := `{
			"resourceType": "Bundle",
			"type": "history",
			"entry": []
		}`

		// Set up handlers that track which endpoints are called
		// All potential resource types
		allResourceTypes := []string{"Organization", "Endpoint", "Location", "HealthcareService", "PractitionerRole", "Practitioner"}
		for _, rt := range allResourceTypes {
			resourceType := rt // capture for closure
			mux.HandleFunc("/fhir/"+resourceType+"/_history", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calledEndpoints[resourceType] = true
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				// Return discovered endpoint only for Endpoint queries
				if resourceType == "Endpoint" {
					w.Write([]byte(discoveredEndpointBundle))
				} else if resourceType == "Organization" {
					w.Write([]byte(discoveredOrganizationBundle))
				} else {
					w.Write([]byte(emptyBundle))
				}
			})
			// Handler for discovered directory
			mux.HandleFunc("/fhir/discovered/"+resourceType+"/_history", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calledEndpoints["discovered/"+resourceType] = true
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(emptyBundle))
			})
		}

		mux.HandleFunc("/fhir/Organization", func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(discoveredOrganizationBundle))
		})

		// Create component with custom DirectoryResourceTypes that includes Practitioner
		customResourceTypes := []string{"Organization", "Endpoint", "Practitioner"}
		config := DefaultConfig()
		config.QueryDirectory = DirectoryConfig{
			FHIRBaseURL: "http://example.com/local/fhir",
		}
		config.DirectoryResourceTypes = customResourceTypes
		component, err := New(config)
		require.NoError(t, err)

		// Verify the component stored the custom resource types
		assert.Equal(t, customResourceTypes, component.directoryResourceTypes)

		capturingClient := &test.StubFHIRClient{}
		component.fhirClientFn = func(baseURL *url.URL) fhirclient.Client {
			if strings.HasPrefix(baseURL.String(), server.URL) {
				return fhirclient.New(baseURL, http.DefaultClient, &fhirclient.Config{UsePostSearch: false})
			}
			if baseURL.String() == "http://example.com/local/fhir" {
				return capturingClient
			}
			return &test.StubFHIRClient{Error: errors.New("unknown URL")}
		}

		// Register the root directory (which will query using rootDirectoryResourceTypes: Organization, Endpoint)
		err = component.registerAdministrationDirectory(ctx, server.URL+"/fhir", rootDirectoryResourceTypes, true, "", "")
		require.NoError(t, err)

		// First update should discover the endpoint from root directory and immediately query it
		// because the update() loop processes newly discovered directories in the same iteration
		_, err = component.update(ctx)
		require.NoError(t, err)

		mu.Lock()
		defer mu.Unlock()

		// Verify root directory queries (uses rootDirectoryResourceTypes)
		assert.True(t, calledEndpoints["Organization"], "Root directory should query Organization")
		assert.True(t, calledEndpoints["Endpoint"], "Root directory should query Endpoint")

		// Verify discovered directory queries (uses component.directoryResourceTypes which is customResourceTypes)
		assert.True(t, calledEndpoints["discovered/Organization"], "Discovered directory should query Organization (in customResourceTypes)")
		assert.True(t, calledEndpoints["discovered/Endpoint"], "Discovered directory should query Endpoint (in customResourceTypes)")
		assert.True(t, calledEndpoints["discovered/Practitioner"], "Discovered directory should query Practitioner (in customResourceTypes)")

		// Verify that resource types NOT in customResourceTypes were NOT queried for discovered directory
		assert.False(t, calledEndpoints["discovered/Location"], "Discovered directory should NOT query Location (not in customResourceTypes)")
		assert.False(t, calledEndpoints["discovered/HealthcareService"], "Discovered directory should NOT query HealthcareService (not in customResourceTypes)")
		assert.False(t, calledEndpoints["discovered/PractitionerRole"], "Discovered directory should NOT query PractitionerRole (not in customResourceTypes)")
	})

	t.Run("uses default DirectoryResourceTypes when not configured", func(t *testing.T) {
		// This test verifies that when using DefaultConfig(),
		// the default resource types are set.

		config := DefaultConfig()
		config.QueryDirectory = DirectoryConfig{
			FHIRBaseURL: "http://example.com/local/fhir",
		}
		component, err := New(config)
		require.NoError(t, err)

		// Verify the component uses default resource types
		expectedDefaults := []string{"Organization", "Endpoint", "Location", "HealthcareService", "PractitionerRole", "Practitioner"}
		assert.Equal(t, expectedDefaults, component.directoryResourceTypes)
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
		"/fhir/Organization":               &emptyResponseStr,
		"/fhir/Location/_history":          &emptyResponseStr,
		"/fhir/HealthcareService/_history": &emptyResponseStr,
		"/fhir/PractitionerRole/_history":  &emptyResponseStr,
		"/fhir/Practitioner/_history":      &emptyResponseStr,
	}
	for path, filename := range filesToServe {
		data, err := os.ReadFile(filename)
		require.NoError(t, err)
		dataStr := string(data)
		pathsToServe[path] = &dataStr
	}

	mockEndpoints(mux, pathsToServe)
	return server
}

func TestComponent_registerAdministrationDirectory(t *testing.T) {
	t.Run("excludes administration directory by exact URL match", func(t *testing.T) {
		config := DefaultConfig()
		config.ExcludeAdminDirectories = []string{"http://example.com/fhir"}
		component, err := New(config)
		require.NoError(t, err)

		err = component.registerAdministrationDirectory(context.Background(), "http://example.com/fhir", []string{"Organization"}, false, "", "")

		require.NoError(t, err, "Should not error when URL is excluded, just skip registration")
		assert.Len(t, component.administrationDirectories, 0, "No directories should be registered")
	})

	t.Run("excludes administration directory with trailing slash trimmed", func(t *testing.T) {
		config := DefaultConfig()
		config.ExcludeAdminDirectories = []string{
			"http://example.com/fhir",
		}
		component, err := New(config)
		require.NoError(t, err)

		// Try to register with trailing slash - should still be excluded
		err = component.registerAdministrationDirectory(context.Background(), "http://example.com/fhir/", []string{"Organization"}, false, "", "")

		require.NoError(t, err, "Should not error when URL is excluded, just skip registration")
		assert.Len(t, component.administrationDirectories, 0, "No directories should be registered")
	})

	t.Run("matches exclusion list entries with trailing slashes", func(t *testing.T) {
		config := DefaultConfig()
		config.ExcludeAdminDirectories = []string{
			"http://example.com/fhir/", // Exclusion list has trailing slash
		}
		component, err := New(config)
		require.NoError(t, err)

		// Try to register without trailing slash - should still be excluded due to trimming
		err = component.registerAdministrationDirectory(context.Background(), "http://example.com/fhir", []string{"Organization"}, false, "", "")

		require.NoError(t, err, "Should not error when URL is excluded, just skip registration")
		assert.Len(t, component.administrationDirectories, 0, "No directories should be registered")
	})

	t.Run("matches with both having trailing slashes", func(t *testing.T) {
		config := DefaultConfig()
		config.ExcludeAdminDirectories = []string{
			"http://example.com/fhir/", // Both have trailing slash
		}
		component, err := New(config)
		require.NoError(t, err)

		err = component.registerAdministrationDirectory(context.Background(), "http://example.com/fhir/", []string{"Organization"}, false, "", "")

		require.NoError(t, err, "Should not error when URL is excluded, just skip registration")
		assert.Len(t, component.administrationDirectories, 0, "No directories should be registered")
	})

	t.Run("allows administration directory not in exclusion list", func(t *testing.T) {
		config := DefaultConfig()
		config.ExcludeAdminDirectories = []string{
			"http://excluded.com/fhir",
		}
		component, err := New(config)
		require.NoError(t, err)

		err = component.registerAdministrationDirectory(context.Background(), "http://allowed.com/fhir", []string{"Organization"}, false, "", "")

		require.NoError(t, err)
		assert.Len(t, component.administrationDirectories, 1, "Directory should be registered")
		assert.Equal(t, "http://allowed.com/fhir", component.administrationDirectories[0].fhirBaseURL)
	})

	t.Run("excludes own query directory from being registered as admin directory", func(t *testing.T) {
		ownFHIRBaseURL := "http://localhost:8080/fhir"
		config := DefaultConfig()
		config.QueryDirectory = DirectoryConfig{
			FHIRBaseURL: ownFHIRBaseURL,
		}
		config.ExcludeAdminDirectories = []string{
			ownFHIRBaseURL,
		}
		component, err := New(config)
		require.NoError(t, err)

		// Try to register the same URL as admin directory - should be excluded
		err = component.registerAdministrationDirectory(context.Background(), ownFHIRBaseURL, []string{"Organization"}, true, "", "")

		require.NoError(t, err, "Should not error when URL is excluded, just skip registration")
		assert.Len(t, component.administrationDirectories, 0, "Own directory should not be registered as admin directory")
	})

	t.Run("excludes multiple directories", func(t *testing.T) {
		config := DefaultConfig()
		config.ExcludeAdminDirectories = []string{
			"http://excluded1.com/fhir",
			"http://excluded2.com/fhir",
			"http://excluded3.com/fhir",
		}
		component, err := New(config)
		require.NoError(t, err)

		// Try to register excluded directories
		err1 := component.registerAdministrationDirectory(context.Background(), "http://excluded1.com/fhir", []string{"Organization"}, false, "", "")
		err2 := component.registerAdministrationDirectory(context.Background(), "http://excluded2.com/fhir", []string{"Organization"}, false, "", "")
		err3 := component.registerAdministrationDirectory(context.Background(), "http://excluded3.com/fhir", []string{"Organization"}, false, "", "")

		// Register an allowed directory
		err4 := component.registerAdministrationDirectory(context.Background(), "http://allowed.com/fhir", []string{"Organization"}, false, "", "")

		require.NoError(t, err1, "Should not error when URL is excluded, just skip registration")
		require.NoError(t, err2, "Should not error when URL is excluded, just skip registration")
		require.NoError(t, err3, "Should not error when URL is excluded, just skip registration")
		require.NoError(t, err4)
		assert.Len(t, component.administrationDirectories, 1, "Only the allowed directory should be registered")
	})

	t.Run("empty exclusion list allows all directories", func(t *testing.T) {
		config := DefaultConfig()
		config.ExcludeAdminDirectories = []string{}
		component, err := New(config)
		require.NoError(t, err)

		err = component.registerAdministrationDirectory(context.Background(), "http://example.com/fhir", []string{"Organization"}, false, "", "")

		require.NoError(t, err)
		assert.Len(t, component.administrationDirectories, 1, "Directory should be registered when exclusion list is empty")
	})

	t.Run("invalid URL returns error even if in exclusion list", func(t *testing.T) {
		config := DefaultConfig()
		config.ExcludeAdminDirectories = []string{
			"not-a-valid-url",
		}
		component, err := New(config)
		require.NoError(t, err)

		// Invalid URL should return error, not silently skip
		err = component.registerAdministrationDirectory(context.Background(), "not-a-valid-url", []string{"Organization"}, false, "", "")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid FHIR base URL")
		assert.Len(t, component.administrationDirectories, 0, "Invalid URL should not be registered")
	})
}

func TestFindParentOrganizationWithURA(t *testing.T) {
	tests := []struct {
		name                 string
		entries              []fhir.BundleEntry
		expectedParentID     *string
		expectedLinkedOrgIDs []string
		description          string
	}{
		{
			name: "finds organization with URA identifier directly",
			entries: []fhir.BundleEntry{
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org1"),
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
								Value:  to.Ptr("12345"),
							},
						},
					}),
				},
			},
			expectedParentID:     to.Ptr("org1"),
			expectedLinkedOrgIDs: []string{},
			description:          "should find and return organization with URA identifier with no linked orgs",
		},
		{
			name:                 "returns nil when no organization has URA",
			entries:              []fhir.BundleEntry{},
			expectedParentID:     nil,
			expectedLinkedOrgIDs: nil,
			description:          "should return nil when no entries or no URA found",
		},
		{
			name: "traverses partOf chain to find parent with URA",
			entries: []fhir.BundleEntry{
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org2"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/org1"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org1"),
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
								Value:  to.Ptr("12345"),
							},
						},
					}),
				},
			},
			expectedParentID:     to.Ptr("org1"),
			expectedLinkedOrgIDs: []string{"org2"},
			description:          "should traverse partOf chain to find parent with URA and include org2 as linked",
		},
		{
			name: "traverses multi-level partOf chain",
			entries: []fhir.BundleEntry{
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org3"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/org2"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org2"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/org1"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org1"),
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
								Value:  to.Ptr("99999"),
							},
						},
					}),
				},
			},
			expectedParentID:     to.Ptr("org1"),
			expectedLinkedOrgIDs: []string{"org2", "org3"},
			description:          "should traverse multi-level partOf chain and include all linked orgs",
		},
		{
			name: "handles entries with non-Organization resources",
			entries: []fhir.BundleEntry{
				{
					Resource: mustMarshalResource(&fhir.Location{
						Id: to.Ptr("loc1"),
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org1"),
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
								Value:  to.Ptr("12345"),
							},
						},
					}),
				},
			},
			expectedParentID:     to.Ptr("org1"),
			expectedLinkedOrgIDs: []string{},
			description:          "should skip non-Organization resources and find Organization with URA",
		},
		{
			name: "excludes organizations not linked to parent",
			entries: []fhir.BundleEntry{
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org1"),
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
								Value:  to.Ptr("12345"),
							},
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org2"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/org1"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org3"),
						// No partOf reference - not linked to parent
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org4"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/org5"),
						},
						// References org5 which doesn't exist - not linked to parent
					}),
				},
			},
			expectedParentID:     to.Ptr("org1"),
			expectedLinkedOrgIDs: []string{"org2"},
			description:          "should exclude organizations not linked to parent through partOf",
		},
		{
			name: "handles organizations in different order",
			entries: []fhir.BundleEntry{
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org2"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/org1"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org3"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/org2"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org1"),
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
								Value:  to.Ptr("12345"),
							},
						},
					}),
				},
			},
			expectedParentID:     to.Ptr("org1"),
			expectedLinkedOrgIDs: []string{"org2", "org3"},
			description:          "should work even when parent is not first in entries",
		},
		{
			name: "handles organizations with VersionId in Meta",
			entries: []fhir.BundleEntry{
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org1"),
						Meta: &fhir.Meta{
							VersionId: to.Ptr("2"),
						},
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
								Value:  to.Ptr("12345"),
							},
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("org2"),
						Meta: &fhir.Meta{
							VersionId: to.Ptr("1"),
						},
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/org1"),
						},
					}),
				},
			},
			expectedParentID:     to.Ptr("org1"),
			expectedLinkedOrgIDs: []string{"org2"},
			description:          "should handle organizations with VersionId in Meta",
		},
		{
			name: "handles real-world Vitaly FHIR bundle with multiple departments",
			entries: []fhir.BundleEntry{
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("5139f7b9-bb82-45ea-b979-285a906d4e54"),
						Meta: &fhir.Meta{
							VersionId:   to.Ptr("1"),
							LastUpdated: to.Ptr("2019-06-09T10:42:57.140+02:00"),
						},
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://keycloak.vitaly.parsek.com"),
								Value:  to.Ptr("silver-river-memorial-hospital"),
							},
							{
								System: to.Ptr("http://clinical.vitaly.parsek.com"),
								Value:  to.Ptr("5139f7b9-bb82-45ea-b979-285a906d4e54"),
							},
						},
						Name: to.Ptr("Silver river memorial hospital"),
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("e47e4672-affd-44c9-a4b2-4355efd1ac31"),
						Meta: &fhir.Meta{
							VersionId:   to.Ptr("1"),
							LastUpdated: to.Ptr("2025-12-09T14:18:38.250+01:00"),
						},
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://keycloak.vitaly.parsek.com"),
								Value:  to.Ptr("Goldriver-Hopsital"),
							},
							{
								System: to.Ptr("http://clinical.vitaly.parsek.com"),
								Value:  to.Ptr("e47e4672-affd-44c9-a4b2-4355efd1ac31"),
							},
						},
						Name: to.Ptr("Goldriver Hopsital"),
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("8d194c7c-91d4-4947-9e93-540d49e28877"),
						Meta: &fhir.Meta{
							VersionId:   to.Ptr("6"),
							LastUpdated: to.Ptr("2025-12-10T12:41:33.137+01:00"),
						},
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://keycloak.vitaly.parsek.com"),
								Value:  to.Ptr("vallee-des-fleurs-clinique"),
							},
							{
								System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
								Value:  to.Ptr("01234567"),
							},
							{
								System: to.Ptr("http://clinical.vitaly.parsek.com"),
								Value:  to.Ptr("8d194c7c-91d4-4947-9e93-540d49e28877"),
							},
						},
						Active: to.Ptr(true),
						Name:   to.Ptr("Vallee des fleurs clinique"),
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("65e98500-c3e1-416f-a7cd-24bd7105c5dc"),
						Meta: &fhir.Meta{
							VersionId:   to.Ptr("2"),
							LastUpdated: to.Ptr("2025-12-10T11:06:27.336+01:00"),
							Profile: []string{
								"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization",
							},
						},
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://clinical.vitaly.parsek.com"),
								Value:  to.Ptr("65e98500-c3e1-416f-a7cd-24bd7105c5dc"),
							},
						},
						Active: to.Ptr(true),
						Name:   to.Ptr("Vallee des fleurs clinique - Gyn. Oncology Department"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/8d194c7c-91d4-4947-9e93-540d49e28877"),
							Type:      to.Ptr("Organization"),
							Display:   to.Ptr("Vallee des fleurs clinique"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("1326c27b-9c6f-4606-bd4b-f184b16cde99"),
						Meta: &fhir.Meta{
							VersionId:   to.Ptr("2"),
							LastUpdated: to.Ptr("2025-12-10T20:10:42.165+01:00"),
							Profile: []string{
								"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization",
							},
						},
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://clinical.vitaly.parsek.com"),
								Value:  to.Ptr("1326c27b-9c6f-4606-bd4b-f184b16cde99"),
							},
						},
						Active: to.Ptr(true),
						Name:   to.Ptr("Vallee des fleurs clinique - HPB Oncology Department"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/8d194c7c-91d4-4947-9e93-540d49e28877"),
							Type:      to.Ptr("Organization"),
							Display:   to.Ptr("Vallee des fleurs clinique"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("e82b5ff7-ccda-4d8a-9a2b-8f6a34ec0aa6"),
						Meta: &fhir.Meta{
							VersionId:   to.Ptr("2"),
							LastUpdated: to.Ptr("2025-12-10T20:12:45.030+01:00"),
							Profile: []string{
								"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization",
							},
						},
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://clinical.vitaly.parsek.com"),
								Value:  to.Ptr("e82b5ff7-ccda-4d8a-9a2b-8f6a34ec0aa6"),
							},
						},
						Active: to.Ptr(true),
						Name:   to.Ptr("Vallee des fleurs clinique - Maternal obstetrics Department"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/8d194c7c-91d4-4947-9e93-540d49e28877"),
							Type:      to.Ptr("Organization"),
							Display:   to.Ptr("Vallee des fleurs clinique"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("21f4ea25-7982-4f5b-a629-62f68e5a1b81"),
						Meta: &fhir.Meta{
							VersionId:   to.Ptr("2"),
							LastUpdated: to.Ptr("2025-12-10T20:12:08.333+01:00"),
							Profile: []string{
								"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization",
							},
						},
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://clinical.vitaly.parsek.com"),
								Value:  to.Ptr("21f4ea25-7982-4f5b-a629-62f68e5a1b81"),
							},
						},
						Active: to.Ptr(true),
						Name:   to.Ptr("Vallee des fleurs clinique - Infectious endocarditis Cardiology Department"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/8d194c7c-91d4-4947-9e93-540d49e28877"),
							Type:      to.Ptr("Organization"),
							Display:   to.Ptr("Vallee des fleurs clinique"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("6da8ec8a-3aa1-4dfc-a474-ab6de3f46576"),
						Meta: &fhir.Meta{
							VersionId:   to.Ptr("2"),
							LastUpdated: to.Ptr("2025-12-10T20:11:26.385+01:00"),
							Profile: []string{
								"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization",
							},
						},
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://clinical.vitaly.parsek.com"),
								Value:  to.Ptr("6da8ec8a-3aa1-4dfc-a474-ab6de3f46576"),
							},
						},
						Active: to.Ptr(true),
						Name:   to.Ptr("Vallee des fleurs clinique - CRC Oncology Department"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/8d194c7c-91d4-4947-9e93-540d49e28877"),
							Type:      to.Ptr("Organization"),
							Display:   to.Ptr("Vallee des fleurs clinique"),
						},
					}),
				},
				{
					Resource: mustMarshalResource(&fhir.Organization{
						Id: to.Ptr("5ff0484d-3342-413f-bddb-e40d2d66f542"),
						Meta: &fhir.Meta{
							VersionId:   to.Ptr("2"),
							LastUpdated: to.Ptr("2025-12-10T20:13:36.241+01:00"),
							Profile: []string{
								"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization",
							},
						},
						Identifier: []fhir.Identifier{
							{
								System: to.Ptr("http://clinical.vitaly.parsek.com"),
								Value:  to.Ptr("5ff0484d-3342-413f-bddb-e40d2d66f542"),
							},
						},
						Active: to.Ptr(true),
						Name:   to.Ptr("Vallee des fleurs clinique - Pulmonary Oncology Department"),
						PartOf: &fhir.Reference{
							Reference: to.Ptr("Organization/8d194c7c-91d4-4947-9e93-540d49e28877"),
							Type:      to.Ptr("Organization"),
							Display:   to.Ptr("Vallee des fleurs clinique"),
						},
					}),
				},
			},
			expectedParentID: to.Ptr("8d194c7c-91d4-4947-9e93-540d49e28877"),
			expectedLinkedOrgIDs: []string{
				"65e98500-c3e1-416f-a7cd-24bd7105c5dc", // Gyn. Oncology Department
				"1326c27b-9c6f-4606-bd4b-f184b16cde99", // HPB Oncology Department
				"e82b5ff7-ccda-4d8a-9a2b-8f6a34ec0aa6", // Maternal obstetrics Department
				"21f4ea25-7982-4f5b-a629-62f68e5a1b81", // Infectious endocarditis Cardiology Department
				"6da8ec8a-3aa1-4dfc-a474-ab6de3f46576", // CRC Oncology Department
				"5ff0484d-3342-413f-bddb-e40d2d66f542", // Pulmonary Oncology Department
			},
			description: "should handle real Vitaly bundle with parent org (URA) and 6 departments, excluding 2 unrelated orgs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parentOrgMap, err := createOrganizationTree(tt.entries)

			require.NoError(t, err, tt.description)

			if tt.expectedParentID == nil {
				require.Empty(t, parentOrgMap, tt.description)
			} else {
				require.NotEmpty(t, parentOrgMap, tt.description)
				require.Len(t, parentOrgMap, 1, "should have exactly one parent organization")

				// Extract the single parent organization and its linked orgs
				var parent *fhir.Organization
				var orgs []*fhir.Organization
				for p, linked := range parentOrgMap {
					parent = p
					orgs = linked
				}

				require.NotNil(t, parent, tt.description)
				require.Equal(t, *tt.expectedParentID, *parent.Id, "parent should have expected ID")

				// Verify the parent has a URA identifier
				uraFound := false
				for _, ident := range parent.Identifier {
					if ident.System != nil && *ident.System == "http://fhir.nl/fhir/NamingSystem/ura" {
						uraFound = true
						break
					}
				}
				require.True(t, uraFound, "parent organization should have URA identifier")

				// Verify linked organizations
				require.NotNil(t, orgs, "linked orgs should not be nil when parent found")
				require.Equal(t, len(tt.expectedLinkedOrgIDs), len(orgs), fmt.Sprintf("expected %d linked orgs, got %d", len(tt.expectedLinkedOrgIDs), len(orgs)))

				// Check that all linked org IDs match expected
				linkedOrgIDs := make(map[string]bool)
				for _, org := range orgs {
					require.NotNil(t, org.Id, "linked org should have an ID")
					linkedOrgIDs[*org.Id] = true
				}

				for _, expectedID := range tt.expectedLinkedOrgIDs {
					require.True(t, linkedOrgIDs[expectedID], fmt.Sprintf("expected linked org %s not found in results", expectedID))
				}

				// Verify parent organization is not in linked orgs
				for _, org := range orgs {
					require.NotEqual(t, *parent.Id, *org.Id, "parent organization should not be in linked orgs list")
				}
			}
		})
	}
}

// mustMarshalResource marshals a resource to JSON bytes, panicking on error.
// Used in tests to quickly create bundle entries with resources.
func mustMarshalResource(resource any) []byte {
	data, err := json.Marshal(resource)
	if err != nil {
		panic(err)
	}
	return data
}
