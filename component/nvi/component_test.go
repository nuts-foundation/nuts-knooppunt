package nvi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/nvi/testdata"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	testUtil "github.com/nuts-foundation/nuts-knooppunt/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_handleRegister(t *testing.T) {
	transactionBundle := testUtil.ReadJSON(t, testdata.FS, "bundle-transaction.json")

	testCases := []struct {
		name                     string
		nviTransportError        error
		requestBody              []byte
		tenantID                 *string
		expectedStatus           int
		expectedOperationOutcome *fhir.OperationOutcome
	}{
		{
			name:           "registered at NVI",
			requestBody:    transactionBundle,
			expectedStatus: http.StatusCreated,
		},
		{
			name: "wrong bundle type",
			requestBody: func() []byte {
				b, _ := json.Marshal(fhir.Bundle{Type: fhir.BundleTypeSearchset})
				return b
			}(),
			expectedStatus: http.StatusBadRequest,
			expectedOperationOutcome: &fhir.OperationOutcome{
				Issue: []fhir.OperationOutcomeIssue{
					{
						Severity:    fhir.IssueSeverityError,
						Code:        fhir.IssueTypeValue,
						Diagnostics: to.Ptr("Bundle must be of type transaction"),
					},
				},
			},
		},
		{
			name:              "NVI is down",
			nviTransportError: assert.AnError,
			requestBody:       transactionBundle,
			expectedStatus:    http.StatusServiceUnavailable,
			expectedOperationOutcome: &fhir.OperationOutcome{
				Issue: []fhir.OperationOutcomeIssue{
					{
						Severity:    fhir.IssueSeverityError,
						Code:        fhir.IssueTypeTransient,
						Diagnostics: to.Ptr("Failed to register Bundle at NVI"),
					},
				},
			},
		},
		{
			name:           "invalid tenant ID",
			requestBody:    transactionBundle,
			expectedStatus: http.StatusBadRequest,
			tenantID:       to.Ptr("invalid"),
			expectedOperationOutcome: &fhir.OperationOutcome{
				Issue: []fhir.OperationOutcomeIssue{
					{
						Severity:    fhir.IssueSeverityError,
						Code:        fhir.IssueTypeValue,
						Diagnostics: to.Ptr("invalid tenant ID in request header"),
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tenantID := coding.URANamingSystem + "|1"
			if testCase.tenantID != nil {
				tenantID = *testCase.tenantID
			}

			nvi := &test.StubFHIRClient{
				Error: testCase.nviTransportError,
			}
			component := Component{
				client: nvi,
			}
			httpRequest := httptest.NewRequest("POST", "/nvi/Bundle", bytes.NewReader(testCase.requestBody))
			httpRequest.Header.Add("Content-Type", "application/fhir+json")
			httpRequest.Header.Add("X-Tenant-ID", tenantID)
			httpResponse := httptest.NewRecorder()

			component.handleRegister(httpResponse, httpRequest)

			require.Equal(t, testCase.expectedStatus, httpResponse.Code)
			responseData, _ := io.ReadAll(httpResponse.Body)

			if testCase.expectedOperationOutcome != nil {
				var operationOutcome fhir.OperationOutcome
				err := json.Unmarshal(responseData, &operationOutcome)
				require.NoError(t, err)
				expectedJSON, _ := json.Marshal(testCase.expectedOperationOutcome)
				require.JSONEq(t, string(expectedJSON), string(responseData))
			}
			if testCase.expectedStatus == http.StatusCreated {
				var responseBundle fhir.Bundle
				err := json.Unmarshal(responseData, &responseBundle)
				require.NoError(t, err)
				assert.Equal(t, fhir.BundleTypeTransactionResponse, responseBundle.Type)
			}
		})
	}
}

func TestComponent_handleSearch(t *testing.T) {
	listResource := testUtil.ParseJSON[fhir.List](t, testdata.FS, "list-resource.json")

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
			name:            "searches List at NVI with GET",
			nviResources:    []any{listResource},
			expectedStatus:  http.StatusOK,
			expectedEntries: 1,
		},
		{
			name:            "searches List at NVI with POST",
			nviResources:    []any{listResource},
			expectedStatus:  http.StatusOK,
			expectedEntries: 1,
			httpMethod:      "POST",
		},
		{
			name:           "NVI returns next page",
			nviResources:   []any{listResource, listResource},
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
						Diagnostics: to.Ptr("Failed to search for List resources at NVI"),
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
				client: nvi,
			}

			searchParams := testCase.searchParams
			if searchParams == "" {
				searchParams = "patient.identifier=http%3A%2F%2Ffhir.nl%2Ffhir%2FNamingSystem%2Fpseudo-bsn%7CUHN1ZWRvYnNuOiA5OTk5NDAwMw%3D%3D&code=LABBEPALING"
			}

			var httpRequest *http.Request
			if testCase.httpMethod == "POST" {
				httpRequest = httptest.NewRequest("POST", "/nvi/List/_search", bytes.NewReader([]byte(searchParams)))
				httpRequest.Header.Add("Content-Type", "application/x-www-form-urlencoded")
			} else {
				httpRequest = httptest.NewRequest("GET", "/nvi/List?"+searchParams, nil)
			}
			httpRequest.Header.Add("X-Tenant-ID", coding.URANamingSystem+"|1")
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

				t.Run("assert search was forwarded to NVI as List search", func(t *testing.T) {
					require.Len(t, nvi.Searches, 1)
					assert.Contains(t, nvi.Searches[0], "List?")
				})
			}
		})
	}
}
