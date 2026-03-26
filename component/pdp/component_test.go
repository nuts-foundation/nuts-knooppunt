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

func TestHandleMainPolicy_WithoutMitz(t *testing.T) {
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
		},
	}

	service, err := New(Config{Enabled: true}, nil)
	require.NoError(t, err)
	service.opaBundleBaseURL = httpServer.URL + "/pdp/bundles/"
	service.pipClient = pipClient

	service.RegisterHttpHandlers(nil, mux)

	require.NoError(t, service.Start())
	defer func() {
		require.NoError(t, service.Stop(context.Background()))
	}()

	t.Run("policy evaluation works without Mitz", func(t *testing.T) {
		response := executePDPRequest(t, service, APIRequest{
			Input: APIInput{
				Subject: APISubject{
					Scope:                    "mcsd_update",
					OrganizationUra:          "00000001",
					OrganizationFacilityType: "TODO",
					UserId:                   "TODO",
					UserRole:                 "TODO",
				},
				Request: HTTPRequest{
					Method:   "GET",
					Protocol: "HTTP/1.1",
					Path:     "/Organization",
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
		})
		assert.Empty(t, response.Error)
		mcsdPolicy, hasPolicy := response.Policies["mcsd_update"]
		require.True(t, hasPolicy, "expected mcsd_update policy to be evaluated")
		assert.True(t, mcsdPolicy.Allow, "expected mcsd_update policy to allow the request")
	})
}

func TestHandleMainPolicy_CaseInsensitivePolicyNames(t *testing.T) {
	mux := http.NewServeMux()
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	service, err := New(Config{Enabled: true}, nil)
	require.NoError(t, err)
	service.opaBundleBaseURL = httpServer.URL + "/pdp/bundles/"
	service.pipClient = &test.StubFHIRClient{}

	service.RegisterHttpHandlers(nil, mux)

	require.NoError(t, service.Start())
	defer func() {
		require.NoError(t, service.Stop(context.Background()))
	}()

	t.Run("mixed case scope resolves to correct policy", func(t *testing.T) {
		response := executePDPRequest(t, service, APIRequest{
			Input: APIInput{
				Subject: APISubject{
					Scope:                    "MCSD-Update",
					OrganizationUra:          "00000001",
					OrganizationFacilityType: "TODO",
					UserId:                   "TODO",
					UserRole:                 "TODO",
				},
				Request: HTTPRequest{
					Method:   "GET",
					Protocol: "HTTP/1.1",
					Path:     "/Organization",
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
		})
		assert.Empty(t, response.Error)
		mcsdPolicy, hasPolicy := response.Policies["mcsd_update"]
		require.True(t, hasPolicy, "expected mcsd_update policy to be evaluated")
		assert.True(t, mcsdPolicy.Allow, "expected mcsd_update policy to allow the request")
	})

	t.Run("duplicate scope with different casing is deduplicated", func(t *testing.T) {
		response := executePDPRequest(t, service, APIRequest{
			Input: APIInput{
				Subject: APISubject{
					Scope:                    "mcsd_update MCSD_UPDATE Mcsd_Update",
					OrganizationUra:          "00000001",
					OrganizationFacilityType: "TODO",
					UserId:                   "TODO",
					UserRole:                 "TODO",
				},
				Request: HTTPRequest{
					Method:   "GET",
					Protocol: "HTTP/1.1",
					Path:     "/Organization",
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
		})
		assert.Empty(t, response.Error)
		assert.Len(t, response.Policies, 1, "duplicate scopes with different casing should be deduplicated")
		mcsdPolicy, hasPolicy := response.Policies["mcsd_update"]
		require.True(t, hasPolicy, "expected mcsd_update policy to be evaluated")
		assert.True(t, mcsdPolicy.Allow)
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
			fhir.Task{
				Id:     to.Ptr("task-1"),
				Status: fhir.TaskStatusRequested,
				Owner: &fhir.Reference{
					Identifier: &fhir.Identifier{
						System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
						Value:  to.Ptr("00000001"),
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
					Method:      httpReqParts[0],
					Protocol:    "HTTP/1.1",
					Path:        httpReqURL.Path,
					QueryParams: httpReqURL.Query(),
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
	t.Run("eoverdracht_sender", func(t *testing.T) {
		testCases := []testCase{
			{
				name:        "allow - Task update fetches resource content from PIP",
				scope:       "eoverdracht-sender",
				httpRequest: `PUT /Task/task-1`,
				decision:    true,
			},
			{
				name:        "deny - Task read without local consent",
				scope:       "eoverdracht-sender",
				httpRequest: `GET /Task/task-1`,
				decision:    false,
			},
			{
				name:        "deny - Task delete",
				scope:       "eoverdracht-sender",
				httpRequest: `DELETE /Task/task-1`,
				decision:    false,
			},
			{
				name:        "allow - Task update with non-existent resource returns pip_error",
				scope:       "eoverdracht-sender",
				httpRequest: `PUT /Task/nonexistent`,
				decision:    true,
				policyReasonCodes: map[string][]TypeResultCode{
					"eoverdracht_sender": {TypeResultCodePIPError},
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
