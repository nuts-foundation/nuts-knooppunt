package mcsd

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// buildUpdateTransaction constructs a FHIR Bundle transaction for updating resources.
// It filters entries based on allowed resource types and sets the source in the resource meta.
// The function takes a context, a Bundle to populate, a slice of Bundle entries, a map of local references,
// and a slice of allowed resource types.
//
// TODO: The localRefMap is used to map local references to their full URLs, which is used for correlating resources in the transaction.
// We don't want to copy the resource ID from remote mCSD Directory, as we can't guarantee IDs from external directories are unique.
// This means, we let our local mCSD Directory assign new IDs to resources, but we have to make sure that updates are applied to the right local resources.
func buildUpdateTransaction(ctx context.Context, tx *fhir.Bundle, entries []fhir.BundleEntry, allowedResourceTypes []string) ([]string, error) {
	var remoteRefToLocalRefMap = make(map[string]string) // map of references of remote Admin Directories (e.g. "Organization/123") to local references.
	var warnings []string
	for i, entry := range entries {
		if entry.Resource == nil {
			warnings = append(warnings, fmt.Sprintf("Skipping entry #%d: missing 'resource' field", i))
			continue
		}
		if entry.FullUrl == nil {
			warnings = append(warnings, fmt.Sprintf("Skipping entry #%d: missing 'fullUrl' field", i))
			continue
		}
		if entry.Request == nil {
			warnings = append(warnings, fmt.Sprintf("Skipping entry #%d: missing 'request' field", i))
			continue
		}

		resource := make(map[string]any)
		if err := json.Unmarshal(entry.Resource, &resource); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resource in entry #%d (fullUrl=%v): %w", i, entry.FullUrl, err)
		}
		resourceType, ok := resource["resourceType"].(string)
		if !ok {
			return nil, fmt.Errorf("entry #%d does not contain a valid resourceType (fullUrl=%v)", i, entry.FullUrl)
		}
		if !slices.Contains(allowedResourceTypes, resourceType) {
			warnings = append(warnings, fmt.Sprintf("Skipping entry #%d: resource type %s not allowed", i, resourceType))
			continue
		}
		log.Ctx(ctx).Debug().Msgf("Adding entry #%d to transaction: %s", i, resourceType)

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
			return nil, fmt.Errorf("failed to normalize references in entry #%d: %w", i, err)
		}

		resourceJSON, err := json.Marshal(resource)
		if err != nil {
			return nil, err
		}
		tx.Entry = append(tx.Entry, fhir.BundleEntry{
			Resource: resourceJSON,
			Request: &fhir.BundleEntryRequest{
				Url:    resourceType,
				Method: entry.Request.Method,
			},
		})
	}
	return warnings, nil
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
