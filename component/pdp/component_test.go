package pdp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	t.Run("disallow - Mitz consent not given", func(t *testing.T) {
		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						ClientQualifications:  []string{"bgz"},
						SubjectOrganizationId: "00000001",
						SubjectFacilityType:   "TODO",
						SubjectRole:           "TODO",
						SubjectId:             "TODO",
					},
				},
				Request: HTTPRequest{
					Method:   "GET",
					Protocol: "HTTP/1.1",
					Path:     "/Patient",
					QueryParams: map[string][]string{
						"_include": {"Patient:general-practitioner"},
						"_id":      {"1001"},
					},
				},
				Context: PDPContext{
					DataHolderOrganizationId: "00000002",
					DataHolderFacilityType:   "TODO",
				},
			},
		}

		response := executePDPRequest(t, service, pdpRequest)

		assert.False(t, response.Result.Allow)
		assert.NotEmpty(t, response.Result.Reasons)
	})
	t.Run("allow - correct Patient query with _include", func(t *testing.T) {
		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						ClientQualifications:  []string{"bgz"},
						SubjectOrganizationId: "00000001",
						SubjectFacilityType:   "TODO",
						SubjectRole:           "TODO",
						SubjectId:             "TODO",
					},
				},
				Request: HTTPRequest{
					Method:   "GET",
					Protocol: "HTTP/1.1",
					Path:     "/Patient",
					QueryParams: map[string][]string{
						"_include": {"Patient:general-practitioner"},
						"_id":      {"1000"},
					},
				},
				Context: PDPContext{
					DataHolderOrganizationId: "00000002",
					DataHolderFacilityType:   "TODO",
				},
			},
		}

		response := executePDPRequest(t, service, pdpRequest)

		assert.True(t, response.Result.Allow, "bgz should allow Patient query with _include=Patient:general-practitioner")
		assert.Empty(t, response.Result.Reasons)
	})
	t.Run("allow - correct MedicationDispense query with category and _include", func(t *testing.T) {
		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						ClientQualifications:  []string{"bgz"},
						SubjectOrganizationId: "00000001",
						SubjectFacilityType:   "TODO",
						SubjectRole:           "TODO",
						SubjectId:             "TODO",
					},
				},
				Request: HTTPRequest{
					Method:   "GET",
					Protocol: "HTTP/1.1",
					Path:     "/MedicationDispense",
					QueryParams: map[string][]string{
						"category": {"http://snomed.info/sct|422037009"},
						"_include": {"MedicationDispense:medication"},
						"patient":  {"Patient/1000"},
					},
				},
				Context: PDPContext{
					DataHolderOrganizationId: "00000002",
					DataHolderFacilityType:   "TODO",
				},
			},
		}

		response := executePDPRequest(t, service, pdpRequest)

		assert.True(t, response.Result.Allow, "bgz should allow MedicationDispense query with category and _include=MedicationDispense:medication")
		assert.Empty(t, response.Result.Reasons)
	})

	t.Run("deny - Patient query with wrong _include parameter", func(t *testing.T) {
		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						ClientQualifications:  []string{"bgz"},
						SubjectOrganizationId: "00000001",
					},
				},
				Request: HTTPRequest{
					Method:   "GET",
					Protocol: "HTTP/1.1",
					Path:     "/Patient?",
					QueryParams: map[string][]string{
						"_include": {"Patient:organization"},
					},
				},
				Context: PDPContext{
					DataHolderOrganizationId: "00000002",
				},
			},
		}

		response := executePDPRequest(t, service, pdpRequest)

		assert.False(t, response.Result.Allow, "bgz should deny Patient query with wrong _include parameter")
	})

	t.Run("deny - Patient query with additional parameters", func(t *testing.T) {
		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						ClientQualifications:  []string{"bgz"},
						SubjectOrganizationId: "00000001",
					},
				},
				Request: HTTPRequest{
					Method:   "GET",
					Protocol: "HTTP/1.1",
					Path:     "/Patient?",
					QueryParams: map[string][]string{
						"_include": {"Patient:general-practitioner"},
						"name":     {"John"},
					},
				},
				Context: PDPContext{
					DataHolderOrganizationId: "00000002",
				},
			},
		}

		response := executePDPRequest(t, service, pdpRequest)

		assert.False(t, response.Result.Allow, "bgz should deny Patient query with additional parameters")
	})
}
