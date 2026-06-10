package lrza

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const testSourceBaseURL = "http://source.example/fhir"

func sourceQuery(t *testing.T, parts ...string) string {
	t.Helper()
	sourceURL, err := libfhir.BuildSourceURL(testSourceBaseURL, parts...)
	require.NoError(t, err)
	return "_source=" + url.QueryEscape(sourceURL)
}

func TestBuildTransaction(t *testing.T) {
	c := &Component{config: Config{LRZABaseUrl: testSourceBaseURL}}

	t.Run("CREATE/UPDATE becomes a conditional PUT keyed by _source", func(t *testing.T) {
		resource, err := json.Marshal(map[string]any{
			"resourceType": "Organization",
			"id":           "123",
			"meta":         map[string]any{"versionId": "3", "lastUpdated": "2026-01-01T00:00:00Z"},
			"partOf":       map[string]any{"reference": "Organization/456"},
		})
		require.NoError(t, err)

		run := &syncRun{entries: []fhir.BundleEntry{{
			Resource: resource,
			Request:  &fhir.BundleEntryRequest{Method: fhir.HTTPVerbPUT, Url: "Organization/123/_history/3"},
		}}}
		c.buildTransaction(context.Background(), run)

		require.Len(t, run.tx.Entry, 1)
		require.Empty(t, run.report.Warnings)
		txEntry := run.tx.Entry[0]
		require.Equal(t, fhir.HTTPVerbPUT, txEntry.Request.Method)
		require.Equal(t, "Organization?"+sourceQuery(t, "Organization", "123"), txEntry.Request.Url)

		var got map[string]any
		require.NoError(t, json.Unmarshal(txEntry.Resource, &got))
		require.NotContains(t, got, "id", "the source id must be stripped")

		meta := got["meta"].(map[string]any)
		sourceURL, _ := libfhir.BuildSourceURL(testSourceBaseURL, "Organization", "123")
		require.Equal(t, sourceURL, meta["source"])
		require.NotContains(t, meta, "versionId")
		require.NotContains(t, meta, "lastUpdated")

		partOf := got["partOf"].(map[string]any)
		require.Equal(t, "Organization?"+sourceQuery(t, "Organization/456"), partOf["reference"], "references must be rewritten to conditional _source references")
	})

	t.Run("DELETE becomes a conditional DELETE keyed by _source", func(t *testing.T) {
		run := &syncRun{entries: []fhir.BundleEntry{{
			Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbDELETE, Url: "Organization/789/_history/2"},
		}}}
		c.buildTransaction(context.Background(), run)

		require.Len(t, run.tx.Entry, 1)
		require.Empty(t, run.report.Warnings)
		txEntry := run.tx.Entry[0]
		require.Equal(t, fhir.HTTPVerbDELETE, txEntry.Request.Method)
		require.Equal(t, "Organization?"+sourceQuery(t, "Organization", "789"), txEntry.Request.Url)
		require.Nil(t, txEntry.Resource)
	})

	t.Run("unprocessable entries become warnings, not transaction entries", func(t *testing.T) {
		run := &syncRun{entries: []fhir.BundleEntry{
			{Request: nil}, // missing request
			{Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbPUT}, Resource: nil}, // missing body
		}}
		c.buildTransaction(context.Background(), run)

		require.Empty(t, run.tx.Entry)
		require.Len(t, run.report.Warnings, 2)
	})
}

func TestTallyTransactionResult(t *testing.T) {
	// put/del build a request entry and its corresponding response entry (same index, as a FHIR
	// transaction response preserves request order).
	put := func(status string) (fhir.BundleEntry, fhir.BundleEntry) {
		req := fhir.BundleEntry{Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbPUT, Url: "Organization?_source=x"}}
		resp := fhir.BundleEntry{Response: &fhir.BundleEntryResponse{Status: status}}
		return req, resp
	}
	del := func(status string) (fhir.BundleEntry, fhir.BundleEntry) {
		req := fhir.BundleEntry{Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbDELETE, Url: "Organization?_source=x"}}
		resp := fhir.BundleEntry{Response: &fhir.BundleEntryResponse{Status: status}}
		return req, resp
	}

	build := func(pairs ...[2]fhir.BundleEntry) *syncRun {
		run := &syncRun{}
		for _, p := range pairs {
			run.tx.Entry = append(run.tx.Entry, p[0])
		}
		var result fhir.Bundle
		for _, p := range pairs {
			result.Entry = append(result.Entry, p[1])
		}
		run.tallyTransactionResult(result)
		return run
	}
	pair := func(req, resp fhir.BundleEntry) [2]fhir.BundleEntry { return [2]fhir.BundleEntry{req, resp} }

	t.Run("a DELETE returning 200 counts as a delete, not an update", func(t *testing.T) {
		// Regression: the FHIR spec lets a DELETE return 200 (with a payload) for the same result as
		// 204, so classifying by status alone would miscount this delete as an update.
		req, resp := del("200 OK")
		run := build(pair(req, resp))
		require.Equal(t, 1, run.report.CountDeleted)
		require.Equal(t, 0, run.report.CountUpdated)
		require.Empty(t, run.report.Warnings)
	})

	t.Run("DELETE returning 204 counts as a delete", func(t *testing.T) {
		req, resp := del("204 No Content")
		run := build(pair(req, resp))
		require.Equal(t, 1, run.report.CountDeleted)
	})

	t.Run("PUT distinguishes created (201) from updated (200)", func(t *testing.T) {
		c1, r1 := put("201 Created")
		c2, r2 := put("200 OK")
		run := build(pair(c1, r1), pair(c2, r2))
		require.Equal(t, 1, run.report.CountCreated)
		require.Equal(t, 1, run.report.CountUpdated)
		require.Equal(t, 0, run.report.CountDeleted)
	})

	t.Run("empty query directory: creates plus no-op deletes, never updates", func(t *testing.T) {
		// Mirrors the reported scenario: fresh directory, source feed has creates and deletions.
		create1, cresp1 := put("201 Created")
		create2, cresp2 := put("201 Created")
		noop1, dresp1 := del("200 OK") // delete with OperationOutcome payload
		noop2, dresp2 := del("204 No Content")
		run := build(pair(create1, cresp1), pair(create2, cresp2), pair(noop1, dresp1), pair(noop2, dresp2))
		require.Equal(t, 2, run.report.CountCreated)
		require.Equal(t, 0, run.report.CountUpdated, "no updates should be reported on an empty directory")
		require.Equal(t, 2, run.report.CountDeleted)
	})

	t.Run("unexpected status is warned, not counted", func(t *testing.T) {
		req, resp := put("409 Conflict")
		run := build(pair(req, resp))
		require.Equal(t, 0, run.report.CountCreated+run.report.CountUpdated+run.report.CountDeleted)
		require.Len(t, run.report.Warnings, 1)
	})
}
