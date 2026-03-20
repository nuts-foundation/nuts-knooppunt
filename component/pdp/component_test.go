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
			fhir.Composition{
				Id: to.Ptr("comp-1"),
			},
			fhir.Consent{
				Id:     to.Ptr("consent-1"),
				Status: fhir.ConsentStateActive,
				Scope: fhir.CodeableConcept{
					Coding: []fhir.Coding{
						{Code: to.Ptr("eoverdracht")},
					},
				},
				Organization: []fhir.Reference{
					{Identifier: &fhir.Identifier{
						System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
						Value:  to.Ptr("00000002"),
					}},
				},
				Provision: &fhir.ConsentProvision{
					Type: to.Ptr(fhir.ConsentProvisionTypePermit),
					Actor: []fhir.ConsentProvisionActor{
						{Reference: fhir.Reference{
							Identifier: &fhir.Identifier{
								System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
								Value:  to.Ptr("00000001"),
							},
						}},
					},
					Data: []fhir.ConsentProvisionData{
						{Reference: fhir.Reference{Reference: to.Ptr("Task/task-1")}},
						{Reference: fhir.Reference{Reference: to.Ptr("Composition/comp-1")}},
					},
					Action: []fhir.CodeableConcept{
						{Coding: []fhir.Coding{{
							System: to.Ptr("http://terminology.hl7.org/CodeSystem/consentaction"),
							Code:   to.Ptr("access"),
						}}},
					},
				},
			},
		},
	}

	service, err := New(Config{
		Enabled: true,
		PIP:     PIPConfig{ResourceContentEnabled: true},
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

	// Policy-specific allow/deny rules are tested in OPA policy tests (*_test.rego).
	// These integration tests focus on the Go handler pipeline: multi-policy evaluation,
	// PIP enrichment, Mitz integration, request parsing, and input validation.
	t.Run("multi-policy evaluation", func(t *testing.T) {
		testCases := []testCase{
			{
				name:        "first policy denies but second allows",
				scope:       "mcsd_update bgz",
				httpRequest: `GET /Organization`,
				decision:    true,
				policyAllow: map[string]bool{
					"mcsd_update": true,
					"bgz":         false,
				},
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				runTest(t, tc)
			})
		}
	})
	t.Run("mitz integration", func(t *testing.T) {
		testCases := []testCase{
			{
				name:        "deny - Mitz consent not given",
				scope:       "bgz",
				httpRequest: `GET /Patient?_include=Patient:general-practitioner&_id=1001`,
				decision:    false,
			},
			{
				name:        "deny - Mitz consent check failure",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn|bsn:error`,
				decision:    false,
				policyReasonCodes: map[string][]TypeResultCode{
					"pzp_gf": {TypeResultCodeInternalError},
				},
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				runTest(t, tc)
			})
		}
	})
	t.Run("request parsing", func(t *testing.T) {
		testCases := []testCase{
			{
				name:        "encoded query parameter is decoded",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?identifier=http://fhir.nl/fhir/NamingSystem/bsn%7C123456789`,
				decision:    true,
			},
			{
				name:        "invalid identifier format",
				scope:       "pzp-gf",
				httpRequest: `GET /Patient?identifier=123456789`,
				decision:    false,
				error:       "invalid request: patient_bsn: expected identifier parameter in format 'system|value'",
			},
			{
				name:        "multiple patient refs rejected",
				scope:       "pzp-gf",
				httpRequest: `GET /Consent?patient=Patient/1000,Patient/1001&_profile=http://nictiz.nl/fhir/StructureDefinition/nl-core-TreatmentDirective2`,
				decision:    false,
				error:       "invalid request: patient_id: expected 1 value in patient parameter, found multiple",
			},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				runTest(t, tc)
			})
		}
	})
	t.Run("search_params_test_policy", func(t *testing.T) {
		testCases := []testCase{
			{
				name:        "allow - OR: category=a,b (comma-separated, single param)",
				scope:       "search_params_test_policy",
				httpRequest: `GET /Observation?category=a,b&patient=Patient/1000`,
				decision:    true,
			},
			{
				name:        "allow - AND: category=1&category=2 (repeated param)",
				scope:       "search_params_test_policy",
				httpRequest: `GET /Observation?category=1&category=2&patient=Patient/1000`,
				decision:    true,
			},
			{
				name:        "allow - AND of ORs: category=a,b&category=1",
				scope:       "search_params_test_policy",
				httpRequest: `GET /Observation?category=a,b&category=1&patient=Patient/1000`,
				decision:    true,
			},
			{
				name:        "deny - wrong OR order does not match AND rule: category=1&category=a,b",
				scope:       "search_params_test_policy",
				httpRequest: `GET /Observation?category=1&category=a,b&patient=Patient/1000`,
				decision:    false,
			},
			{
				name:        "deny - only one of the two AND values present",
				scope:       "search_params_test_policy",
				httpRequest: `GET /Observation?category=1&patient=Patient/1000`,
				decision:    false,
			},
			{
				name:        "deny - OR values in wrong order",
				scope:       "search_params_test_policy",
				httpRequest: `GET /Observation?category=b,a&patient=Patient/1000`,
				decision:    false,
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
				name:        "BSN enriched from PIP allows bgz request",
				scope:       "bgz",
				httpRequest: `GET /Patient?_include=Patient:general-practitioner&_id=1000`,
				decision:    true,
			},
			{
				name:        "OtherProps flow through to policy input",
				scope:       "medicatieoverdracht",
				httpRequest: `GET /MedicationRequest?category=http://snomed.info/sct|422037009&_include=MedicationRequest:medication&patient=Patient/1000`,
				decision:    true,
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
