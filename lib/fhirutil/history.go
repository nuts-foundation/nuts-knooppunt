package fhirutil

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// DeduplicateHistoryEntries collapses a FHIR _history feed so each resource appears at most once,
// keeping only its most recent version. A _history query can return several versions of the same
// resource (including a DELETE), but a transaction Bundle must reference each resource at most once,
// so the feed must be deduplicated before it can be replayed as a transaction.
//
// FHIR _history entries are full snapshots rather than deltas, so keeping the latest version yields
// the resource's current state; versions are not merged.
//
// The implementation relies on the spec-mandated history ordering: a history Bundle is "sorted with
// oldest versions last" (default _sort is -_lastUpdated), so the first entry seen for a given
// resource is its most recent version. We therefore keep the first occurrence of each resource and
// drop the rest - no timestamp comparison needed - which also makes DELETEs (the newest version,
// hence first) correctly win. Callers querying _history must not override that ordering. Input order
// is preserved in the output.
//
// Resources are keyed by "ResourceType/id", not by bare id: a FHIR logical id is only unique within
// a resource type, so e.g. Organization/1 and Endpoint/1 are distinct resources that must not be
// collapsed into one.
func DeduplicateHistoryEntries(entries []fhir.BundleEntry) []fhir.BundleEntry {
	seen := make(map[string]struct{}, len(entries))
	result := make([]fhir.BundleEntry, 0, len(entries))

	for _, entry := range entries {
		key := historyEntryKey(entry)
		if key == "" {
			// No key to dedup on (e.g. a malformed entry); keep it rather than silently drop it.
			result = append(result, entry)
			continue
		}
		if _, exists := seen[key]; exists {
			// An older version of a resource we've already kept; skip it.
			continue
		}
		seen[key] = struct{}{}
		result = append(result, entry)
	}

	return result
}

// historyEntryKey returns a "ResourceType/id" key identifying the resource an entry refers to: built
// from the resource body for create/update entries, or from the request URL for DELETE entries
// (which carry no body). Returns "" when no key can be determined, in which case the entry can't be
// deduplicated.
func historyEntryKey(entry fhir.BundleEntry) string {
	if entry.Resource == nil {
		if entry.Request != nil && entry.Request.Method == fhir.HTTPVerbDELETE {
			return deleteEntryKey(entry)
		}
		return ""
	}
	info, err := ExtractResourceInfo(entry.Resource)
	if err != nil || info.ResourceType == "" || info.ID == "" {
		return ""
	}
	return info.ResourceType + "/" + info.ID
}

// deleteEntryKey extracts a "ResourceType/id" key from a DELETE entry's request URL (e.g.
// "Organization/123"), falling back to the last two segments of the entry's fullUrl.
func deleteEntryKey(entry fhir.BundleEntry) string {
	if entry.Request != nil {
		if key := typeIDFromReference(entry.Request.Url); key != "" {
			return key
		}
	}
	if entry.FullUrl != nil {
		if key := typeIDFromReference(*entry.FullUrl); key != "" {
			return key
		}
	}
	return ""
}

// typeIDFromReference returns the "ResourceType/id" key for a FHIR reference or URL, or "" when one
// can't be determined. See TypeAndIDFromReference for the forms handled.
func typeIDFromReference(ref string) string {
	resourceType, id, ok := TypeAndIDFromReference(ref)
	if !ok {
		return ""
	}
	return resourceType + "/" + id
}
