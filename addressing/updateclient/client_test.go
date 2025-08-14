package updateclient

import (
	"encoding/json"
	"net/url"
	"os"
	"slices"
	"testing"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestExcludeUnauthorizedEntries(t *testing.T) {
	// Test case 1: Nil bundle should return an error
	t.Run("nil bundle", func(t *testing.T) {
		result, err := excludeUnauthorizedEntries(nil, []string{"124"})
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
		result, err := excludeUnauthorizedEntries(emptyBundle, []string{"124"})
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
		result, err := excludeUnauthorizedEntries(bundle, []string{"124"})
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
		result, err := excludeUnauthorizedEntries(bundle, []string{"124"})
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
		result, err := excludeUnauthorizedEntries(bundle, []string{"124"})
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
		result, err := excludeUnauthorizedEntries(bundle, []string{"124"})
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

func TestUpdateClient_GetOrganisationsPerDirectory(t *testing.T) {
	// Test case 1: Nil bundle should return an error
	t.Run("nil bundle", func(t *testing.T) {
		client := &UpdateClient{}
		_, err := client.GetOrganisationsPerDirectory(nil)
		if err == nil {
			t.Errorf("Expected an error for nil bundle, got nil")
		}
	})

	// Test case 2: Empty bundle should return an empty map
	t.Run("empty bundle", func(t *testing.T) {
		total := 0
		emptyBundle := &fhir.Bundle{
			Type:  fhir.BundleTypeHistory,
			Total: &total,
			Entry: []fhir.BundleEntry{},
		}
		client := &UpdateClient{}
		result, err := client.GetOrganisationsPerDirectory(emptyBundle)
		if err != nil {
			t.Errorf("Expected no error for empty bundle, got %v", err)
		}
		if len(result) != 0 {
			t.Errorf("Expected empty result for empty bundle, got %d entries", len(result))
		}
	})

	// Test case 3: Bundle with 2 directories having 2 organizations each
	t.Run("multiple directories with multiple organizations", func(t *testing.T) {
		// Load test bundle file with all resources
		bundleData, err := os.ReadFile("testdata/test_bundle_with_directories.json")
		if err != nil {
			t.Fatalf("Failed to load test_bundle_with_directories.json: %v", err)
		}
		
		// Parse the bundle
		bundle := &fhir.Bundle{}
		if err := json.Unmarshal(bundleData, bundle); err != nil {
			t.Fatalf("Failed to unmarshal test bundle: %v", err)
		}

		// Define expected values for validation
		dir1Url := "https://directory1.example.org"
		dir2Url := "https://directory2.example.org"
		org1Value := "111"
		org2Value := "222"
		org3Value := "333"
		org4Value := "444"

		// Call the function
		client := &UpdateClient{}
		result, err := client.GetOrganisationsPerDirectory(bundle)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		// Validate the result
		if len(result) != 2 {
			t.Errorf("Expected 2 directories in result, got %d", len(result))
		}

		// Parse URLs for comparison
		dir1ParsedUrl, _ := url.Parse(dir1Url)
		dir2ParsedUrl, _ := url.Parse(dir2Url)

		// Check directory 1 organizations
		dir1Orgs, ok := result[*dir1ParsedUrl]
		if !ok {
			t.Errorf("Expected directory1 URL in result, not found")
		} else {
			if len(dir1Orgs) != 2 {
				t.Errorf("Expected 2 organizations for directory1, got %d", len(dir1Orgs))
			}
			if !slices.Contains(dir1Orgs, org1Value) {
				t.Errorf("Expected organization 1 (%s) in directory1 orgs, not found", org1Value)
			}
			if !slices.Contains(dir1Orgs, org2Value) {
				t.Errorf("Expected organization 2 (%s) in directory1 orgs, not found", org2Value)
			}
		}

		// Check directory 2 organizations
		dir2Orgs, ok := result[*dir2ParsedUrl]
		if !ok {
			t.Errorf("Expected directory2 URL in result, not found")
		} else {
			if len(dir2Orgs) != 2 {
				t.Errorf("Expected 2 organizations for directory2, got %d", len(dir2Orgs))
			}
			if !slices.Contains(dir2Orgs, org3Value) {
				t.Errorf("Expected organization 3 (%s) in directory2 orgs, not found", org3Value)
			}
			if !slices.Contains(dir2Orgs, org4Value) {
				t.Errorf("Expected organization 4 (%s) in directory2 orgs, not found", org4Value)
			}
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

// Helper function to create an Endpoint resource for testing
func createEndpointResource(t *testing.T, id string, address string, orgReference *string, connectionType string) *fhir.Endpoint {
	t.Helper()

	status := fhir.EndpointStatusActive
	endpoint := &fhir.Endpoint{
		Id:      &id,
		Status:  status,
		Address: address,
		ConnectionType: fhir.Coding{
			Code: &connectionType,
		},
	}

	if orgReference != nil {
		endpoint.ManagingOrganization = &fhir.Reference{
			Reference: orgReference,
		}
	}

	return endpoint
}
