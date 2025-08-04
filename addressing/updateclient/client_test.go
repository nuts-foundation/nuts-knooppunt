package updateclient_test

import (
	"encoding/json"
	"net/http"
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
		bundle, err := client.GetUpdate("/fhir/test", nil)
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
		bundle, err := client.GetUpdate("/fhir/PARTITION-123", nil)
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
		bundle, err := client.GetUpdate("/fhir/error", nil)

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
		bundle, err := client.GetUpdate("/fhir/validate", nil)
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
		bundle, err := client.GetUpdate("/fhir/malformed", nil)
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
		bundle, err := client.GetUpdate("/fhir/with-since", &sinceTime)
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
