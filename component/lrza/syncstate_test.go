package lrza

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncState(t *testing.T) {
	// newComponent creates a component syncing source into query, as after a (re)start: nothing in memory.
	newComponent := func(t *testing.T, source, query *fakeLRZA) *Component {
		return newTestComponent(t, source, func(config *Config) {
			config.QueryBaseUrl = query.server.URL
		})
	}
	syncOK := func(t *testing.T, c *Component) {
		t.Helper()
		_, err := c.update(t.Context())
		require.NoError(t, err)
	}
	storedSyncTime := func(t *testing.T, c *Component, query *fakeLRZA) string {
		t.Helper()
		id, err := c.syncStateID()
		require.NoError(t, err)
		stored := query.get("Basic/" + id)
		require.NotNil(t, stored, "no sync state in the query directory")
		extensions, _ := stored["extension"].([]any)
		require.Len(t, extensions, 1)
		extension := extensions[0].(map[string]any)
		assert.Equal(t, syncStateExtensionURL, extension["url"])
		return extension["valueInstant"].(string)
	}
	// fullReads and historyReads count the full searches and incremental _history reads of the LRZA.
	fullReads := func(source *fakeLRZA) int {
		return len(source.requestsTo(http.MethodGet, "/Organization"))
	}
	historyReads := func(source *fakeLRZA) int {
		return len(source.requestsTo(http.MethodGet, "/Organization/_history"))
	}

	t.Run("a successful sync saves its time", func(t *testing.T) {
		source, query := newFakeLRZA(t, testDirectory()...), newFakeLRZA(t)
		c := newComponent(t, source, query)

		syncOK(t, c)

		assert.NotEmpty(t, storedSyncTime(t, c, query))
	})
	t.Run("after a restart, the first sync is incremental from the saved time", func(t *testing.T) {
		source, query := newFakeLRZA(t, testDirectory()...), newFakeLRZA(t)
		first := newComponent(t, source, query)
		syncOK(t, first)
		require.Equal(t, 1, fullReads(source))
		saved := storedSyncTime(t, first, query)

		restarted := newComponent(t, source, query)
		syncOK(t, restarted)

		assert.Equal(t, 1, fullReads(source), "no second full sync")
		require.Equal(t, 1, historyReads(source))
		since := source.requestsTo(http.MethodGet, "/Organization/_history")[0].query.Get("_since")
		assert.Equal(t, saved, since)
	})
	t.Run("a failed sync keeps the last successful time, also across a restart", func(t *testing.T) {
		source, query := newFakeLRZA(t, testDirectory()...), newFakeLRZA(t)
		c := newComponent(t, source, query)
		syncOK(t, c)
		lastSuccess := storedSyncTime(t, c, query)

		// The query directory lets the sync time be read, then rejects the first page's transaction.
		query.failNext(0, http.StatusInternalServerError)
		_, err := c.update(t.Context())
		require.Error(t, err)

		assert.Equal(t, lastSuccess, storedSyncTime(t, c, query))
		restarted := newComponent(t, source, query)
		since, err := restarted.loadSyncTime(t.Context())
		require.NoError(t, err)
		assert.Equal(t, lastSuccess, since.Format(time.RFC3339Nano))
	})
	t.Run("failing to save the time fails the sync", func(t *testing.T) {
		// One Organization, so the query directory sees: read the time, one transaction, save the time.
		source, query := newFakeLRZA(t, testDirectory()[0]), newFakeLRZA(t)
		c := newComponent(t, source, query)
		query.failNext(0, 0, http.StatusInternalServerError)

		report, err := c.update(t.Context())

		require.ErrorContains(t, err, "failed to write LRZA sync time")
		assert.Equal(t, 1, report.CountCreated, "the applied page is still reported")
		id, err := c.syncStateID()
		require.NoError(t, err)
		assert.Nil(t, query.get("Basic/"+id))
	})
	t.Run("a query directory wiped while running means a full sync", func(t *testing.T) {
		source, query := newFakeLRZA(t, testDirectory()...), newFakeLRZA(t)
		c := newComponent(t, source, query)
		syncOK(t, c)
		query.mux.Lock()
		query.resources = map[string]resource{}
		query.mux.Unlock()

		syncOK(t, c)

		assert.Equal(t, 2, fullReads(source))
		assert.Zero(t, historyReads(source))
	})
	t.Run("an invalid saved time fails the sync, instead of a full sync that would miss deletions", func(t *testing.T) {
		source, query := newFakeLRZA(t, testDirectory()...), newFakeLRZA(t)
		c := newComponent(t, source, query)
		syncOK(t, c)
		id, err := c.syncStateID()
		require.NoError(t, err)
		stored := query.get("Basic/" + id)
		stored["extension"] = []any{map[string]any{"url": syncStateExtensionURL, "valueInstant": "not a time"}}
		query.mux.Lock()
		query.put(stored)
		query.mux.Unlock()

		_, err = c.update(t.Context())

		require.ErrorContains(t, err, "invalid LRZA sync time")
		assert.Equal(t, 1, fullReads(source), "no second full sync")
		assert.Zero(t, historyReads(source))
	})
	t.Run("an empty query directory means a full sync", func(t *testing.T) {
		source, query := newFakeLRZA(t, testDirectory()...), newFakeLRZA(t)
		c := newComponent(t, source, query)

		since, err := c.loadSyncTime(t.Context())

		require.NoError(t, err)
		assert.True(t, since.IsZero())
	})
	t.Run("failing to read the saved time fails the sync before reading the LRZA", func(t *testing.T) {
		source, query := newFakeLRZA(t, testDirectory()...), newFakeLRZA(t)
		c := newComponent(t, source, query)
		query.failNext(http.StatusInternalServerError)

		_, err := c.update(t.Context())

		require.Error(t, err)
		assert.Zero(t, fullReads(source)+historyReads(source))
		// The next sync reads the saved time again and goes ahead.
		syncOK(t, c)
		assert.Equal(t, 1, fullReads(source))
	})
}
