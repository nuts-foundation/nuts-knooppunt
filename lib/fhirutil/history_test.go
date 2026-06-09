package fhirutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestTypeIDFromReference(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		expected string
	}{
		{name: "relative reference", ref: "Organization/123", expected: "Organization/123"},
		{name: "absolute reference", ref: "http://example.org/fhir/Organization/abc123", expected: "Organization/abc123"},
		{name: "UUID-format id", ref: "Organization/fd3524f9-705e-453c-8130-71cdf51cfcb9", expected: "Organization/fd3524f9-705e-453c-8130-71cdf51cfcb9"},
		{name: "relative history version url", ref: "Endpoint/123/_history/2", expected: "Endpoint/123"},
		{name: "absolute history version url", ref: "http://example.org/fhir/Endpoint/123/_history/2", expected: "Endpoint/123"},
		{name: "trailing slash is trimmed", ref: "Organization/123/", expected: "Organization/123"},
		{name: "single segment has no type/id", ref: "Organization", expected: ""},
		{name: "empty reference", ref: "", expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, typeIDFromReference(tt.ref))
		})
	}
}

func TestDeleteEntryKey(t *testing.T) {
	tests := []struct {
		name     string
		entry    fhir.BundleEntry
		expected string
	}{
		{
			name: "from Request.Url",
			entry: fhir.BundleEntry{
				Request: &fhir.BundleEntryRequest{Url: "Organization/123"},
			},
			expected: "Organization/123",
		},
		{
			name: "falls back to fullUrl when Request.Url is empty",
			entry: fhir.BundleEntry{
				FullUrl: to.Ptr("http://example.org/fhir/Organization/abc123"),
				Request: &fhir.BundleEntryRequest{Url: ""},
			},
			expected: "Organization/abc123",
		},
		{
			name: "empty when neither yields a type/id",
			entry: fhir.BundleEntry{
				FullUrl: to.Ptr(""),
				Request: &fhir.BundleEntryRequest{Url: "Organization"},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, deleteEntryKey(tt.entry))
		})
	}
}

func TestDeduplicateHistoryEntries(t *testing.T) {
	// orgVersion builds a history entry for Organization/id at a given version, with the resource
	// body carrying the id. Ordering of entries in these tests is newest-first, as the FHIR spec
	// guarantees for _history results.
	orgVersion := func(id, version string) fhir.BundleEntry {
		return fhir.BundleEntry{
			FullUrl:  to.Ptr("http://example.org/fhir/Organization/" + id),
			Resource: []byte(`{"resourceType":"Organization","id":"` + id + `","meta":{"versionId":"` + version + `"}}`),
		}
	}
	// orgDelete mirrors a real _history DELETE entry: no body, and a versioned request URL. The
	// version suffix must still resolve to the same key as the resource versions so the DELETE
	// suppresses them.
	orgDelete := func(id string) fhir.BundleEntry {
		return fhir.BundleEntry{
			FullUrl: to.Ptr("http://example.org/fhir/Organization/" + id),
			Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbDELETE, Url: "Organization/" + id + "/_history/3"},
		}
	}

	t.Run("keeps only the first (newest) version of each resource", func(t *testing.T) {
		entries := []fhir.BundleEntry{
			orgVersion("1", "3"), // newest
			orgVersion("1", "2"),
			orgVersion("1", "1"), // oldest
		}
		result := DeduplicateHistoryEntries(entries)
		require.Len(t, result, 1)
		require.Equal(t, orgVersion("1", "3"), result[0])
	})

	t.Run("a newest DELETE wins over older versions", func(t *testing.T) {
		entries := []fhir.BundleEntry{
			orgDelete("1"), // newest: resource was deleted
			orgVersion("1", "2"),
			orgVersion("1", "1"),
		}
		result := DeduplicateHistoryEntries(entries)
		require.Len(t, result, 1)
		require.Equal(t, orgDelete("1"), result[0])
	})

	t.Run("preserves input order across distinct resources", func(t *testing.T) {
		entries := []fhir.BundleEntry{
			orgVersion("2", "1"),
			orgVersion("1", "2"),
			orgVersion("1", "1"), // older version of org 1, dropped
			orgVersion("3", "1"),
		}
		result := DeduplicateHistoryEntries(entries)
		require.Equal(t, []fhir.BundleEntry{
			orgVersion("2", "1"),
			orgVersion("1", "2"),
			orgVersion("3", "1"),
		}, result)
	})

	t.Run("same id on different resource types are kept separately", func(t *testing.T) {
		// FHIR logical ids are unique only within a resource type, so Organization/1 and
		// Endpoint/1 are distinct and must both survive deduplication.
		endpointV1 := fhir.BundleEntry{
			FullUrl:  to.Ptr("http://example.org/fhir/Endpoint/1"),
			Resource: []byte(`{"resourceType":"Endpoint","id":"1"}`),
		}
		entries := []fhir.BundleEntry{orgVersion("1", "1"), endpointV1}
		result := DeduplicateHistoryEntries(entries)
		require.Equal(t, []fhir.BundleEntry{orgVersion("1", "1"), endpointV1}, result)
	})

	t.Run("keeps entries that have no identifiable resource id", func(t *testing.T) {
		noID := fhir.BundleEntry{Resource: []byte(`{invalid json}`)}
		entries := []fhir.BundleEntry{noID, noID}
		result := DeduplicateHistoryEntries(entries)
		require.Len(t, result, 2)
	})

	t.Run("empty input yields empty output", func(t *testing.T) {
		require.Empty(t, DeduplicateHistoryEntries(nil))
	})
}
