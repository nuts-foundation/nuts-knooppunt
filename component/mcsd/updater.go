package mcsd

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
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
// The localRefMap is used to map local references to their full URLs, which is used for correlating resources in the transaction.
// We don't want to copy the resource ID from remote mCSD Directory, as we can't guarantee IDs from external directories are unique.
// This means, we let our local mCSD Directory assign new IDs to resources, but we have to make sure that updates are applied to the right local resources.
func buildUpdateTransaction(tx *fhir.Bundle, entry fhir.BundleEntry, allowedResourceTypes []string, isDiscoverableDirectory bool, localRefMap map[string]string) (string, error) {
	if entry.Resource == nil {
		return "", errors.New("missing 'resource' field")
	}
	if entry.FullUrl == nil {
		return "", errors.New("missing 'fullUrl' field")
	}
	if entry.Request == nil {
		return "", errors.New("missing 'request' field")
	}

	resource := make(map[string]any)
	if err := json.Unmarshal(entry.Resource, &resource); err != nil {
		return "", fmt.Errorf("failed to unmarshal resource (fullUrl=%v): %w", entry.FullUrl, err)
	}
	resourceType, ok := resource["resourceType"].(string)
	if !ok {
		return "", fmt.Errorf("not a valid resourceType (fullUrl=%v)", entry.FullUrl)
	}
	if !slices.Contains(allowedResourceTypes, resourceType) {
		return "", fmt.Errorf("resource type %s not allowed", resourceType)
	}

	// Only sync resources from non-discoverable directories to the query directory
	// Exception: mCSD directory endpoints are synced even from discoverable directories for resilience (e.g. if the root directory is down)
	if isDiscoverableDirectory {
		if resourceType == "Endpoint" {
			// Check if this is an mCSD directory endpoint
			var endpoint fhir.Endpoint
			if err := json.Unmarshal(entry.Resource, &endpoint); err != nil {
				return "", fmt.Errorf("failed to unmarshal Endpoint resource: %w", err)
			}
			// Import mCSD directory endpoints even from discoverable directories
			if !coding.EqualsCode(endpoint.ConnectionType, coding.MCSDConnectionTypeSystem, coding.MCSDConnectionTypeDirectoryCode) {
				return resourceType, nil
			}
		}
	}

	setResourceMetaSource(resource, "")
	// Get or create local reference
	// TODO: If it's an incremental update, the ID shouldn't be generated, but we should find the existing resource by some other means (e.g. identifier)
	localResourceID := localRefMap[*entry.FullUrl]
	if localResourceID == "" {
		localResourceID = fmt.Sprintf("urn:uuid:%s", uuid.NewString())
		localRefMap[*entry.FullUrl] = localResourceID
		resource["id"] = localResourceID
	}
	resourceJSON, _ := json.Marshal(resource)
	tx.Entry = append(tx.Entry, fhir.BundleEntry{
		Resource: resourceJSON,
		Request: &fhir.BundleEntryRequest{
			Url:    resourceType,
			Method: entry.Request.Method,
		},
	})
	return resourceType, nil
}

func setResourceMetaSource(resource map[string]any, source string) {
	if meta, ok := resource["meta"].(map[string]any); ok {
		meta["source"] = source
	} else {
		resource["meta"] = map[string]any{"source": source}
	}
}
