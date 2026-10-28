package fhirutil

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestCachingClient(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/Organization/1":
			reads.Add(1)
			w.Header().Set("ETag", `W/"3"`)
			_, _ = w.Write([]byte(`{"resourceType":"Organization","id":"1","name":"Sunflower"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/Organization/missing":
			reads.Add(1)
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"resourceType":"OperationOutcome","issue":[{"severity":"error","code":"not-found"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/Organization/flaky":
			reads.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"resourceType":"Organization","id":"1"}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	baseURL, _ := url.Parse(server.URL)
	newClient := func() *CachingClient {
		reads.Store(0)
		return NewCachingClient(fhirclient.New(baseURL, server.Client(), nil))
	}

	t.Run("a second read is served from the cache, with its headers and status", func(t *testing.T) {
		client := newClient()
		var first fhir.Organization
		require.NoError(t, client.ReadWithContext(t.Context(), "Organization/1", &first))

		var second []byte
		var headers fhirclient.Headers
		var status int
		require.NoError(t, client.ReadWithContext(t.Context(), "Organization/1", &second,
			fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status)))

		assert.Equal(t, int32(1), reads.Load())
		assert.Equal(t, "Sunflower", *first.Name)
		assert.JSONEq(t, `{"resourceType":"Organization","id":"1","name":"Sunflower"}`, string(second))
		assert.Equal(t, `W/"3"`, headers.Get("ETag"))
		assert.Equal(t, http.StatusOK, status)
	})

	t.Run("a not-found is cached too", func(t *testing.T) {
		client := newClient()
		for range 2 {
			var status int
			err := client.ReadWithContext(t.Context(), "Organization/missing", &fhir.Organization{}, fhirclient.ResponseStatusCode(&status))
			require.Error(t, err)
			assert.Equal(t, http.StatusNotFound, status)
		}
		assert.Equal(t, int32(1), reads.Load())
	})

	t.Run("a server error is not cached", func(t *testing.T) {
		client := newClient()
		for range 2 {
			require.Error(t, client.ReadWithContext(t.Context(), "Organization/flaky", &fhir.Organization{}))
		}
		assert.Equal(t, int32(2), reads.Load())
	})

	t.Run("a write empties the cache", func(t *testing.T) {
		client := newClient()
		require.NoError(t, client.ReadWithContext(t.Context(), "Organization/1", &fhir.Organization{}))
		require.NoError(t, client.UpdateWithContext(t.Context(), "Organization/1", fhir.Organization{Id: new("1")}, nil))
		require.NoError(t, client.ReadWithContext(t.Context(), "Organization/1", &fhir.Organization{}))

		assert.Equal(t, int32(2), reads.Load())
	})

	t.Run("a read with a pre-request option bypasses the cache", func(t *testing.T) {
		client := newClient()
		for range 2 {
			require.NoError(t, client.ReadWithContext(t.Context(), "Organization/1", &fhir.Organization{},
				fhirclient.RequestHeaders(http.Header{"X-Example": []string{"1"}})))
		}
		assert.Equal(t, int32(2), reads.Load())
	})
}
