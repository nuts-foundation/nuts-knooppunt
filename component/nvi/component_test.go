package nvi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/nvi/testdata"
	"github.com/nuts-foundation/nuts-knooppunt/component/pseudonimization"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	testUtil "github.com/nuts-foundation/nuts-knooppunt/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_handleRegister(t *testing.T) {
	testCases := []struct {
		name                       string
		nviTransportError          error
		requestBody                []byte
		expectedStatus             int
		expectedOperationOutcome   *fhir.OperationOutcome
		expectedNVICreatedResource fhir.DocumentReference
	}{
		{
			name:                       "registered at NVI",
			requestBody:                testUtil.ReadJSON(t, testdata.FS, "documentreference.json"),
			expectedStatus:             http.StatusCreated,
			expectedNVICreatedResource: testUtil.ParseJSON[fhir.DocumentReference](t, testdata.FS, "documentreference-response.json"),
		},
		{
			name:                       "sets profile if not set",
			requestBody:                testUtil.ReadJSON(t, testdata.FS, "documentreference-without-profile.json"),
			expectedStatus:             http.StatusCreated,
			expectedNVICreatedResource: testUtil.ParseJSON[fhir.DocumentReference](t, testdata.FS, "documentreference-response.json"),
		},
		{
			name:              "NVI is down",
			nviTransportError: assert.AnError,
			requestBody:       testUtil.ReadJSON(t, testdata.FS, "documentreference.json"),
			expectedStatus:    http.StatusServiceUnavailable,
			expectedOperationOutcome: &fhir.OperationOutcome{
				Issue: []fhir.OperationOutcomeIssue{
					{
						Severity:    fhir.IssueSeverityError,
						Code:        fhir.IssueTypeTransient,
						Diagnostics: to.Ptr("Failed to register DocumentReference at NVI"),
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			nvi := &test.StubFHIRClient{
				Error: testCase.nviTransportError,
			}
			component := Component{
				client:        nvi,
				pseudonymizer: &pseudonimization.Component{},
			}
			httpRequest := httptest.NewRequest("POST", "/nvi/DocumentReference", bytes.NewReader(testCase.requestBody))
			httpRequest.Header.Add("Content-Type", "application/fhir+json")
			httpResponse := httptest.NewRecorder()

			component.handleRegister(httpResponse, httpRequest)

			require.Equal(t, testCase.expectedStatus, httpResponse.Code)
			responseData, _ := io.ReadAll(httpResponse.Body)

			if testCase.expectedOperationOutcome != nil {
				var operationOutcome fhir.OperationOutcome
				err := json.Unmarshal(responseData, &operationOutcome)
				require.NoError(t, err)
				require.Equal(t, testCase.expectedOperationOutcome, &operationOutcome)
			}
			if testCase.expectedStatus == http.StatusCreated {
				require.Len(t, nvi.CreatedResources["DocumentReference"], 1)
				actual := nvi.CreatedResources["DocumentReference"][0].(fhir.DocumentReference)
				// copy identifier because it's scrubbed
				actualPatientIdentifier := fhir.Identifier{
					System: to.Ptr(*actual.Subject.Identifier.System),
					Value:  to.Ptr(*actual.Subject.Identifier.Value),
				}
				// scrub patient identifier value for comparison (it's random)
				actual.Subject.Identifier.Value = nil
				actualJSON, _ := json.Marshal(actual)
				expectedJSON, _ := json.Marshal(testCase.expectedNVICreatedResource)
				assert.JSONEq(t, string(expectedJSON), string(actualJSON))

				t.Run("assert BSNs are translated", func(t *testing.T) {
					assert.Equal(t, "http://fhir.nl/fhir/NamingSystem/bsn-transport-token", *actualPatientIdentifier.System)
					assert.Contains(t, *actualPatientIdentifier.Value, "token-nvi")
				})
			}
		})
	}

}

func TestComponent_handleSearch(t *testing.T) {
	ref := testUtil.ParseJSON[fhir.DocumentReference](t, testdata.FS, "documentreference.json")

	testCases := []struct {
		name                     string
		nviResources             []any
		nviTransportError        error
		searchParams             string
		expectedStatus           int
		expectedEntries          int
		expectedOperationOutcome *fhir.OperationOutcome
		httpMethod               string
	}{
		{
			name:            "searches at NVI with POST",
			nviResources:    []any{ref},
			expectedStatus:  http.StatusOK,
			expectedEntries: 1,
			searchParams:    "status=current",
		},
		{
			name:            "searches at NVI with GET",
			nviResources:    []any{ref},
			expectedStatus:  http.StatusOK,
			expectedEntries: 1,
			httpMethod:      "GET",
			searchParams:    "status=current",
		},
		{
			name:           "invalid search request",
			nviResources:   nil,
			searchParams:   ";",
			expectedStatus: http.StatusBadRequest,
			expectedOperationOutcome: &fhir.OperationOutcome{
				Issue: []fhir.OperationOutcomeIssue{
					{
						Severity:    fhir.IssueSeverityError,
						Code:        fhir.IssueTypeInvalid,
						Diagnostics: to.Ptr("request body is not valid application/x-www-form-urlencoded"),
					},
				},
			},
		},
		{
			name:           "NVI returns next page",
			nviResources:   []any{ref, ref},
			searchParams:   "_count=1",
			expectedStatus: http.StatusUnprocessableEntity,
			expectedOperationOutcome: &fhir.OperationOutcome{
				Issue: []fhir.OperationOutcomeIssue{
					{
						Severity:    fhir.IssueSeverityError,
						Code:        fhir.IssueTypeTooCostly,
						Diagnostics: to.Ptr("NVI returned more results than can be handled. Please refine your search, or increase _count."),
					},
				},
			},
		},
		{
			name:              "NVI is down",
			nviTransportError: assert.AnError,
			expectedStatus:    http.StatusServiceUnavailable,
			expectedOperationOutcome: &fhir.OperationOutcome{
				Issue: []fhir.OperationOutcomeIssue{
					{
						Severity:    fhir.IssueSeverityError,
						Code:        fhir.IssueTypeTransient,
						Diagnostics: to.Ptr("Failed to search for DocumentReferences at NVI"),
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			nvi := &test.StubFHIRClient{
				Resources: testCase.nviResources,
				Error:     testCase.nviTransportError,
			}
			component := Component{
				client:        nvi,
				pseudonymizer: &pseudonimization.Component{},
			}

			var httpRequest *http.Request
			if testCase.httpMethod == "GET" {
				httpRequest = httptest.NewRequest("GET", "/nvi/DocumentReference?"+testCase.searchParams, nil)
			} else {
				httpRequest = httptest.NewRequest("POST", "/nvi/DocumentReference/_search", bytes.NewReader([]byte(testCase.searchParams)))
				httpRequest.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			}
			httpResponse := httptest.NewRecorder()
			component.handleSearch(httpResponse, httpRequest)

			require.Equal(t, testCase.expectedStatus, httpResponse.Code)
			responseData, _ := io.ReadAll(httpResponse.Body)

			if testCase.expectedOperationOutcome != nil {
				var operationOutcome fhir.OperationOutcome
				err := json.Unmarshal(responseData, &operationOutcome)
				require.NoError(t, err)
				require.Equal(t, testCase.expectedOperationOutcome, &operationOutcome)
			}
			if testCase.expectedEntries > 0 {
				var bundle fhir.Bundle
				err := json.Unmarshal(responseData, &bundle)
				require.NoError(t, err)
				require.Len(t, bundle.Entry, testCase.expectedEntries)
			}
		})
	}
}
