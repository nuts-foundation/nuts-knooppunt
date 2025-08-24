package mcsd

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

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

		setResourceMetaSource(resource, "")
		// Get or create local reference
		// TODO: Lookup ID local to local mCSD Directory, not remote
		//localResourceID := localRefMap[*entry.FullUrl]
		//if localResourceID == "" {
		//	localResourceID = fmt.Sprintf("urn:uuid:%s", uuid.NewString())
		//	localRefMap[*entry.FullUrl] = localResourceID
		//}
		//resource["id"] = localResourceID
		resourceJSON, _ := json.Marshal(resource)
		tx.Entry = append(tx.Entry, fhir.BundleEntry{
			Resource: resourceJSON,
			Request: &fhir.BundleEntryRequest{
				Url:    resourceType + "/" + resource["id"].(string),
				Method: entry.Request.Method,
			},
		})
	}
	return warnings, nil
}

func setResourceMetaSource(resource map[string]any, source string) {
	if meta, ok := resource["meta"].(map[string]any); ok {
		meta["source"] = source
	} else {
		resource["meta"] = map[string]any{"source": source}
	}
}
