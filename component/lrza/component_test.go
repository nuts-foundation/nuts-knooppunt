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
	c := &Component{config: Config{FHIRBaseURL: testSourceBaseURL}}

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

func TestTypeAndIDFromURL(t *testing.T) {
	tests := []struct {
		name             string
		url              string
		wantType, wantID string
		wantOK           bool
	}{
		{name: "bare type/id", url: "Organization/123", wantType: "Organization", wantID: "123", wantOK: true},
		{name: "versioned history url", url: "Organization/123/_history/2", wantType: "Organization", wantID: "123", wantOK: true},
		{name: "single segment", url: "Organization", wantOK: false},
		{name: "empty", url: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotID, gotOK := typeAndIDFromURL(tt.url)
			require.Equal(t, tt.wantOK, gotOK)
			require.Equal(t, tt.wantType, gotType)
			require.Equal(t, tt.wantID, gotID)
		})
	}
}
