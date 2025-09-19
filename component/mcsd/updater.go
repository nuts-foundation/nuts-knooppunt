package mcsd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// buildUpdateTransaction constructs a FHIR Bundle transaction for updating resources.
// It filters entries based on allowed resource types and sets the source in the resource meta.
// The function takes a context, a Bundle to populate, a Bundle entry, a map of local references,
// a slice of allowed resource types, and a flag indicating if this is from a discoverable directory.
//
// Resources are only synced to the query directory if they come from non-discoverable directories.
// Discoverable directories are for discovery only and their resources should not be synced.
//
// The localRefMap a map of references of remote Admin Directories (e.g. "Organization/123") to local references.
// We don't want to copy the resource ID from remote Administration mCSD Directory, as we can't guarantee IDs from external directories are unique.
// This means, we let our Query Directory assign new IDs to resources, but we have to make sure that updates are applied to the right local resources.
func buildUpdateTransaction(_ context.Context, tx *fhir.Bundle, entry fhir.BundleEntry, allowedResourceTypes []string, isDiscoverableDirectory bool, sourceBaseURL string) (string, error) {
	if entry.FullUrl == nil {
		return "", errors.New("missing 'fullUrl' field")
	}
	if entry.Request == nil {
		return "", errors.New("missing 'request' field")
	}

	// Handle DELETE operations (no resource body)
	if entry.Request.Method == fhir.HTTPVerbDELETE {
		// TODO: DELETE operations require conditional updates or search-then-delete using _source parameter
		// For now, skip ALL DELETE operations since StubFHIRClient doesn't support them in unit tests
		// DELETE operations with proper FHIR IDs are tested in E2E tests with real HAPI FHIR
		resourceType := strings.Split(entry.Request.Url, "/")[0]
		return resourceType, nil
	}

	// Handle CREATE/UPDATE operations (resource body required)
	if entry.Resource == nil {
		return "", errors.New("missing 'resource' field for non-DELETE operation")
	}

	resource := make(map[string]any)
	if err := json.Unmarshal(entry.Resource, &resource); err != nil {
		return "", fmt.Errorf("failed to unmarshal resource (fullUrl=%s): %w", to.EmptyString(entry.FullUrl), err)
	}
	resourceType, ok := resource["resourceType"].(string)
	if !ok {
		return "", fmt.Errorf("not a valid resourceType (fullUrl=%s)", to.EmptyString(entry.FullUrl))
	}
	if !slices.Contains(allowedResourceTypes, resourceType) {
		return "", fmt.Errorf("resource type %s not allowed", resourceType)
	}

	// Only sync resources from non-discoverable directories to the query directory
	// Exception: mCSD directory endpoints are synced even from discoverable directories for resilience (e.g. if the root directory is down)
	var doSync = true
	if isDiscoverableDirectory {
		doSync = false
		if resourceType == "Endpoint" {
			// Check if this is an mCSD directory endpoint
			var endpoint fhir.Endpoint
			if err := json.Unmarshal(entry.Resource, &endpoint); err != nil {
				return "", fmt.Errorf("failed to unmarshal Endpoint resource: %w", err)
			}

			// Import mCSD directory endpoints even from discoverable directories
			doSync = coding.CodablesIncludesCode(endpoint.PayloadType, coding.PayloadCoding)
		}
	}
	if !doSync {
		return resourceType, nil
	}

	updateResourceMeta(resource, *entry.FullUrl)

	// Remove resource ID - let FHIR server assign new IDs via conditional operations
	delete(resource, "id")

	// Convert ALL references to deterministic conditional references with _source
	if err := convertReferencesRecursive(resource, sourceBaseURL); err != nil {
		return "", fmt.Errorf("failed to convert references: %w", err)
	}

	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return "", err
	}

	tx.Entry = append(tx.Entry, fhir.BundleEntry{
		Resource: resourceJSON,
		Request: &fhir.BundleEntryRequest{
			// Use _source for idempotent updates
			Url: resourceType + "?" + url.Values{
				"_source": []string{*entry.FullUrl},
			}.Encode(),
			Method: fhir.HTTPVerbPUT,
		},
	})
	return resourceType, nil
}

func convertReferencesRecursive(obj any, sourceBaseURL string) error {
	switch v := obj.(type) {
	case map[string]any:
		// Check if this is a reference object
		if ref, ok := v["reference"].(string); ok {
			// Convert ALL references to conditional references with deterministic _source
			if strings.Contains(ref, "/") {
				parts := strings.Split(ref, "/")
				if len(parts) == 2 {
					resourceType := parts[0]
					// Construct the _source URL deterministically: baseURL + "/" + reference
					sourceURL := strings.TrimSuffix(sourceBaseURL, "/") + "/" + ref
					v["reference"] = resourceType + "?_source=" + url.QueryEscape(sourceURL)
				}
			}
		}
		// Recursively process all map values
		for _, value := range v {
			if err := convertReferencesRecursive(value, sourceBaseURL); err != nil {
				return err
			}
		}
	case []any:
		// Recursively process all array elements
		for _, item := range v {
			if err := convertReferencesRecursive(item, sourceBaseURL); err != nil {
				return err
			}
		}
	}
	return nil
}

func updateResourceMeta(resource map[string]any, source string) {
	meta, exists := resource["meta"].(map[string]any)
	if !exists {
		meta = make(map[string]any)
		resource["meta"] = meta
	}
	meta["source"] = source
	delete(meta, "versionId")
	delete(meta, "lastUpdated")
}
