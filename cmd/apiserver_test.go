package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/nuts-foundation/nuts-knooppunt/component/lrza"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
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

func TestResponseErrorHandler(t *testing.T) {
	handle := func(err error) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		responseErrorHandler(recorder, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/nvi/List", nil), err)
		return recorder
	}

	t.Run("an unexpected error is a 500 OperationOutcome without the error's text", func(t *testing.T) {
		recorder := handle(errors.New("connection to db.internal refused"))

		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.Equal(t, "application/fhir+json", recorder.Header().Get("Content-Type"))
		var outcome fhir.OperationOutcome
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &outcome))
		require.Len(t, outcome.Issue, 1)
		assert.Equal(t, fhir.IssueTypeException, outcome.Issue[0].Code)
		assert.Equal(t, "Internal server error", *outcome.Issue[0].Diagnostics)
		assert.NotContains(t, recorder.Body.String(), "db.internal")
	})
	t.Run("a disabled component is a 404", func(t *testing.T) {
		assert.Equal(t, http.StatusNotFound, handle(errComponentDisabled).Code)
	})
}

func TestStrictAPIServer_RequestLimits(t *testing.T) {
	const bodyLimit = 1024
	const tenantID = "http://fhir.nl/fhir/NamingSystem/ura|00000020"
	// The components are left disabled, so a request that passes the Content-Type check and body
	// decoding ends in a 404 (errComponentDisabled) without contacting anything.
	serve := func(t *testing.T, method, path, contentType string, body io.Reader) *httptest.ResponseRecorder {
		t.Helper()
		mux := http.NewServeMux()
		(&strictAPIServer{}).RegisterHttpHandlers(http.NewServeMux(), mux)
		// Stand-in for the body ceiling the HTTP component puts in front of the mux.
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
			mux.ServeHTTP(w, r)
		})
		request := httptest.NewRequestWithContext(context.Background(), method, path, body)
		request.Header.Set("X-Tenant-ID", tenantID)
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	operationOutcome := func(t *testing.T, recorder *httptest.ResponseRecorder) fhir.OperationOutcome {
		t.Helper()
		assert.Equal(t, "application/fhir+json", recorder.Header().Get("Content-Type"))
		var outcome fhir.OperationOutcome
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &outcome))
		require.Len(t, outcome.Issue, 1)
		return outcome
	}
	const bundle = `{"resourceType":"Bundle","type":"transaction"}`

	t.Run("body over the limit is a 413 OperationOutcome", func(t *testing.T) {
		body := `{"resourceType":"Bundle","type":"transaction","id":"` + strings.Repeat("a", bodyLimit) + `"}`
		recorder := serve(t, http.MethodPost, "/lrza/fhir", "application/fhir+json", strings.NewReader(body))
		assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
		outcome := operationOutcome(t, recorder)
		assert.Equal(t, fhir.IssueTypeTooLong, outcome.Issue[0].Code)
		assert.Equal(t, "Request body exceeds 1024 bytes", *outcome.Issue[0].Diagnostics)
	})
	t.Run("form body over the limit is a 413 OperationOutcome", func(t *testing.T) {
		body := "code=" + strings.Repeat("a", bodyLimit)
		recorder := serve(t, http.MethodPost, "/nvi/List/_search", "application/x-www-form-urlencoded", strings.NewReader(body))
		assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
		assert.Equal(t, fhir.IssueTypeTooLong, operationOutcome(t, recorder).Issue[0].Code)
	})
	t.Run("malformed JSON is a 400 OperationOutcome", func(t *testing.T) {
		recorder := serve(t, http.MethodPost, "/lrza/fhir", "application/fhir+json", strings.NewReader(`{"resourceType":`))
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		outcome := operationOutcome(t, recorder)
		assert.Equal(t, fhir.IssueTypeInvalid, outcome.Issue[0].Code)
		assert.True(t, strings.HasPrefix(*outcome.Issue[0].Diagnostics, "Invalid request body: "), *outcome.Issue[0].Diagnostics)
	})
	t.Run("Content-Type", func(t *testing.T) {
		testCases := []struct {
			name        string
			method      string
			path        string
			contentType string
			body        string
			wantCode    int
		}{
			{"text/plain is rejected", http.MethodPost, "/lrza/fhir", "text/plain", bundle, http.StatusUnsupportedMediaType},
			{"missing is rejected", http.MethodPost, "/lrza/fhir", "", bundle, http.StatusUnsupportedMediaType},
			{"unparsable is rejected", http.MethodPost, "/lrza/fhir", "application/fhir+json; charset", bundle, http.StatusUnsupportedMediaType},
			{"application/json passes", http.MethodPost, "/lrza/fhir", "application/json", bundle, http.StatusNotFound},
			{"application/fhir+json with charset passes", http.MethodPost, "/lrza/fhir", "application/fhir+json; charset=utf-8", bundle, http.StatusNotFound},
			{"FHIR JSON on a path parameter route passes", http.MethodPut, "/lrza/fhir/Endpoint/1", "application/fhir+json", `{"resourceType":"Endpoint"}`, http.StatusNotFound},
			{"text/plain on a path parameter route is rejected", http.MethodPut, "/lrza/fhir/Endpoint/1", "text/plain", `{"resourceType":"Endpoint"}`, http.StatusUnsupportedMediaType},
			{"JSON on the form search is rejected", http.MethodPost, "/nvi/List/_search", "application/json", `{}`, http.StatusUnsupportedMediaType},
			{"a form on the form search passes", http.MethodPost, "/nvi/List/_search", "application/x-www-form-urlencoded", "code=x", http.StatusNotFound},
			{"FHIR JSON on the PDP is rejected", http.MethodPost, "/pdp", "application/fhir+json", `{}`, http.StatusUnsupportedMediaType},
			{"JSON on the PDP passes", http.MethodPost, "/pdp", "application/json", `{}`, http.StatusNotFound},
			{"a GET without Content-Type passes", http.MethodGet, "/nvi/List", "", "", http.StatusNotFound},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				recorder := serve(t, tc.method, tc.path, tc.contentType, strings.NewReader(tc.body))
				assert.Equal(t, tc.wantCode, recorder.Code, recorder.Body.String())
				if tc.wantCode == http.StatusUnsupportedMediaType {
					outcome := operationOutcome(t, recorder)
					assert.Equal(t, fhir.IssueTypeNotSupported, outcome.Issue[0].Code)
					assert.Contains(t, *outcome.Issue[0].Diagnostics, "Content-Type must be one of: ")
				}
			})
		}
	})
}

// TestRequestContentTypes_MatchOpenAPISpec fails when requestContentTypes drifts from the request
// bodies declared in openapi.yaml, e.g. when an operation with a body is added to the spec.
func TestRequestContentTypes_MatchOpenAPISpec(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile("../openapi.yaml")
	require.NoError(t, err)

	expected := map[string][]string{}
	for path, pathItem := range doc.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			if operation.RequestBody == nil || operation.RequestBody.Value == nil {
				continue
			}
			var contentTypes []string
			for contentType := range operation.RequestBody.Value.Content {
				contentTypes = append(contentTypes, contentType)
			}
			if slices.Contains(contentTypes, "application/fhir+json") && !slices.Contains(contentTypes, "application/json") {
				contentTypes = append(contentTypes, "application/json")
			}
			slices.Sort(contentTypes)
			expected[method+" "+path] = contentTypes
		}
	}
	require.NotEmpty(t, expected)

	actual := map[string][]string{}
	for pattern, contentTypes := range requestContentTypes {
		actual[pattern] = slices.Sorted(slices.Values(contentTypes))
	}
	assert.Equal(t, expected, actual)
}
