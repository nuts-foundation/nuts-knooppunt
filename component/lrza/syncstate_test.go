package lrza

import (
	"net/http"
	"testing"

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
		return extension["valueString"].(string)
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

		require.NotEmpty(t, c.lastUpdateTime)
		assert.Equal(t, c.lastUpdateTime, storedSyncTime(t, c, query))
	})
	t.Run("after a restart, the first sync is incremental from the saved time", func(t *testing.T) {
		source, query := newFakeLRZA(t, testDirectory()...), newFakeLRZA(t)
		syncOK(t, newComponent(t, source, query))
		require.Equal(t, 1, fullReads(source))

		restarted := newComponent(t, source, query)
		syncOK(t, restarted)

		assert.Equal(t, 1, fullReads(source), "no second full sync")
		assert.Equal(t, 1, historyReads(source))
	})
	t.Run("a failed sync keeps the last successful time, also across a restart", func(t *testing.T) {
		source, query := newFakeLRZA(t, testDirectory()...), newFakeLRZA(t)
		c := newComponent(t, source, query)
		syncOK(t, c)
		lastSuccess := c.lastUpdateTime

		// The query directory rejects the first page's transaction.
		query.failNext(http.StatusInternalServerError)
		_, err := c.update(t.Context())
		require.Error(t, err)

		assert.Equal(t, lastSuccess, c.lastUpdateTime)
		assert.Equal(t, lastSuccess, storedSyncTime(t, c, query))
		restarted := newComponent(t, source, query)
		require.NoError(t, restarted.loadSyncState(t.Context()))
		assert.Equal(t, lastSuccess, restarted.lastUpdateTime)
	})
	t.Run("an empty query directory means a full sync", func(t *testing.T) {
		source, query := newFakeLRZA(t, testDirectory()...), newFakeLRZA(t)
		c := newComponent(t, source, query)

		require.NoError(t, c.loadSyncState(t.Context()))

		assert.Empty(t, c.lastUpdateTime)
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
