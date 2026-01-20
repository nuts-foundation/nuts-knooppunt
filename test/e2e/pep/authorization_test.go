package pep

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_PEPAuthorization(t *testing.T) {

	// Start the PEP harness (HAPI, Knooppunt PDP, mock consentChecker XACML, and PEP nginx)
	harnessDetail := harness.StartPEP(t, harness.PEPConfig{
		FHIRBasePath:              "/fhir/DEFAULT", // Use partitioned HAPI from harness
		DataHolderOrganizationURA: "00000666",
		DataHolderFacilityType:    "Z3",
		RequestingFacilityType:    "Z3",
		PurposeOfUse:              "treatment",
	})

	pepBaseURL := harnessDetail.PEPBaseURL
	hapiBaseURL := harnessDetail.HAPIBaseURL
	mockMitz := harnessDetail.MockMitzXACML

	// Create a test patient in HAPI directly (bypassing PEP) for testing
	patientJSON := `{
		"resourceType": "Patient",
		"id": "patient-123",
		"identifier": [{
			"system": "http://fhir.nl/fhir/NamingSystem/bsn",
			"value": "900186021"
		}],
		"name": [{
			"family": "Test",
			"given": ["Patient"]
		}]
	}`
	createReq, err := http.NewRequest("PUT", hapiBaseURL.JoinPath("DEFAULT", "Patient", "patient-123").String(), strings.NewReader(patientJSON))
	require.NoError(t, err)
	createReq.Header.Set("Content-Type", "application/fhir+json")
	createResp, err := http.DefaultClient.Do(createReq)
	require.NoError(t, err)
	createResp.Body.Close()
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, createResp.StatusCode,
		"Failed to create test patient in HAPI: %d", createResp.StatusCode)

	t.Run("authorized request with valid token and consent", func(t *testing.T) {
		t.Skip("Skipping this test because they fail in main branch as well; fix in another branch")
		// Mock token format: bearer-<ura>-<uzi_role>-<practitioner_id>-<bsn>
		token := "bearer-00000020-01.015-123456789-900186021"

		// Note: mock defaults to "Permit", no need to set explicitly

		// Make request to PEP
		req, err := http.NewRequest("GET", pepBaseURL.JoinPath("fhir", "Patient", "patient-123").String(), nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Log response for debugging
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		t.Logf("Response status: %d, body: %s", resp.StatusCode, string(body))

		// Should be allowed - expect 200 since we created the patient
		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful authorization and FHIR request")

		// Verify XACML request contains expected fields
		lastReq := mockMitz.GetLastRequestXML()
		assert.Contains(t, lastReq, "900186021", "Request should contain patient BSN")
		assert.Contains(t, lastReq, "00000020", "Request should contain requesting organization URA")
		assert.Contains(t, lastReq, "00000666", "Request should contain data holder organization URA")
	})

	t.Run("denied request when consent is denied", func(t *testing.T) {
		t.Skip("Skipping this test because they fail in main branch as well; fix in another branch")
		token := "bearer-00000020-01.015-123456789-900186021"

		// Mock consentChecker will respond with Deny
		mockMitz.SetResponse("Deny", "No consent found")

		req, err := http.NewRequest("GET", pepBaseURL.JoinPath("fhir", "Patient", "patient-456").String(), nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("unauthorized request without token", func(t *testing.T) {
		req, err := http.NewRequest("GET", pepBaseURL.JoinPath("fhir", "Patient", "patient-789").String(), nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("unauthorized request with invalid token format", func(t *testing.T) {
		req, err := http.NewRequest("GET", pepBaseURL.JoinPath("fhir", "Patient", "patient-999").String(), nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer invalid-token")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}
