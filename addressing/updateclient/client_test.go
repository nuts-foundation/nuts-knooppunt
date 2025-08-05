package updateclient_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/addressing/updateclient"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// validateBundle is a helper function to validate bundle properties and content
func validateBundle(t *testing.T, bundle *fhir.Bundle, expectedNil bool, expectedType fhir.BundleType, expectedTotal int, expectedResources map[string]int) {
	t.Helper()

	if expectedNil {
		if bundle != nil {
			t.Errorf("Expected nil bundle, got %v", bundle)
		}
		return
	}

	// If we expect a non-nil bundle, verify it exists
	if bundle == nil {
		t.Errorf("Expected a bundle, got nil")
		return
	}

	if bundle.Type != expectedType {
		t.Errorf("Expected Type '%s', got '%s'", expectedType, bundle.Type)
	}

	if *bundle.Total != expectedTotal {
		t.Errorf("Expected Total %d, got %d", expectedTotal, bundle.Total)
	}

	// If we have expected resources, validate them
	if expectedResources != nil && len(expectedResources) > 0 {
		// Count occurrences of each resource type
		resourceCounts := make(map[string]int)

		for _, entry := range bundle.Entry {
			resource := map[string]any{}
			json.Unmarshal(entry.Resource, &resource)
			resourceType, ok := resource["resourceType"].(string)
			if !ok {
				t.Errorf("Entry resource missing resourceType field")
				continue
			}
			resourceCounts[resourceType]++
		}

		// Verify we have the expected counts
		for resourceType, expectedCount := range expectedResources {
			actualCount := resourceCounts[resourceType]
			if actualCount != expectedCount {
				t.Errorf("Expected %d resources of type '%s', got %d", expectedCount, resourceType, actualCount)
			}
		}
	}
}

func TestUpdateClient_GetUpdate(t *testing.T) {

	t.Run("test server setup", func(t *testing.T) {
		// Create test server
		server := NewTestServer()
		defer server.Close()
		// Start the server
		server.Start()
		// Check if the server is running
		resp, err := http.Get(server.URL() + "/status")
		if err != nil {
			t.Fatalf("Failed to connect to test server: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status OK, got %s", resp.Status)
		}
		t.Logf("Test server is running at %s", server.URL())
	})

	t.Run("sync with empty history bundle", func(t *testing.T) {
		// Create test server
		server := NewTestServer()
		defer server.Close()

		// Configure the test server to serve our empty history JSON response
		server.AddJSONFileHandler("/fhir/test/_history", "lrza_empty_history_response.json")

		server.Start()

		// Create a client that points to our test server
		client := updateclient.NewUpdateClient(updateclient.WithBaseURL(server.URL()))

		// Make the request without a since parameter
		bundle, err := client.GetHistoryBundle("/fhir/test", nil)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Validate the bundle using our helper function
		validateBundle(t, bundle, false, fhir.BundleTypeHistory, 0, nil)
	})

	t.Run("sync with history entries", func(t *testing.T) {
		// Create test server
		server := NewTestServer()
		defer server.Close()

		// Configure the test server to serve our JSON response with history entries
		server.AddJSONFileHandler("/fhir/PARTITION-123/_history", "lrza_initial_history_response.json")
		server.Start()

		// Create a client that points to our test server
		client := updateclient.NewUpdateClient(updateclient.WithBaseURL(server.URL()))

		// Make the request without a since parameter
		bundle, err := client.GetHistoryBundle("/fhir/PARTITION-123", nil)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Validate the bundle using our helper function
		// Check for one Endpoint and one Organization resource
		expectedResources := map[string]int{
			"Endpoint":     1,
			"Organization": 1,
		}
		validateBundle(t, bundle, false, fhir.BundleTypeHistory, 2, expectedResources)

		// Also check that we have the expected number of entries
		if bundle != nil && len(bundle.Entry) != 2 {
			t.Errorf("Expected 2 entries, got %d", len(bundle.Entry))
		}
	})

	t.Run("server error response", func(t *testing.T) {
		// Create test server
		server := NewTestServer()
		defer server.Close()

		// Configure the test server to return a 500 error
		server.AddCustomHandler("/fhir/error/_history", http.StatusInternalServerError, `{"error":"Internal server error"}`)
		server.Start()

		// Create a client that points to our test server
		client := updateclient.NewUpdateClient(updateclient.WithBaseURL(server.URL()))

		// Make the request
		bundle, err := client.GetHistoryBundle("/fhir/error", nil)

		// Verify that we got an error
		if err == nil {
			t.Errorf("Expected an error, got nil")
		} else if !strings.Contains(err.Error(), "unexpected status code: 500") {
			t.Errorf("Expected error with status code 500, got: %v", err)
		}

		// Validate the bundle using our helper function - expecting nil bundle
		validateBundle(t, bundle, true, fhir.BundleTypeBatch, 0, nil)
	})

	t.Run("request validation", func(t *testing.T) {
		// Create test server
		server := NewTestServer()
		defer server.Close()

		// Configure the test server with a validation handler
		server.AddRequestValidationHandler("/fhir/validate/_history", func(r *http.Request) (int, string) {
			// Check for required query parameter
			if r.URL.Query().Get("_since") == "" {
				return http.StatusBadRequest, `{"error":"Missing required parameter: _since"}`
			}
			return http.StatusOK, `{"resourceType":"Bundle","type":"history","total":0}`
		})
		server.Start()

		// Create a client that points to our test server
		client := updateclient.NewUpdateClient(updateclient.WithBaseURL(server.URL()))

		// Make the request without a since parameter - should fail validation
		bundle, err := client.GetHistoryBundle("/fhir/validate", nil)
		// Our client doesn't add the _since parameter, so we expect this to fail with a 400
		if err == nil {
			t.Errorf("Expected an error, got nil")
		} else if !strings.Contains(err.Error(), "unexpected status code: 400") {
			t.Errorf("Expected error with status code 400, got: %v", err)
		}

		// Validate the bundle using our helper function - expecting nil bundle
		validateBundle(t, bundle, true, fhir.BundleType(0), 0, nil)
	})

	t.Run("malformed JSON response", func(t *testing.T) {
		// Create test server
		server := NewTestServer()
		defer server.Close()

		// Configure the test server to serve our malformed JSON response
		server.AddJSONFileHandler("/fhir/malformed/_history", "malformed_response.json")
		server.Start()

		// Create a client that points to our test server
		client := updateclient.NewUpdateClient(updateclient.WithBaseURL(server.URL()))

		// Make the request
		bundle, err := client.GetHistoryBundle("/fhir/malformed", nil)
		if err == nil {
			t.Errorf("Expected an error for malformed JSON, got nil")
		} else if !strings.Contains(err.Error(), "failed to decode response") {
			t.Errorf("Expected error about failed decoding, got: %v", err)
		}

		// Validate the bundle using our helper function - expecting nil bundle
		validateBundle(t, bundle, true, 0, 0, nil)
	})

	t.Run("with since parameter", func(t *testing.T) {
		// Create test server
		server := NewTestServer()
		defer server.Close()

		// Capture the request to validate the since parameter is correctly used
		var capturedRequest *http.Request
		server.AddRequestValidationHandler("/fhir/with-since/_history", func(r *http.Request) (int, string) {
			capturedRequest = r
			return http.StatusOK, `{"resourceType":"Bundle","type":"history","total":0}`
		})

		server.Start()

		// Create a client that points to our test server
		client := updateclient.NewUpdateClient(updateclient.WithBaseURL(server.URL()))

		// Make the request with a since parameter
		sinceTime := time.Date(2025, 8, 1, 10, 15, 30, 0, time.UTC)
		bundle, err := client.GetHistoryBundle("/fhir/with-since", &sinceTime)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Validate the bundle using our helper function
		validateBundle(t, bundle, false, fhir.BundleTypeHistory, 0, nil)

		// Verify the _since parameter was correctly formatted and added to the request
		if capturedRequest == nil {
			t.Fatalf("Request was not captured")
		}
		sinceParam := capturedRequest.URL.Query().Get("_since")
		if sinceParam == "" {
			t.Errorf("Expected _since parameter to be set")
		}
		expectedSince := "2025-08-01T10:15:30Z" // RFC3339 format
		if sinceParam != expectedSince {
			t.Errorf("Expected _since=%s, got %s", expectedSince, sinceParam)
		}
	})
}

func TestUpdateClient_GetOrganizationsPerDirectory(t *testing.T) {
	// Create test server
	server := NewTestServer()
	defer server.Close()

	// Configure the test server to serve our JSON response with organizations
	server.AddJSONFileHandler("/fhir/_history", "lrza_initial_history_response.json")
	server.Start()

	// Create a client that points to our test server
	client := updateclient.NewUpdateClient(updateclient.WithBaseURL(server.URL()))

	historyBundle, err := client.GetHistoryBundle("/fhir", nil)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	dirOrgMap, err := client.GetOrganisationsPerDirectory(historyBundle)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Validate the organizations
	if len(dirOrgMap) != 1 {
		t.Errorf("Expected 1 directory, got %d", len(dirOrgMap))
	}

	expectedURL, _ := url.Parse("http://localhost:8080/fhir/OrgA/")

	// Check the directory URL
	orgs, ok := dirOrgMap[*expectedURL]
	if !ok {
		t.Errorf("Expected directory 'http://localhost:8080/fhir/OrgA/' to be in map")
	}

	if len(orgs) != 1 {
		t.Errorf("Expected 1 organization in directory 'foo', got %d", len(orgs))
	}

	if orgs[0] != "124" {
		t.Errorf("Expected organization ID '124', got '%s'", orgs[0])
	}
}

func TestExcludeUnauthorizedEntries(t *testing.T) {
	// Test case 1: Nil bundle should return an error
	t.Run("nil bundle", func(t *testing.T) {
		result, err := updateclient.ExcludeUnauthorizedEntries(nil, []string{"124"})
		if err == nil {
			t.Errorf("Expected an error for nil bundle, got nil")
		}
		if result != nil {
			t.Errorf("Expected nil result for nil bundle, got %v", result)
		}
	})

	// Test case 2: Empty bundle should return an empty bundle
	t.Run("empty bundle", func(t *testing.T) {
		total := 0
		emptyBundle := &fhir.Bundle{
			Type:  fhir.BundleTypeHistory,
			Total: &total,
			Entry: []fhir.BundleEntry{},
		}
		result, err := updateclient.ExcludeUnauthorizedEntries(emptyBundle, []string{"124"})
		if err != nil {
			t.Errorf("Expected no error for empty bundle, got %v", err)
		}
		if result == nil {
			t.Errorf("Expected non-nil result for empty bundle")
		}
		if len(result.Entry) != 0 {
			t.Errorf("Expected empty entries in result, got %d entries", len(result.Entry))
		}
	})

	// Test case 3: Bundle with authorized organization should keep the organization
	t.Run("authorized organization", func(t *testing.T) {
		// Create an organization resource with an authorized identifier
		system := "http://fhir.nl/fhir/NamingSystem/ura"
		value := "124"
		id := "org-1"
		orgResource := createOrganizationResource(t, &id, &system, &value)
		orgJson, _ := json.Marshal(orgResource)

		// Create a bundle with the authorized organization
		total := 1
		bundle := &fhir.Bundle{
			Type:  fhir.BundleTypeHistory,
			Total: &total,
			Entry: []fhir.BundleEntry{
				{
					Resource: orgJson,
				},
			},
		}

		// Call the function with authorized org IDs
		result, err := updateclient.ExcludeUnauthorizedEntries(bundle, []string{"124"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Validate the bundle contains the authorized organization
		if result == nil {
			t.Fatalf("Expected non-nil result")
		}
		if len(result.Entry) != 1 {
			t.Errorf("Expected 1 entry in result, got %d entries", len(result.Entry))
		}

		// Extract resource type and validate
		if len(result.Entry) > 0 {
			resourceType, err := extractResourceType(result.Entry[0].Resource)
			if err != nil {
				t.Errorf("Failed to extract resource type: %v", err)
			}
			if resourceType != "Organization" {
				t.Errorf("Expected Organization resource, got %s", resourceType)
			}

			// Extract organization and validate identifier
			org := &fhir.Organization{}
			if err := json.Unmarshal(result.Entry[0].Resource, org); err != nil {
				t.Errorf("Failed to unmarshal Organization: %v", err)
			}
			if org.Id == nil || *org.Id != "org-1" {
				t.Errorf("Expected organization ID 'org-1', got %v", org.Id)
			}
		}
	})

	// Test case 4: Bundle with unauthorized organization should remove the organization
	t.Run("unauthorized organization", func(t *testing.T) {
		// Create an organization resource with an unauthorized identifier
		system := "http://fhir.nl/fhir/NamingSystem/ura"
		value := "999"
		id := "org-2"
		orgResource := createOrganizationResource(t, &id, &system, &value)
		orgJson, _ := json.Marshal(orgResource)

		// Create a bundle with the unauthorized organization
		total := 1
		bundle := &fhir.Bundle{
			Type:  fhir.BundleTypeHistory,
			Total: &total,
			Entry: []fhir.BundleEntry{
				{
					Resource: orgJson,
				},
			},
		}

		// Call the function with authorized org IDs that don't include this one
		result, err := updateclient.ExcludeUnauthorizedEntries(bundle, []string{"124"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Validate the bundle has no entries (unauthorized org was removed)
		if result == nil {
			t.Fatalf("Expected non-nil result")
		}
		if len(result.Entry) != 0 {
			t.Errorf("Expected 0 entries in result (unauthorized org removed), got %d entries", len(result.Entry))
		}
	})

	// Test case 5: Bundle with mixed resource types should only keep authorized organizations
	t.Run("mixed resource types", func(t *testing.T) {
		// Create an organization resource with an authorized identifier
		system := "http://fhir.nl/fhir/NamingSystem/ura"
		value := "124"
		id := "org-1"
		orgResource := createOrganizationResource(t, &id, &system, &value)
		orgJson, _ := json.Marshal(orgResource)

		// Create an endpoint resource (non-Organization type)
		endpointResource := map[string]any{
			"resourceType": "Endpoint",
			"id":           "endpoint-1",
			"status":       "active",
			"address":      "http://example.org",
		}
		endpointJson, _ := json.Marshal(endpointResource)

		// Create a bundle with mixed resource types
		total := 2
		bundle := &fhir.Bundle{
			Type:  fhir.BundleTypeHistory,
			Total: &total,
			Entry: []fhir.BundleEntry{
				{
					Resource: orgJson,
				},
				{
					Resource: endpointJson,
				},
			},
		}

		// Call the function with authorized org IDs
		result, err := updateclient.ExcludeUnauthorizedEntries(bundle, []string{"124"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Validate only authorized Organization is kept (Endpoint should be removed)
		if result == nil {
			t.Fatalf("Expected non-nil result")
		}
		if len(result.Entry) != 1 {
			t.Errorf("Expected 1 entry in result (only authorized org), got %d entries", len(result.Entry))
		}

		// Validate the remaining entry is the Organization
		if len(result.Entry) > 0 {
			resourceType, err := extractResourceType(result.Entry[0].Resource)
			if err != nil {
				t.Errorf("Failed to extract resource type: %v", err)
			}
			if resourceType != "Organization" {
				t.Errorf("Expected Organization resource, got %s", resourceType)
			}
		}
	})

	// Test case 6: Organization with no identifiers should be removed
	t.Run("organization without identifier", func(t *testing.T) {
		// Create an organization resource with no identifier
		id := "org-3"
		orgResource := createOrganizationResource(t, &id, nil, nil)
		orgJson, _ := json.Marshal(orgResource)

		// Create a bundle with the organization without identifier
		total := 1
		bundle := &fhir.Bundle{
			Type:  fhir.BundleTypeHistory,
			Total: &total,
			Entry: []fhir.BundleEntry{
				{
					Resource: orgJson,
				},
			},
		}

		// Call the function with any org IDs
		result, err := updateclient.ExcludeUnauthorizedEntries(bundle, []string{"124"})
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Validate the bundle has no entries (org with no identifier was removed)
		if result == nil {
			t.Fatalf("Expected non-nil result")
		}
		if len(result.Entry) != 0 {
			t.Errorf("Expected 0 entries in result (org with no identifier removed), got %d entries", len(result.Entry))
		}
	})
}

// Helper function to create an Organization resource for testing
func createOrganizationResource(t *testing.T, id, identifierSystem, identifierValue *string) *fhir.Organization {
	t.Helper()
	active := true
	org := &fhir.Organization{
		Id:     id,
		Active: &active,
	}

	if identifierSystem != nil && identifierValue != nil {
		org.Identifier = []fhir.Identifier{
			{
				System: identifierSystem,
				Value:  identifierValue,
			},
		}
	}

	return org
}

// Helper function to extract resource type from a raw JSON resource
func extractResourceType(r json.RawMessage) (string, error) {
	resource := struct {
		ResourceType string `json:"resourceType"`
	}{}

	if err := json.Unmarshal(r, &resource); err != nil {
		return "", err
	}
	return resource.ResourceType, nil
}
