package pdp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	service, err := New(Config{
		Enabled: true,
	}, &mitz.Component{})
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
					},
				},
				Request: HTTPRequest{
					Method:   "GET",
					Protocol: "HTTP/1.1",
					Path:     "/Patient?",
					QueryParams: map[string][]string{
						"_include": {"Patient:general-practitioner"},
					},
				},
				Context: PDPContext{
					DataHolderOrganizationId: "00000002",
				},
			},
		}

		response := executePDPRequest(t, service, pdpRequest)

		assert.True(t, response.Result.Allow, "bgz_patient should allow Patient query with _include=Patient:general-practitioner")
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
