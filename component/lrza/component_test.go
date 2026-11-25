package lrza

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/httpauth"
	"github.com/nuts-foundation/nuts-knooppunt/lib/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const testSourceBaseURL = "http://source.example/fhir"

// localRef is the query directory's reference to the copy of a source resource.
func localRef(t *testing.T, resourceType, resourceID string) string {
	t.Helper()
	_, id, err := localID(testSourceBaseURL, resourceType, resourceID)
	require.NoError(t, err)
	return resourceType + "/" + id
}

func TestLocalID(t *testing.T) {
	sourceURL, id, err := localID(testSourceBaseURL, "Organization", "123")
	require.NoError(t, err)
	require.Equal(t, "http://source.example/fhir/Organization/123", sourceURL)
	require.NotEqual(t, "123", id, "the source id must not be reused: it may already exist in another partition of the same server")

	_, again, err := localID(testSourceBaseURL, "Organization", "123")
	require.NoError(t, err)
	require.Equal(t, id, again, "the id must be deterministic, so writes are idempotent and references need no lookup")

	_, otherSource, err := localID("http://other.example/fhir", "Organization", "123")
	require.NoError(t, err)
	require.NotEqual(t, id, otherSource, "the id must be namespaced by source")
}

func TestBuildTransaction(t *testing.T) {
	c := &Component{config: Config{LRZABaseUrl: testSourceBaseURL}}

	t.Run("CREATE/UPDATE becomes a PUT by the derived id", func(t *testing.T) {
		resource, err := json.Marshal(map[string]any{
			"resourceType": "Organization",
			"id":           "123",
			"meta":         map[string]any{"versionId": "3", "lastUpdated": "2026-01-01T00:00:00Z"},
			"partOf":       map[string]any{"reference": "Organization/456"},
		})
		require.NoError(t, err)

		run := &syncRun{}
		c.buildTransaction(context.Background(), run, []fhir.BundleEntry{{
			Resource: resource,
			Request:  &fhir.BundleEntryRequest{Method: fhir.HTTPVerbPUT, Url: "Organization/123/_history/3"},
		}})

		require.Len(t, run.tx.Entry, 1)
		require.Empty(t, run.report.Warnings)
		txEntry := run.tx.Entry[0]
		require.Equal(t, fhir.HTTPVerbPUT, txEntry.Request.Method)
		require.Equal(t, localRef(t, "Organization", "123"), txEntry.Request.Url)

		var got map[string]any
		require.NoError(t, json.Unmarshal(txEntry.Resource, &got))
		require.Equal(t, localRef(t, "Organization", "123"), "Organization/"+got["id"].(string))

		meta := got["meta"].(map[string]any)
		require.Equal(t, "http://source.example/fhir/Organization/123", meta["source"])
		require.NotContains(t, meta, "versionId")
		require.NotContains(t, meta, "lastUpdated")

		partOf := got["partOf"].(map[string]any)
		require.Equal(t, localRef(t, "Organization", "456"), partOf["reference"], "references must be rewritten to the target's derived id")
	})

	t.Run("a reference to a resource not part of this sync is accepted, not rejected", func(t *testing.T) {
		// Per https://minvws.github.io/generiekefuncties-docs/en/csd.html, a Local Replica SHALL
		// accept resources whose references cannot (yet) be resolved - e.g. Organization and Endpoint
		// reference each other, so neither can always be synced before the other. appendTransactionEntry
		// only rewrites the reference to the target's derived id; it never checks whether the target
		// exists, here or anywhere else.
		resource, err := json.Marshal(map[string]any{
			"resourceType": "Organization",
			"id":           "123",
			"endpoint":     []map[string]any{{"reference": "Endpoint/not-yet-synced"}},
		})
		require.NoError(t, err)

		run := &syncRun{}
		c.buildTransaction(context.Background(), run, []fhir.BundleEntry{{
			Resource: resource,
			Request:  &fhir.BundleEntryRequest{Method: fhir.HTTPVerbPUT, Url: "Organization/123/_history/1"},
		}})

		require.Len(t, run.tx.Entry, 1)
		require.Empty(t, run.report.Warnings, "an unresolved reference must not be treated as an error")

		var got map[string]any
		require.NoError(t, json.Unmarshal(run.tx.Entry[0].Resource, &got))
		endpoints, ok := got["endpoint"].([]any)
		require.True(t, ok)
		require.Len(t, endpoints, 1)
		endpointRef := endpoints[0].(map[string]any)
		require.Equal(t, localRef(t, "Endpoint", "not-yet-synced"), endpointRef["reference"])
	})

	t.Run("DELETE becomes a DELETE by the derived id", func(t *testing.T) {
		run := &syncRun{}
		c.buildTransaction(context.Background(), run, []fhir.BundleEntry{{
			Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbDELETE, Url: "Organization/789/_history/2"},
		}})

		require.Len(t, run.tx.Entry, 1)
		require.Empty(t, run.report.Warnings)
		txEntry := run.tx.Entry[0]
		require.Equal(t, fhir.HTTPVerbDELETE, txEntry.Request.Method)
		require.Equal(t, localRef(t, "Organization", "789"), txEntry.Request.Url)
		require.Nil(t, txEntry.Resource)
	})

	t.Run("a full-search entry (no request) becomes a PUT", func(t *testing.T) {
		// Initial full-sync results are plain searchset entries: a resource body with no request.
		resource, err := json.Marshal(map[string]any{"resourceType": "Organization", "id": "123"})
		require.NoError(t, err)

		run := &syncRun{}
		c.buildTransaction(context.Background(), run, []fhir.BundleEntry{{
			FullUrl:  to.Ptr("http://source.example/fhir/Organization/123"),
			Resource: resource,
		}})

		require.Len(t, run.tx.Entry, 1)
		require.Empty(t, run.report.Warnings)
		txEntry := run.tx.Entry[0]
		require.Equal(t, fhir.HTTPVerbPUT, txEntry.Request.Method)
		require.Equal(t, localRef(t, "Organization", "123"), txEntry.Request.Url)
		require.NotNil(t, txEntry.Resource)
	})

	t.Run("unprocessable entries become warnings, not transaction entries", func(t *testing.T) {
		run := &syncRun{}
		c.buildTransaction(context.Background(), run, []fhir.BundleEntry{
			{Request: nil}, // missing request
			{Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbPUT}, Resource: nil}, // missing body
		})

		require.Empty(t, run.tx.Entry)
		require.Len(t, run.report.Warnings, 2)
	})
}

func TestTallyTransactionResult(t *testing.T) {
	// put/del build a request entry and its corresponding response entry (same index, as a FHIR
	// transaction response preserves request order).
	put := func(status string) (fhir.BundleEntry, fhir.BundleEntry) {
		req := fhir.BundleEntry{Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbPUT, Url: "Organization/x"}}
		resp := fhir.BundleEntry{Response: &fhir.BundleEntryResponse{Status: status}}
		return req, resp
	}
	del := func(status string) (fhir.BundleEntry, fhir.BundleEntry) {
		req := fhir.BundleEntry{Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbDELETE, Url: "Organization/x"}}
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

// newHangingServer returns a server that accepts requests and never answers them, until the test ends or
// the client gives up on the request.
func newHangingServer(t *testing.T) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	// Cleanups run last-in first-out: release the handlers before Close waits for them.
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	return server
}

// setDuration sets *target to value for the duration of the test.
func setDuration(t *testing.T, target *time.Duration, value time.Duration) {
	t.Helper()
	original := *target
	*target = value
	t.Cleanup(func() { *target = original })
}

func TestNewTransport(t *testing.T) {
	t.Run("limits the connections to one host", func(t *testing.T) {
		assert.Equal(t, maxConnsPerHost, newTransport(nil).MaxConnsPerHost)
	})
}

func TestNew_ClientsAreBounded(t *testing.T) {
	source, err := newSourceHTTPClient(Config{LRZABaseUrl: "http://lrza.example/fhir"})
	require.NoError(t, err)
	// x/oauth2 builds its own client, which takes its limits from the token client in NewOAuth2HTTPClient.
	sourceWithOAuth2, err := newSourceHTTPClient(Config{LRZABaseUrl: "http://lrza.example/fhir", Auth: httpauth.OAuth2Config{
		TokenEndpoint: "http://auth.example/token", ClientID: "client", ClientSecret: "secret",
	}})
	require.NoError(t, err)

	for name, client := range map[string]*http.Client{"source": source, "source with OAuth2": sourceWithOAuth2, "query": newQueryHTTPClient()} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, httpclient.DefaultRequestTimeout, client.Timeout)
			assert.NotNil(t, client.CheckRedirect)
		})
	}
}

func TestUpdate_Bounded(t *testing.T) {
	t.Run("waiting for a running sync ends with the caller's context", func(t *testing.T) {
		c, err := New(Config{LRZABaseUrl: "http://lrza.example/fhir", QueryBaseUrl: "http://query.example/fhir"})
		require.NoError(t, err)
		require.NoError(t, c.syncLock.Acquire(context.Background(), 1)) // a sync is running

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err = c.update(ctx)

		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.ErrorContains(t, err, "waiting for a running LRZA sync")
	})

	t.Run("a sync that stops making progress ends after pageTimeout and frees the lock", func(t *testing.T) {
		setDuration(t, &pageTimeout, 100*time.Millisecond)
		source := newHangingServer(t)
		c, err := New(Config{LRZABaseUrl: source.URL, QueryBaseUrl: "http://query.example/fhir"})
		require.NoError(t, err)

		_, err = c.update(context.Background())

		require.ErrorIs(t, err, errPageTimeout)
		assert.True(t, c.syncLock.TryAcquire(1), "the next sync must be able to run")
	})

	t.Run("a slow sync that keeps making progress is not cut off by pageTimeout", func(t *testing.T) {
		// Every page takes 30 ms and there are 14 of them (10 Organization pages, plus one empty page for
		// each other type), so the sync as a whole takes several times pageTimeout.
		setDuration(t, &pageTimeout, 200*time.Millisecond)
		const organizationPages = 10
		var source *httptest.Server
		source = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(30 * time.Millisecond)
			bundle := map[string]any{"resourceType": "Bundle", "type": "searchset"}
			if r.URL.Path == "/Organization" {
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				bundle["entry"] = []any{map[string]any{"resource": map[string]any{"resourceType": "Organization", "id": fmt.Sprintf("org-%d", page)}}}
				if page+1 < organizationPages {
					bundle["link"] = []any{map[string]any{"relation": "next", "url": fmt.Sprintf("%s/Organization?page=%d", source.URL, page+1)}}
				}
			}
			writeJSON(w, http.StatusOK, bundle)
		}))
		t.Cleanup(source.Close)
		query := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"resourceType": "Bundle", "type": "transaction-response",
				"entry": []any{map[string]any{"response": map[string]any{"status": "201 Created"}}}})
		}))
		t.Cleanup(query.Close)
		c, err := New(Config{LRZABaseUrl: source.URL, QueryBaseUrl: query.URL})
		require.NoError(t, err)

		start := time.Now()
		report, err := c.update(context.Background())

		require.NoError(t, err)
		assert.Equal(t, organizationPages, report.CountCreated)
		assert.Greater(t, time.Since(start), 2*pageTimeout, "the test must outlast pageTimeout to show anything")
	})

	t.Run("a sync that never ends stops after syncTimeout and frees the lock", func(t *testing.T) {
		setDuration(t, &syncTimeout, 100*time.Millisecond)
		source := newHangingServer(t)
		c, err := New(Config{LRZABaseUrl: source.URL, QueryBaseUrl: "http://query.example/fhir"})
		require.NoError(t, err)

		_, err = c.update(context.Background())

		require.ErrorIs(t, err, errSyncTimeout)
		assert.True(t, c.syncLock.TryAcquire(1), "the next sync must be able to run")
	})
}
