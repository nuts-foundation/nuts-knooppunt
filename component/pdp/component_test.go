package pdp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
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
	opaBundleBaseURL = httpServer.URL + "/pdp/bundles/"

	// Mock Mitz; return Permit for all consent checks
	ctrl := gomock.NewController(t)
	consentChecker := mitz.NewMockConsentChecker(ctrl)
	consentChecker.EXPECT().
		CheckConsent(gomock.Any(), gomock.Any()).Return(&xacml.XACMLResponse{Decision: xacml.DecisionPermit}, nil).
		AnyTimes()

	service, err := New(Config{
		Enabled: true,
	}, consentChecker)
	require.NoError(t, err)

	service.RegisterHttpHandlers(nil, mux)

	require.NoError(t, service.Start())
	defer func() {
		require.NoError(t, service.Stop(context.Background()))
	}()

	t.Run("allow - correct Patient query with _include", func(t *testing.T) {
		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						ClientQualifications:  []string{"bgz_patient"},
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
					},
				},
				Context: PDPContext{
					DataHolderOrganizationId: "00000002",
					DataHolderFacilityType:   "TODO",
					PatientBSN:               "123456789",
				},
			},
		}

		response := executePDPRequest(t, service, pdpRequest)

		assert.True(t, response.Result.Allow, "bgz_patient should allow Patient query with _include=Patient:general-practitioner")
		assert.Empty(t, response.Result.Reasons)
	})
	t.Run("allow - correct MedicationDispense query with category and _include", func(t *testing.T) {
		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						ClientQualifications:  []string{"bgz_patient"},
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
					},
				},
				Context: PDPContext{
					DataHolderOrganizationId: "00000002",
					DataHolderFacilityType:   "TODO",
					PatientBSN:               "123456789",
				},
			},
		}

		response := executePDPRequest(t, service, pdpRequest)

		assert.True(t, response.Result.Allow, "bgz_patient should allow MedicationDispense query with category and _include=MedicationDispense:medication")
		assert.Empty(t, response.Result.Reasons)
	})

	t.Run("deny - Patient query with wrong _include parameter", func(t *testing.T) {
		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						ClientQualifications:  []string{"bgz_patient"},
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

		assert.False(t, response.Result.Allow, "bgz_patient should deny Patient query with wrong _include parameter")
	})

	t.Run("deny - Patient query with additional parameters", func(t *testing.T) {
		pdpRequest := PDPRequest{
			Input: PDPInput{
				Subject: Subject{
					Properties: SubjectProperties{
						ClientQualifications:  []string{"bgz_patient"},
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

		assert.False(t, response.Result.Allow, "bgz_patient should deny Patient query with additional parameters")
	})
}
