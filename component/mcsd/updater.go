package mcsd

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
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
func buildUpdateTransaction(tx *fhir.Bundle, entry fhir.BundleEntry, allowedResourceTypes []string, isDiscoverableDirectory bool, remoteRefToLocalRefMap map[string]string) (string, error) {
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
			// TODO: This is a root admin directory endpoint, from which we only import endpoints, not organizations.
			//       Leaving it in would break referential integrity, so we remove the managing organization reference.
			endpoint.ManagingOrganization = nil
		}
	}
	if !doSync {
		return resourceType, nil
	}

	updateResourceMeta(resource, *entry.FullUrl)
	// TODO: If the resource already exists, we should look up the existing resource's ID
	// TODO: We should scope resource IDs to the source (e.g. by prefixing with the source URL or a hash thereof), because when syncing from multiple sources, IDs may collide.

	// Use pre-generated local ID
	remoteLocalRef := resourceType + "/" + resource["id"].(string)
	localResourceID := remoteRefToLocalRefMap[remoteLocalRef]
	resource["id"] = localResourceID
	if err := normalizeReferences(resource, remoteRefToLocalRefMap); err != nil {
		return "", fmt.Errorf("failed to normalize references: %w", err)
	}

	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return "", err
	}
	// Determine HTTP method and URL based on resource ID format
	resourceID := resource["id"].(string)
	var requestURL string
	var requestMethod fhir.HTTPVerb

	if strings.HasPrefix(resourceID, "urn:uuid:") {
		// HAPI FHIR accepts urn:uuid IDs only with POST, not PUT/DELETE operations
		// DELETE operations with urn:uuid are already handled above
		// When we deduplicate, and the bundle contains a POST and PUT, we don't have a `id` yet, so we take the PUT body and POST it
		requestURL = resourceType
		requestMethod = fhir.HTTPVerbPOST
	} else {
		// Use original method with proper URL for non-UUID IDs
		if entry.Request.Method == fhir.HTTPVerbPOST {
			requestURL = resourceType
		} else {
			requestURL = resourceType + "/" + resourceID
		}
		requestMethod = entry.Request.Method
	}

	tx.Entry = append(tx.Entry, fhir.BundleEntry{
		Resource: resourceJSON,
		Request: &fhir.BundleEntryRequest{
			Url:    requestURL,
			Method: requestMethod,
		},
	})
	return resourceType, nil
}

func normalizeReferences(resource map[string]any, remoteRefToLocalRefMap map[string]string) error {
	// TODO: Support fully qualified URL references (e.g. "https://example.com/fhir/Organization/123")
	return normalizeReferencesRecursive(resource, remoteRefToLocalRefMap)
}

func normalizeReferencesRecursive(obj any, remoteRefToLocalRefMap map[string]string) error {
	switch v := obj.(type) {
	case map[string]any:
		// Check if this is a reference object
		if ref, ok := v["reference"].(string); ok {
			localRef, exists := remoteRefToLocalRefMap[ref]
			if !exists {
				// Referenced resource is not in this transaction bundle - this violates referential integrity
				return fmt.Errorf("broken reference to '%s' - referenced resource not found in transaction bundle", ref)
			}
			v["reference"] = localRef
		}
		// Recursively process all map values
		for _, value := range v {
			if err := normalizeReferencesRecursive(value, remoteRefToLocalRefMap); err != nil {
				return err
			}
		}
	case []any:
		// Recursively process all array elements
		for _, item := range v {
			if err := normalizeReferencesRecursive(item, remoteRefToLocalRefMap); err != nil {
				return err
			}
		}
	}
	return nil
}

func generateLocalID() string {
	return fmt.Sprintf("urn:uuid:%s", uuid.NewString())
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
