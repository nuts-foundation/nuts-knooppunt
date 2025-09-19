package nvi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/nvi/testdata"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_handleRegister(t *testing.T) {
	t.Run("registered at NVI", func(t *testing.T) {
		testResource, _ := testdata.FS.ReadFile("documentreference.json")
		nvi := &test.StubFHIRClient{}
		component := Component{
			client: nvi,
		}
		httpRequest := httptest.NewRequest("POST", "/nvi/DocumentReference", bytes.NewReader(testResource))
		httpRequest.Header.Add("Content-Type", "application/fhir+json")
		httpResponse := httptest.NewRecorder()

		component.handleRegister(httpResponse, httpRequest)

		require.Equal(t, http.StatusCreated, httpResponse.Code)
		require.Len(t, nvi.CreatedResources["DocumentReference"], 1)
		actual := nvi.CreatedResources["DocumentReference"][0].(fhir.DocumentReference)
		actualJSON, _ := json.Marshal(actual)
		require.JSONEq(t, string(testResource), string(actualJSON))
	})
	t.Run("sets profile if not set", func(t *testing.T) {
		testResource, _ := testdata.FS.ReadFile("documentreference-without-profile.json")
		nvi := &test.StubFHIRClient{}
		component := Component{
			client: nvi,
		}
		httpRequest := httptest.NewRequest("POST", "/nvi/DocumentReference", bytes.NewReader(testResource))
		httpRequest.Header.Add("Content-Type", "application/fhir+json")
		httpResponse := httptest.NewRecorder()

		component.handleRegister(httpResponse, httpRequest)

		require.Equal(t, http.StatusCreated, httpResponse.Code)
		require.Len(t, nvi.CreatedResources["DocumentReference"], 1)
		actual := nvi.CreatedResources["DocumentReference"][0].(fhir.DocumentReference)
		require.NotNil(t, actual.Meta)
		require.Len(t, actual.Meta.Profile, 1)
		require.Equal(t, "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-localization-documentreference", actual.Meta.Profile[0])
	})
	t.Run("NVI is down", func(t *testing.T) {
		testResource, _ := testdata.FS.ReadFile("documentreference.json")
		nvi := &test.StubFHIRClient{
			Error: assert.AnError,
		}
		component := Component{
			client: nvi,
		}
		httpRequest := httptest.NewRequest("POST", "/nvi/DocumentReference", bytes.NewReader(testResource))
		httpRequest.Header.Add("Content-Type", "application/fhir+json")
		httpResponse := httptest.NewRecorder()

		component.handleRegister(httpResponse, httpRequest)

		require.Equal(t, http.StatusServiceUnavailable, httpResponse.Code)
		responseData, _ := io.ReadAll(httpResponse.Body)
		var operationOutcome fhir.OperationOutcome
		err := json.Unmarshal(responseData, &operationOutcome)
		require.NoError(t, err)
		require.Len(t, operationOutcome.Issue, 1)
		require.Equal(t, fhir.IssueSeverityError, operationOutcome.Issue[0].Severity)
		require.Equal(t, fhir.IssueTypeTransient, operationOutcome.Issue[0].Code)
		require.Equal(t, *operationOutcome.Issue[0].Diagnostics, "Failed to register DocumentReference at NVI")
	})
}
