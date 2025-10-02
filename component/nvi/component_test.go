package nvi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/nvi/testdata"
	"github.com/nuts-foundation/nuts-knooppunt/component/pseudonimization"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	testUtil "github.com/nuts-foundation/nuts-knooppunt/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
	"go.uber.org/mock/gomock"
)

var bsnIdentifier = fhir.Identifier{
	System: to.Ptr(coding.BSNNamingSystem),
	Value:  to.Ptr("123456789"),
}
var bsnTokenIdentifier = fhir.Identifier{
	System: to.Ptr(coding.BSNTransportTokenNamingSystem),
	Value:  to.Ptr("abcdefghi"),
}

func TestComponent_handleRegister(t *testing.T) {
	testCases := []struct {
		name                       string
		nviTransportError          error
		requestBody                []byte
		tenantID                   *string
		expectedStatus             int
		expectedOperationOutcome   *fhir.OperationOutcome
		expectedNVICreatedResource fhir.DocumentReference
	}{
		{
			name:                       "registered at NVI",
			requestBody:                testUtil.ReadJSON(t, testdata.FS, "documentreference-bsn.json"),
			expectedStatus:             http.StatusCreated,
			expectedNVICreatedResource: testUtil.ParseJSON[fhir.DocumentReference](t, testdata.FS, "documentreference-tokenized.json"),
		},
		{
			name:                       "sets profile if not set",
			requestBody:                testUtil.ReadJSON(t, testdata.FS, "documentreference-without-profile.json"),
			expectedStatus:             http.StatusCreated,
			expectedNVICreatedResource: testUtil.ParseJSON[fhir.DocumentReference](t, testdata.FS, "documentreference-tokenized.json"),
		},
		{
			name:              "NVI is down",
			nviTransportError: assert.AnError,
			requestBody:       testUtil.ReadJSON(t, testdata.FS, "documentreference-bsn.json"),
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
		{
			name:           "invalid tenant ID",
			requestBody:    testUtil.ReadJSON(t, testdata.FS, "documentreference-bsn.json"),
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
			tenantURA := "1"
			tenantID := coding.URANamingSystem + "|" + tenantURA
			if testCase.tenantID != nil {
				tenantID = *testCase.tenantID
				identifier, err := fhirutil.TokenToIdentifier(*testCase.tenantID)
				if err == nil && identifier.Value != nil {
					tenantURA = *identifier.Value
				}
			}

			ctrl := gomock.NewController(t)
			pseudonymizer := pseudonimization.NewMockPseudonymizer(ctrl)
			pseudonymizer.EXPECT().IdentifierToToken(bsnIdentifier, "nvi").Return(&bsnTokenIdentifier, nil).AnyTimes()
			pseudonymizer.EXPECT().TokenToBSN(bsnTokenIdentifier, tenantURA).Return(&bsnIdentifier, nil).AnyTimes()
			nvi := &test.StubFHIRClient{
				Error: testCase.nviTransportError,
			}
			component := Component{
				client:        nvi,
				pseudonymizer: pseudonymizer,
				audience:      "nvi",
			}
			httpRequest := httptest.NewRequest("POST", "/nvi/DocumentReference", bytes.NewReader(testCase.requestBody))
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
				require.Len(t, nvi.CreatedResources["DocumentReference"], 1)
				actual := nvi.CreatedResources["DocumentReference"][0].(fhir.DocumentReference)
				actualJSON, _ := json.Marshal(actual)
				expectedJSON, _ := json.Marshal(testCase.expectedNVICreatedResource)
				assert.JSONEq(t, string(expectedJSON), string(actualJSON))

				t.Run("assert BSNs are translated", func(t *testing.T) {
					assert.Equal(t, bsnTokenIdentifier, *actual.Subject.Identifier)
				})
			}
		})
	}

}

func TestComponent_handleSearch(t *testing.T) {
	documentReference := testUtil.ParseJSON[fhir.DocumentReference](t, testdata.FS, "documentreference-tokenized.json")

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
			nviResources:    []any{documentReference},
			expectedStatus:  http.StatusOK,
			expectedEntries: 1,
		},
		{
			name:            "searches at NVI with GET",
			nviResources:    []any{documentReference},
			expectedStatus:  http.StatusOK,
			expectedEntries: 1,
			httpMethod:      "GET",
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
			nviResources:   []any{documentReference, documentReference},
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
			ctrl := gomock.NewController(t)
			pseudonymizer := pseudonimization.NewMockPseudonymizer(ctrl)
			pseudonymizer.EXPECT().IdentifierToToken(bsnIdentifier, "nvi").Return(&bsnTokenIdentifier, nil).AnyTimes()
			pseudonymizer.EXPECT().TokenToBSN(bsnTokenIdentifier, "1").Return(&bsnIdentifier, nil).AnyTimes()
			nvi := &test.StubFHIRClient{
				Resources: testCase.nviResources,
				Error:     testCase.nviTransportError,
			}
			component := Component{
				client:        nvi,
				pseudonymizer: pseudonymizer,
			}

			searchParams := testCase.searchParams
			if searchParams == "" {
				searchParams = "patient:identifier=" + url.PathEscape(*bsnIdentifier.System+"|"+*bsnIdentifier.Value)
			}

			var httpRequest *http.Request
			if testCase.httpMethod == "GET" {
				httpRequest = httptest.NewRequest("GET", "/nvi/DocumentReference?"+searchParams, nil)
			} else {
				httpRequest = httptest.NewRequest("POST", "/nvi/DocumentReference/_search", bytes.NewReader([]byte(searchParams)))
				httpRequest.Header.Add("Content-Type", "application/x-www-form-urlencoded")
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
			} else {
				t.Run("assert BSNs are translated to transport tokens, as search input to NVI", func(t *testing.T) {
					require.Len(t, nvi.Searches, 1)
					assert.Equal(t, "DocumentReference?patient%3Aidentifier=http%3A%2F%2Ffhir.nl%2Ffhir%2FNamingSystem%2Fbsn-transport-token%7Cabcdefghi", nvi.Searches[0])
				})
			}
			if testCase.expectedEntries > 0 {
				var bundle fhir.Bundle
				err := json.Unmarshal(responseData, &bundle)
				require.NoError(t, err)
				require.Len(t, bundle.Entry, testCase.expectedEntries)

				t.Run("assert BSN transport tokens are translated back to BSNs", func(t *testing.T) {
					for _, entry := range bundle.Entry {
						var documentReference fhir.DocumentReference
						err := json.Unmarshal(entry.Resource, &documentReference)
						require.NoError(t, err)
						assert.Equal(t, bsnIdentifier, *documentReference.Subject.Identifier)
					}
				})
			}
		})
	}
}
