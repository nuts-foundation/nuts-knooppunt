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
// The localRefMap a map of references of remote Admin Directories (e.g. "Organization/123") to local references.
// We don't want to copy the resource ID from remote mCSD Directory, as we can't guarantee IDs from external directories are unique.
// This means, we let our local mCSD Directory assign new IDs to resources, but we have to make sure that updates are applied to the right local resources.
func buildUpdateTransaction(tx *fhir.Bundle, entry fhir.BundleEntry, allowedResourceTypes []string, isDiscoverableDirectory bool, remoteRefToLocalRefMap map[string]string) (string, error) {
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
			doSync = coding.EqualsCode(endpoint.ConnectionType, coding.MCSDConnectionTypeSystem, coding.MCSDConnectionTypeDirectoryCode)
		}
	}
	if !doSync {
		return resourceType, nil
	}

	updateResourceMeta(resource, *entry.FullUrl)
	// Get or create local reference
	// TODO: If the resource already exists, we should look up the existing resource's ID
	// TODO: We should scope resource IDs to the source (e.g. by prefixing with the source URL or a hash thereof), because when syncing from multiple sources, IDs may collide.
	remoteLocalRef := resourceType + "/" + resource["id"].(string)
	localResourceID := remoteRefToLocalRefMap[remoteLocalRef]
	if localResourceID == "" {
		localResourceID = generateLocalID()
		remoteRefToLocalRefMap[remoteLocalRef] = localResourceID
	}
	resource["id"] = localResourceID
	if err := normalizeReferences(resource, remoteRefToLocalRefMap); err != nil {
		return "", fmt.Errorf("failed to normalize references: %w", err)
	}

	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return "", err
	}
	tx.Entry = append(tx.Entry, fhir.BundleEntry{
		Resource: resourceJSON,
		Request: &fhir.BundleEntryRequest{
			Url:    resourceType,
			Method: entry.Request.Method,
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
				// Doesn't exist yet, create a new local reference
				// TODO: When incremental updating, we should look up if the resource already exists and use that ID instead of generating a new one
				localRef = generateLocalID()
				remoteRefToLocalRefMap[ref] = localRef
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
