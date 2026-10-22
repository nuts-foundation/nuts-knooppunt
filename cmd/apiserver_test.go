package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/lrza"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictAPIServer_LrzaRegistration(t *testing.T) {
	newLRZA := func(t *testing.T, registration bool) *lrza.Component {
		t.Helper()
		config := lrza.DefaultConfig()
		config.LRZABaseUrl = "http://lrza.example/fhir"
		config.Registration.Enabled = registration
		component, err := lrza.New(config)
		require.NoError(t, err)
		return component
	}
	post := func(t *testing.T, server *strictAPIServer) *httptest.ResponseRecorder {
		t.Helper()
		mux := http.NewServeMux()
		server.RegisterHttpHandlers(http.NewServeMux(), mux)
		// OrganizationAffiliation is rejected before the LRZA is contacted, so no LRZA is needed.
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

	t.Run("404 when registration is disabled", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, post(t, &strictAPIServer{lrza: newLRZA(t, false)}).Code)
	})

	t.Run("served when registration is enabled", func(t *testing.T) {
		recorder := post(t, &strictAPIServer{lrza: newLRZA(t, true)})
		assert.Equal(t, http.StatusForbidden, recorder.Code)
		assert.Equal(t, "application/fhir+json", recorder.Header().Get("Content-Type"))
	})

	t.Run("a missing tenant header is a 400 OperationOutcome", func(t *testing.T) {
		mux := http.NewServeMux()
		(&strictAPIServer{lrza: newLRZA(t, true)}).RegisterHttpHandlers(http.NewServeMux(), mux)
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/lrza/fhir/Endpoint/1", nil)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "OperationOutcome")
	})
}
