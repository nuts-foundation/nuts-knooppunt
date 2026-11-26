package lrza

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPagingSource returns a source directory that serves pages Organizations, one per page, each page
// linking to the next. Every other resource type has a single, empty page.
func newPagingSource(t *testing.T, pages int) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bundle := map[string]any{"resourceType": "Bundle", "type": "searchset"}
		if r.URL.Path == "/Organization" {
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			bundle["entry"] = []any{map[string]any{"resource": map[string]any{
				"resourceType": "Organization",
				"id":           fmt.Sprintf("org-%d", page),
			}}}
			if page+1 < pages {
				bundle["link"] = []any{map[string]any{
					"relation": "next",
					"url":      fmt.Sprintf("%s/Organization?page=%d", server.URL, page+1),
				}}
			}
		}
		writeJSON(w, http.StatusOK, bundle)
	}))
	t.Cleanup(server.Close)
	return server
}

// organizations counts the Organizations in a fake query directory.
func organizations(query *fakeLRZA) int {
	query.mux.Lock()
	defer query.mux.Unlock()
	count := 0
	for reference := range query.resources {
		if strings.HasPrefix(reference, "Organization/") {
			count++
		}
	}
	return count
}

func TestUpdate_Pagination(t *testing.T) {
	t.Run("a feed of more than 100 pages is synced completely", func(t *testing.T) {
		// Paginate's default limit of 100 iterations stopped the sync after 99 pages.
		const pages = 150
		query := newFakeLRZA(t)
		c, err := New(Config{
			LRZABaseUrl:  newPagingSource(t, pages).URL,
			QueryBaseUrl: query.server.URL,
		})
		require.NoError(t, err)

		report, err := c.update(context.Background())

		require.NoError(t, err)
		assert.Equal(t, pages, report.CountCreated)
		assert.Equal(t, pages, organizations(query))
		syncedUpTo, err := c.loadSyncedUpTo(context.Background())
		require.NoError(t, err)
		assert.False(t, syncedUpTo.IsZero(), "a completed sync saves the time for the next one")
	})

	t.Run("a feed of exactly maxPagesPerResourceType pages is accepted", func(t *testing.T) {
		setMaxPages(t, 5)
		query := newFakeLRZA(t)
		c, err := New(Config{
			LRZABaseUrl:  newPagingSource(t, 5).URL,
			QueryBaseUrl: query.server.URL,
		})
		require.NoError(t, err)

		_, err = c.update(context.Background())

		require.NoError(t, err)
		assert.Equal(t, 5, organizations(query))
	})

	t.Run("a feed beyond maxPagesPerResourceType fails the sync", func(t *testing.T) {
		setMaxPages(t, 5)
		query := newFakeLRZA(t)
		c, err := New(Config{
			LRZABaseUrl:  newPagingSource(t, 6).URL,
			QueryBaseUrl: query.server.URL,
		})
		require.NoError(t, err)

		_, err = c.update(context.Background())

		require.ErrorContains(t, err, "Organization failed: more than 5 pages, the page cap")
		assert.Equal(t, 5, organizations(query), "pages before the limit are applied")
		syncedUpTo, err := c.loadSyncedUpTo(context.Background())
		require.NoError(t, err)
		assert.True(t, syncedUpTo.IsZero(), "a failed sync must not save a time")
	})
}

// setMaxPages sets maxPagesPerResourceType for the duration of the test.
func setMaxPages(t *testing.T, value int) {
	t.Helper()
	original := maxPagesPerResourceType
	maxPagesPerResourceType = value
	t.Cleanup(func() { maxPagesPerResourceType = original })
}
