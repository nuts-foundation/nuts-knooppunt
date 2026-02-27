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

func TestHandleMainPolicy(t *testing.T) {
	t.Run("invalid HTTP request body", func(t *testing.T) {
		service := &Component{}
		httpRequest := httptest.NewRequest("POST", "/pdp", strings.NewReader("invalid json"))
		httpRequest.Header.Set("Content-Type", "application/json")
		httpResponse := httptest.NewRecorder()

		service.HandleMainPolicy(httpResponse, httpRequest)

		assert.Equal(t, http.StatusBadRequest, httpResponse.Code)
		var actual PDPResponse
		err := json.NewDecoder(httpResponse.Body).Decode(&actual)
		require.NoError(t, err)
		require.False(t, actual.Result.Allow)
		assert.Len(t, actual.Result.Reasons, 1)
		assert.Equal(t, TypeResultCodeUnexpectedInput, actual.Result.Reasons[0].Code)
	})
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
		properties           map[string]any
		mainReasonCodes      []TypeResultCode
		policyReasonCodes    map[string][]TypeResultCode
		policyAllow          map[string]bool // which policies should allow (true) or deny (false)
	}
	runTest := func(t *testing.T, tc testCase) {
		t.Helper()
		httpReqParts := strings.Split(tc.httpRequest, " ")
		httpReqURL, err := url.Parse("http://localhost" + httpReqParts[1])
		require.NoError(t, err)

		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						OtherProps:            tc.properties,
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
					Path:        httpReqURL.Path,
					QueryParams: httpReqURL.Query(),
					Header: http.Header{
						"Content-Type": {"application/fhir+json"},
					},
				},
				Context: PDPContext{
					DataHolderOrganizationId: "00000002",
					DataHolderFacilityType:   "TODO",
					ConnectionTypeCode:       "hl7-fhir-rest",
				},
			},
		}
		if tc.httpRequestBody != "" {
			pdpRequest.Input.Request.Body = tc.httpRequestBody
		}
		response := executePDPRequest(t, service, pdpRequest)
		if tc.decision {
			assert.True(t, response.Result.Allow, tc.name)
		} else {
			assert.False(t, response.Result.Allow, tc.name)
		}
		for _, expectedCode := range tc.mainReasonCodes {
			found := false
			for _, reason := range response.Result.Reasons {
				if reason.Code == expectedCode {
					found = true
					break
				}
			}
			assert.True(t, found, "expected reason code %s not found in response (got: %v)", expectedCode, response.Result.Reasons)
		}
		if tc.policyReasonCodes != nil {
			for policyName, expectedCodes := range tc.policyReasonCodes {
				policyResult, ok := response.Policies[policyName]
				require.True(t, ok, "expected policy result for policy %s not found in response", policyName)
				for _, expectedCode := range expectedCodes {
					found := false
					for _, reason := range policyResult.Reasons {
						if reason.Code == expectedCode {
							found = true
							break
						}
					}
					assert.True(t, found, "expected reason code %s for policy %s not found in response (got: %v)", expectedCode, policyName, policyResult.Reasons)
				}
			}
		}
		if tc.policyAllow != nil {
			for policyName, expectedAllow := range tc.policyAllow {
				policyResult, ok := response.Policies[policyName]
				require.True(t, ok, "expected policy result for policy %s not found in response", policyName)
				assert.Equal(t, expectedAllow, policyResult.Allow, "expected policy %s allow to be %v, got %v", policyName, expectedAllow, policyResult.Allow)
			}
		}
	}

	t.Run("bgz", func(t *testing.T) {
		testCases := []testCase{
			{
				name:                 "allow - multiple policies, first denies but second allows",
				clientQualifications: []string{"medicatieoverdracht", "bgz"},
				httpRequest:          `GET /Patient?_include=Patient:general-practitioner&_id=1000`,
				decision:             true,
				policyReasonCodes: map[string][]TypeResultCode{
					"medicatieoverdracht": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
					"bgz":                 {},
				},
				policyAllow: map[string]bool{
					"medicatieoverdracht": false,
					"bgz":                 true,
				},
			},
			{
				name:                 "disallow - Mitz consent not given",
				clientQualifications: []string{"bgz"},
				httpRequest:          `GET /Patient?_include=Patient:general-practitioner&_id=1001`,
				decision:             false,
				policyReasonCodes: map[string][]TypeResultCode{
					"bgz": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
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
				policyReasonCodes: map[string][]TypeResultCode{
					"bgz": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:                 "disallow - Patient query with additional parameters",
				clientQualifications: []string{"bgz"},
				httpRequest:          `GET /Patient?_include=Patient:general-practitioner&name=John`,
				policyReasonCodes: map[string][]TypeResultCode{
					"bgz": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:                 "disallow - Patient query without patient_id or patient_bsn",
				clientQualifications: []string{"bgz"},
				httpRequest:          `GET /Patient?_include=Patient:general-practitioner`,
				policyReasonCodes: map[string][]TypeResultCode{
					"bgz": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
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
				name:                 "allow - dash is normalized to underscore",
				clientQualifications: []string{"pzp-gf"},
				httpRequest:          `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|123456789`,
				decision:             true,
			},
			{
				name:                 "allow - patient identifier is encoded",
				clientQualifications: []string{"pzp-gf"},
				httpRequest:          `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn%7C123456789`,
				decision:             true,
			},
			{
				name:                 "allow - Patient search with BSN identifier",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|123456789`,
				decision:             true,
			},
			{
				name:                 "deny - Patient search without BSN namespace",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Patient?identifier=123456789`,
				decision:             false,
				mainReasonCodes:      []TypeResultCode{TypeResultCodeUnexpectedInput},
			},
			{
				name:                 "deny - Patient search with wrong identifier system",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Patient?identifier=http://example.com/identifier|123456789`,
				decision:             false,
				mainReasonCodes:      []TypeResultCode{TypeResultCodeUnexpectedInput},
			},
			{
				name:                 "allow - Consent search with patient, scope and category",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Consent?patient=Patient/1000&scope=http://terminology.hl7.org/CodeSystem/consentscope|treatment&category=http://snomed.info/sct|129125009`,
				decision:             true,
			},
			{
				name:                 "allow - Consent search with patient, scope, category and include",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Consent?patient=Patient/1000&scope=http://terminology.hl7.org/CodeSystem/consentscope|treatment&category=http://snomed.info/sct|129125009&_include=Consent:actor`,
				decision:             true,
			},
			{
				name:                 "deny - Consent search with multiple patient refs",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Consent?patient=Patient/1000,Patient/1001&_profile=http://nictiz.nl/fhir/StructureDefinition/nl-core-TreatmentDirective2`,
				decision:             false,
				mainReasonCodes:      []TypeResultCode{TypeResultCodeUnexpectedInput},
			},
			{
				name:                 "deny - Consent search without patient parameter",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Consent?_profile=http://example.com/fhir/StructureDefinition/consent-profile`,
				decision:             false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:                 "deny - Consent search without _profile parameter",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Consent?patient=Patient/1000`,
				decision:             false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:                 "deny - Consent search with empty patient parameter",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Consent?patient=&_profile=http://example.com/fhir/StructureDefinition/consent-profile`,
				decision:             false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:                 "deny - Patient search without patient_id or patient_bsn",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Patient?`,
				decision:             false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:                 "deny - Mitz consent check failure",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|bsn:error`,
				decision:             false,
				mainReasonCodes:      []TypeResultCode{TypeResultCodeInternalError},
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:                 "deny - Mitz consent not given",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|bsn:deny`,
				decision:             false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:                 "deny - unsupported resource type",
				clientQualifications: []string{"pzp_gf"},
				httpRequest:          `GET /Observation?patient=Patient/1000`,
				decision:             false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				runTest(t, tc)
			})
		}
	})
	t.Run("medicatieoverdracht", func(t *testing.T) {
		testCases := []testCase{
			{
				name:                 "allow - MedicationRequest with correct category and _include",
				clientQualifications: []string{"medicatieoverdracht"},
				httpRequest:          `GET /MedicationRequest?category=http://snomed.info/sct|422037009&_include=MedicationRequest:medication&patient=Patient/1000`,
				decision:             true,
				properties: OtherSubjectProperties{
					"patient_enrollment_identifier": "http://fhir.nl/fhir/NamingSystem/bsn|123456789",
				},
			},
			{
				name:                 "deny - List search",
				clientQualifications: []string{"medicatieoverdracht"},
				httpRequest:          `GET /List?patient=Patient/1000`,
				decision:             false,
				policyReasonCodes: map[string][]TypeResultCode{
					"medicatieoverdracht": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
				properties: OtherSubjectProperties{
					"patient_enrollment_identifier": "http://fhir.nl/fhir/NamingSystem/bsn|123456789",
				},
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				runTest(t, tc)
			})
		}
	})
}
