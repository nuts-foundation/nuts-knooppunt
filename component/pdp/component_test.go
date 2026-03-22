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
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp/policies"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/from"
	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// executePDPRequest is a helper function that sends a PDP request and returns the response
func executePDPRequest(t *testing.T, service *Component, pdpRequest APIRequest) APIResponse {
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

	response, err := from.JSONResponse[APIResponse](w.Result())
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
		var actual APIResponse
		err := json.NewDecoder(httpResponse.Body).Decode(&actual)
		require.NoError(t, err)
		require.False(t, actual.Allow)
		assert.Equal(t, "unable to parse request body: invalid character 'i' looking for beginning of value", actual.Error)
	})
}

func TestHandleMainPolicy_Integration(t *testing.T) {
	// Load all bundles including test_ prefixed ones for unit testing purposes.
	// Test bundles are excluded from production bundle loading (policies.Bundles),
	// but are needed here to test AND/OR search param logic via evalRegoPolicy.
	allBundles, err := policies.GenerateBundles(func(name string) bool { return false })
	require.NoError(t, err)

	mux := http.NewServeMux()
	// Serve all bundles (including test_ ones) over HTTP so OPA can fetch them.
	mux.HandleFunc("GET /pdp/bundles/{policyName}", func(w http.ResponseWriter, r *http.Request) {
		policyName := strings.TrimSuffix(r.PathValue("policyName"), ".tar.gz")
		data, found := allBundles[policyName]
		if !found {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(data)
	})
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

	// Start OPA with all bundles including test_ ones
	opaService, err := createOPAService(t.Context(), service.opaBundleBaseURL, allBundles)
	require.NoError(t, err)
	service.opaService = opaService
	defer func() {
		require.NoError(t, service.Stop(context.Background()))
	}()

	type testCase struct {
		name              string
		scope             string
		httpRequest       string
		httpRequestBody   string
		decision          bool
		properties        map[string]any
		error             string
		policyReasonCodes map[string][]TypeResultCode
		policyAllow       map[string]bool // which policies should allow (true) or deny (false)
	}
	runTest := func(t *testing.T, tc testCase) {
		t.Helper()
		httpReqParts := strings.Split(tc.httpRequest, " ")
		httpReqURL, err := url.Parse("http://localhost" + httpReqParts[1])
		require.NoError(t, err)

		pdpRequest := APIRequest{
			Input: APIInput{
				Subject: APISubject{
					OtherProps:               tc.properties,
					Scope:                    tc.scope,
					OrganizationUra:          "00000001",
					OrganizationFacilityType: "TODO",
					UserId:                   "TODO",
					UserRole:                 "TODO",
				},
				Request: HTTPRequest{
					Method:   httpReqParts[0],
					Protocol: "HTTP/1.1",
					Path:     httpReqURL.Path,
					Query:    httpReqURL.RawQuery,
					Header: http.Header{
						"Content-Type": {"application/fhir+json"},
					},
				},
				Context: APIContext{
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
			assert.True(t, response.Allow, tc.name)
			assert.Empty(t, response.Error, "expected no error when decision is allow")
		} else {
			assert.False(t, response.Allow, tc.name)
		}
		if tc.error != "" || response.Error != "" {
			assert.Equal(t, tc.error, response.Error, tc.name)
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
				name:        "allow - multiple policies, first denies but second allows",
				scope:       "mcsd_update bgz",
				httpRequest: `GET /Organization`,
				decision:    true,
				policyReasonCodes: map[string][]TypeResultCode{
					"mcsd_update": {},
					"bgz":         {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
				policyAllow: map[string]bool{
					"mcsd_update": true,
					"bgz":         false,
				},
			},
			{
				name:        "disallow - Mitz consent not given",
				scope:       "bgz",
				httpRequest: `GET /Patient?_include=Patient:general-practitioner&_id=1001`,
				decision:    false,
				policyReasonCodes: map[string][]TypeResultCode{
					"bgz": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:        "allow - correct Patient query with _include",
				scope:       "bgz",
				httpRequest: `GET /Patient?_include=Patient:general-practitioner&_id=1000`,
				decision:    true,
			},
			{
				name:        "allow - correct Patient query with BSN",
				scope:       "bgz",
				httpRequest: `GET /Patient?_include=Patient:general-practitioner&_id=1000`,
				decision:    true,
			},
			{
				name:        "allow - correct MedicationDispense query with category and _include",
				scope:       "bgz",
				httpRequest: `GET /MedicationDispense?category=http://snomed.info/sct|422037009&_include=MedicationDispense:medication&patient=Patient/1000`,
				decision:    true,
			},
			{
				name:        "allow - correct Observation lastn query with single code",
				scope:       "bgz",
				httpRequest: `GET /Observation/$lastn?code=http://snomed.info/sct|365508006&patient=Patient/1000`,
				decision:    true,
			},
			{
				name:        "allow - correct Observation lastn query with multiple codes",
				scope:       "bgz",
				httpRequest: `GET /Observation/$lastn?code=http://loinc.org|8302-2,http://loinc.org|8306-3,http://loinc.org|8308-9&patient=Patient/1000`,
				decision:    true,
			},
			{
				name:        "allow - correct Observation lastn query with multiple codes turned around",
				scope:       "bgz",
				httpRequest: `GET /Observation/$lastn?code=http://loinc.org|8306-3,http://loinc.org|8302-2,http://loinc.org|8308-9&patient=Patient/1000`,
				decision:    true,
			},
			{
				name:        "allow - correct Observation lastn query with additional not supported param",
				scope:       "bgz",
				httpRequest: `GET /Observation/$lastn?code=http://loinc.org|8306-3,http://loinc.org|8302-2,http://loinc.org|8308-9,http://loinc.org|8308-19&patient=Patient/1000`,
				decision:    false,
			},
			{
				name:        "disallow - Patient query with wrong _include parameter",
				scope:       "bgz",
				httpRequest: `GET /Patient?_include=Patient:organization`,
				policyReasonCodes: map[string][]TypeResultCode{
					"bgz": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:        "disallow - Patient query with additional parameters",
				scope:       "bgz",
				httpRequest: `GET /Patient?_include=Patient:general-practitioner&name=John`,
				policyReasonCodes: map[string][]TypeResultCode{
					"bgz": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:        "disallow - Patient query without patient_id or patient_bsn",
				scope:       "bgz",
				httpRequest: `GET /Patient?_include=Patient:general-practitioner`,
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
				name:        "allow - dash is normalized to underscore",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|123456789`,
				decision:    true,
			},
			{
				name:        "allow - patient identifier is encoded",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn%7C123456789`,
				decision:    true,
			},
			{
				name:        "allow - Patient search with BSN identifier",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|123456789`,
				decision:    true,
			},
			{
				name:        "deny - Patient search without BSN namespace",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?identifier=123456789`,
				decision:    false,
				error:       "invalid request: patient_bsn: expected identifier parameter in format 'system|value'",
			},
			{
				name:        "deny - Patient search with wrong identifier system",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?identifier=http://example.com/identifier|123456789`,
				decision:    false,
				error:       "invalid request: patient_bsn: expected identifier system to be 'http://fhir.nl/fhir/NamingSystem/bsn', found 'http://example.com/identifier'",
			},
			{
				name:        "allow - Consent search with patient, scope and category",
				scope:       "pzp-gf",
				httpRequest: `GET /Consent?patient=Patient/1000&scope=http://terminology.hl7.org/CodeSystem/consentscope|treatment&category=http://snomed.info/sct|129125009`,
				decision:    true,
			},
			{
				name:        "allow - Consent search with patient, scope, category and include",
				scope:       "pzp-gf",
				httpRequest: `GET /Consent?patient=Patient/1000&scope=http://terminology.hl7.org/CodeSystem/consentscope|treatment&category=http://snomed.info/sct|129125009&_include=Consent:actor`,
				decision:    true,
			},
			{
				name:        "deny - Consent search with multiple patient refs",
				scope:       "pzp-gf",
				httpRequest: `GET /Consent?patient=Patient/1000,Patient/1001&_profile=http://nictiz.nl/fhir/StructureDefinition/nl-core-TreatmentDirective2`,
				decision:    false,
				error:       "invalid request: patient_id: expected 1 value in patient parameter, found multiple",
			},
			{
				name:        "deny - Consent search without patient parameter",
				scope:       "pzp-gf",
				httpRequest: `GET /Consent?_profile=http://example.com/fhir/StructureDefinition/consent-profile`,
				decision:    false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:        "deny - Consent search without _profile parameter",
				scope:       "pzp-gf",
				httpRequest: `GET /Consent?patient=Patient/1000`,
				decision:    false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:        "deny - Consent search with empty patient parameter",
				scope:       "pzp-gf",
				httpRequest: `GET /Consent?patient=&_profile=http://example.com/fhir/StructureDefinition/consent-profile`,
				decision:    false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:        "deny - Patient search without patient_id or patient_bsn",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?`,
				decision:    false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:        "deny - Mitz consent check failure",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|bsn:error`,
				decision:    false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeInternalError, TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:        "deny - Mitz consent not given",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|bsn:deny`,
				decision:    false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
			},
			{
				name:        "deny - unsupported resource type",
				scope:       "pzp-gf",
				httpRequest: `GET /Observation?patient=Patient/1000`,
				decision:    false,
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
	t.Run("test_search_params policy - blocked at handler", func(t *testing.T) {
		// test_ prefixed scopes must be rejected by the HTTP handler with 400
		body, _ := json.Marshal(APIRequest{
			Input: APIInput{
				Subject: APISubject{Scope: "test_search_params"},
				Request: HTTPRequest{Method: "GET", Path: "/Observation", Query: "category=a,b&patient=Patient%2F1000"},
				Context: APIContext{ConnectionTypeCode: "hl7-fhir-rest"},
			},
		})
		req := httptest.NewRequest("POST", "/pdp", bytes.NewReader(body))
		w := httptest.NewRecorder()
		service.HandleMainPolicy(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		var response APIResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.False(t, response.Allow)
		assert.Contains(t, response.Error, "policy not allowed")
	})
	t.Run("test_search_params policy - search param AND/OR via evalRegoPolicy", func(t *testing.T) {
		type testCase struct {
			name     string
			query    string
			expected bool
		}
		testCases := []testCase{
			{
				name:     "allow - OR: category=a,b (comma-separated, single param)",
				query:    "category=a,b&patient=Patient%2F1000",
				expected: true,
			},
			{
				name:     "allow - AND: category=1&category=2 (repeated param)",
				query:    "category=1&category=2&patient=Patient%2F1000",
				expected: true,
			},
			{
				name:     "allow - AND of ORs: category=a,b&category=1",
				query:    "category=a,b&category=1&patient=Patient%2F1000",
				expected: true,
			},
			{
				name:     "deny - wrong OR order does not match AND rule: category=1&category=a,b",
				query:    "category=1&category=a,b&patient=Patient%2F1000",
				expected: false,
			},
			{
				name:     "deny - only one of the two AND values present",
				query:    "category=1&patient=Patient%2F1000",
				expected: false,
			},
			{
				name:     "deny - OR values in wrong order",
				query:    "category=b,a&patient=Patient%2F1000",
				expected: false,
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				apiRequest := APIRequest{
					Input: APIInput{
						Subject: APISubject{Scope: "test_search_params"},
						Request: HTTPRequest{Method: "GET", Path: "/Observation", Query: tc.query},
						Context: APIContext{ConnectionTypeCode: "hl7-fhir-rest"},
					},
				}
				policyInput, err := NewPolicyInput(apiRequest)
				require.NoError(t, err)
				result, err := service.evalRegoPolicy(t.Context(), "test_search_params", *policyInput)
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result.Allow)
			})
		}
	})
	t.Run("medicatieoverdracht", func(t *testing.T) {
		testCases := []testCase{
			{
				name:        "allow - MedicationRequest with correct category and _include",
				scope:       "medicatieoverdracht",
				httpRequest: `GET /MedicationRequest?category=http://snomed.info/sct|422037009&_include=MedicationRequest:medication&patient=Patient/1000`,
				decision:    true,
				properties: map[string]any{
					"patient_enrollment_identifier": "http://fhir.nl/fhir/NamingSystem/bsn|123456789",
				},
			},
			{
				name:        "deny - List search",
				scope:       "medicatieoverdracht",
				httpRequest: `GET /List?patient=Patient/1000`,
				decision:    false,
				policyReasonCodes: map[string][]TypeResultCode{
					"medicatieoverdracht": {TypeResultCodeNotAllowed, TypeResultCodeInformational},
				},
				properties: map[string]any{
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
