package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/lrza"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictAPIServer_LrzaRegistration(t *testing.T) {
	newLRZA := func(t *testing.T) *lrza.Component {
		t.Helper()
		config := lrza.DefaultConfig()
		config.LRZABaseUrl = "http://lrza.example/fhir"
		component, err := lrza.New(config)
		require.NoError(t, err)
		return component
	}
	post := func(t *testing.T, server *strictAPIServer) *httptest.ResponseRecorder {
		t.Helper()
		mux := http.NewServeMux()
		server.RegisterHttpHandlers(http.NewServeMux(), mux)
		request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/lrza/fhir/OrganizationAffiliation", strings.NewReader(`{"resourceType":"OrganizationAffiliation"}`))
		request.Header.Set("X-Tenant-ID", "http://fhir.nl/fhir/NamingSystem/ura|00000020")
		request.Header.Set("Content-Type", "application/fhir+json")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}

	t.Run("404 when the LRZA component is disabled", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, post(t, &strictAPIServer{}).Code)
	})

	t.Run("served when the LRZA component is enabled", func(t *testing.T) {
		recorder := post(t, &strictAPIServer{lrza: newLRZA(t)})
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, "application/fhir+json", recorder.Header().Get("Content-Type"))
	})

	t.Run("an encoded path in the id is rejected before the LRZA is contacted", func(t *testing.T) {
		mux := http.NewServeMux()
		(&strictAPIServer{lrza: newLRZA(t)}).RegisterHttpHandlers(http.NewServeMux(), mux)
		// The router decodes %2F, so {id} would be "../OrganizationAffiliation/1".
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/lrza/fhir/Endpoint/..%2FOrganizationAffiliation%2F1", nil)
		request.Header.Set("X-Tenant-ID", "http://fhir.nl/fhir/NamingSystem/ura|00000020")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "invalid resource id")
	})

	t.Run("a missing tenant header is a 400 OperationOutcome", func(t *testing.T) {
		mux := http.NewServeMux()
		(&strictAPIServer{lrza: newLRZA(t)}).RegisterHttpHandlers(http.NewServeMux(), mux)
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/lrza/fhir/Endpoint/1", nil)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "OperationOutcome")
	})
}

// The generated binders' error messages may quote the request's values; none of them may end up
// in the response.
func TestStrictAPIServer_ErrorResponsesDoNotEchoRequestValues(t *testing.T) {
	const bsn = "900186021"
	serve := func(t *testing.T, request *http.Request) *httptest.ResponseRecorder {
		t.Helper()
		mux := http.NewServeMux()
		(&strictAPIServer{}).RegisterHttpHandlers(http.NewServeMux(), mux)
		request.Header.Set("X-Tenant-ID", "http://fhir.nl/fhir/NamingSystem/ura|00000020")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}

	t.Run("invalid query parameter", func(t *testing.T) {
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/nvi/List?_count="+bsn+"x", nil)
		recorder := serve(t, request)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Invalid format for parameter _count")
		assert.NotContains(t, recorder.Body.String(), bsn)
	})
	t.Run("invalid request body", func(t *testing.T) {
		// A non-integer number for an int field: encoding/json quotes the literal in its error.
		request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/nvi", strings.NewReader(`{"resourceType":"Bundle","type":"transaction","total":`+bsn+`.5}`))
		request.Header.Set("Content-Type", "application/fhir+json")
		recorder := serve(t, request)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "OperationOutcome")
		assert.Contains(t, recorder.Body.String(), `Invalid request body: JSON field \"total\" has the wrong type, expected int`)
		assert.NotContains(t, recorder.Body.String(), bsn)
	})
}

func TestDescribeDecodeError(t *testing.T) {
	// An out-of-range number is quoted in the error.
	var target struct {
		Value int16 `json:"value"`
	}
	err := json.Unmarshal([]byte(`{"value":900186021}`), &target)
	require.Error(t, err)
	require.Contains(t, err.Error(), "900186021")
	description := describeDecodeError(fmt.Errorf("can't decode JSON body: %w", err))
	assert.NotContains(t, description, "900186021")
	assert.Equal(t, `JSON field "value" has the wrong type, expected int16`, description)

	assert.Equal(t, "body could not be decoded", describeDecodeError(fmt.Errorf("can't decode formdata: %w", errors.New("form value 900186021"))))
}
