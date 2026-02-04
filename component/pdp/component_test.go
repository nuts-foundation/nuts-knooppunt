package pdp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// executePDPRequest is a helper function that sends a PDP request and returns the response
func executePDPRequest(t *testing.T, service *Component, pdpRequest PDPRequest) PDPResponse {
	t.Helper()

	// Marshal the request body
	requestBody, err := json.Marshal(pdpRequest)
	require.NoError(t, err)

	// Create HTTP request
	req := httptest.NewRequest("POST", "/pdp", bytes.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	// Create response recorder
	w := httptest.NewRecorder()

	// Call the handler
	service.HandleMainPolicy(w, req)

	// Check the response
	assert.Equal(t, http.StatusOK, w.Code)

	var response PDPResponse
	err = json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	return response
}

func TestHandleMainPolicy_Integration(t *testing.T) {
	mux := http.NewServeMux()
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	pipClient := &test.StubFHIRClient{
		Resources: []any{
			fhir.Patient{
				Id: to.Ptr("1000"),
				Identifier: []fhir.Identifier{
					{
						System: to.Ptr(coding.BSNNamingSystem),
						Value:  to.Ptr("123456789"),
					},
				},
			},
			fhir.Patient{
				Id: to.Ptr("1001"),
				Identifier: []fhir.Identifier{
					{
						System: to.Ptr(coding.BSNNamingSystem),
						Value:  to.Ptr("bsn:deny"),
					},
				},
			},
		},
	}

	service, err := New(Config{
		Enabled: true,
	}, mitz.NewTestInstance(t))
	require.NoError(t, err)
	service.opaBundleBaseURL = httpServer.URL + "/pdp/bundles/"
	service.pipClient = pipClient

	service.RegisterHttpHandlers(nil, mux)

	require.NoError(t, service.Start())
	defer func() {
		require.NoError(t, service.Stop(context.Background()))
	}()

	type testCase struct {
		name                 string
		clientQualifications []string
		httpRequest          string
		httpRequestBody      string
		decision             bool
	}
	runTest := func(t *testing.T, tc testCase) {
		t.Helper()
		httpReqParts := strings.Split(tc.httpRequest, " ")
		httpReqURL, err := url.Parse("http://localhost" + httpReqParts[1])
		require.NoError(t, err)
		path := httpReqURL.Path
		if httpReqURL.RawQuery != "" {
			path += "?"
		}

		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						ClientQualifications:  tc.clientQualifications,
						SubjectOrganizationId: "00000001",
						SubjectFacilityType:   "TODO",
						SubjectRole:           "TODO",
						SubjectId:             "TODO",
					},
				},
				Request: HTTPRequest{
					Method:      httpReqParts[0],
					Protocol:    "HTTP/1.1",
					Path:        path,
					QueryParams: httpReqURL.Query(),
					Header: http.Header{
						"Content-Type": {"application/fhir+json"},
					},
				},
				Context: PDPContext{
					DataHolderOrganizationId: "00000002",
					DataHolderFacilityType:   "TODO",
					ConnectionTypeCode: "hl7-fhir-rest",
				},
			},
		}
		if tc.httpRequestBody != "" {
			pdpRequest.Input.Request.Body = tc.httpRequestBody
		}
		response := executePDPRequest(t, service, pdpRequest)
		if tc.decision {
			assert.True(t, response.Result.Allow, tc.name)
			assert.Empty(t, response.Result.Reasons, tc.name)
		} else {
			assert.False(t, response.Result.Allow, tc.name)
			assert.NotEmpty(t, response.Result.Reasons, tc.name)
		}
	}

	t.Run("bgz", func(t *testing.T) {
		testCases := []testCase{
			{
				name:                 "disallow - Mitz consent not given",
				clientQualifications: []string{"bgz"},
				httpRequest:          `GET /Patient?_include=Patient:general-practitioner&_id=1001`,
				decision:             false,
			},
			{
				name:                 "allow - correct Patient query with _include",
				clientQualifications: []string{"bgz"},
				httpRequest:          `GET /Patient?_include=Patient:general-practitioner&_id=1000`,
				decision:             true,
			},
			{
				name:                 "allow - correct Patient query with BSN",
				clientQualifications: []string{"bgz"},
				httpRequest:          `GET /Patient?_include=Patient:general-practitioner&_id=1000`,
				decision:             true,
			},
			{
				name:                 "allow - correct MedicationDispense query with category and _include",
				clientQualifications: []string{"bgz"},
				httpRequest:          `GET /MedicationDispense?category=http://snomed.info/sct|422037009&_include=MedicationDispense:medication&patient=Patient/1000`,
				decision:             true,
			},
			{
				name:                 "disallow - Patient query with wrong _include parameter",
				clientQualifications: []string{"bgz"},
				httpRequest:          `GET /Patient?_include=Patient:organization`,
			},
			{
				name:                 "disallow - Patient query with additional parameters",
				clientQualifications: []string{"bgz"},
				httpRequest:          `GET /Patient?_include=Patient:general-practitioner&name=John`,
			},
			{
				name:                 "disallow - Patient query without patient_id or patient_bsn",
				clientQualifications: []string{"bgz"},
				httpRequest:          `GET /Patient?_include=Patient:general-practitioner`,
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				runTest(t, tc)
			})
		}
	})
	t.Run("pzp", func(t *testing.T) {
		testCases := []testCase{
			{
				name:                 "allow - Patient search with BSN identifier",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|123456789`,
				decision:             true,
			},
			{
				name:                 "deny - Patient search without BSN namespace",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Patient?identifier=123456789`,
				decision:             false,
			},
			{
				name:                 "deny - Patient search with wrong identifier system",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Patient?identifier=http://example.com/identifier|123456789`,
				decision:             false,
			},
			{
				name:                 "allow - Consent search with patient and _profile",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Consent?patient=Patient/1000&_profile=http://nictiz.nl/fhir/StructureDefinition/nl-core-TreatmentDirective2`,
				decision:             true,
			},
			{
				name:                 "deny - Consent search with multiple patient refs",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Consent?patient=Patient/1000,Patient/1001&_profile=http://nictiz.nl/fhir/StructureDefinition/nl-core-TreatmentDirective2`,
				decision:             false,
			},
			{
				name:                 "deny - Consent search without patient parameter",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Consent?_profile=http://example.com/fhir/StructureDefinition/consent-profile`,
				decision:             false,
			},
			{
				name:                 "deny - Consent search without _profile parameter",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Consent?patient=Patient/1000`,
				decision:             false,
			},
			{
				name:                 "deny - Consent search with empty patient parameter",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Consent?patient=&_profile=http://example.com/fhir/StructureDefinition/consent-profile`,
				decision:             false,
			},
			{
				name:                 "deny - Patient search without patient_id or patient_bsn",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Patient?`,
				decision:             false,
			},
			{
				name:                 "deny - Mitz consent not given",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|bsn:deny`,
				decision:             false,
			},
			{
				name:                 "deny - unsupported resource type",
				clientQualifications: []string{"pzp"},
				httpRequest:          `GET /Observation?patient=Patient/1000`,
				decision:             false,
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				runTest(t, tc)
			})
		}
	})
}
