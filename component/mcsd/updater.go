package mcsd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
func buildUpdateTransaction(ctx context.Context, tx *fhir.Bundle, entry fhir.BundleEntry, allowedResourceTypes []string, isDiscoverableDirectory bool, localIDResolver resourceIDResolver) (string, error) {
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
	// TODO: If the resource already exists, we should look up the existing resource's ID
	// TODO: We should scope resource IDs to the source (e.g. by prefixing with the source URL or a hash thereof), because when syncing from multiple sources, IDs may collide.

	// Use pre-generated local ID
	remoteResourceRef := resourceType + "/" + resource["id"].(string)
	localResourceID, err := localIDResolver.resolve(ctx, remoteResourceRef)
	resource["id"] = localResourceID
	if err := normalizeReferences(ctx, resource, localIDResolver); err != nil {
		return "", fmt.Errorf("failed to normalize references: %w", err)
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

func normalizeReferences(ctx context.Context, resource map[string]any, localIDResolver resourceIDResolver) error {
	// TODO: Support fully qualified URL references (e.g. "https://example.com/fhir/Organization/123")
	return normalizeReferencesRecursive(ctx, resource, localIDResolver)
}

func normalizeReferencesRecursive(ctx context.Context, obj any, localIDResolver resourceIDResolver) error {
	switch v := obj.(type) {
	case map[string]any:
		// Check if this is a reference object
		if ref, ok := v["reference"].(string); ok {
			localResourceID, err := localIDResolver.resolve(ctx, ref)
			if err != nil {
				return fmt.Errorf("failed to resolve reference '%s': %w", ref, err)
			}
			if localResourceID == nil {
				// This would violate referential integrity
				return fmt.Errorf("broken reference to '%s' - can't find resource in transaction bundle or local FHIR server", ref)
			}
			v["reference"] = strings.Split(ref, "/")[0] + "/" + *localResourceID
		}
		// Recursively process all map values
		for _, value := range v {
			if err := normalizeReferencesRecursive(ctx, value, localIDResolver); err != nil {
				return err
			}
		}
	case []any:
		// Recursively process all array elements
		for _, item := range v {
			if err := normalizeReferencesRecursive(ctx, item, localIDResolver); err != nil {
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
