package pep

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test_PEPAuthorization tests the PEP authorization flow with real Nuts node
// credential validation using the medicatieoverdracht scope. The access policy
// in testdata/ is based on config/policies/medicatieoverdracht-policy.json with
// additional claim mappings for MITZ input validation (see accesspolicy.json).
//
// This test validates the FULL credential flow:
//   - X509Credential issued via go-didx509-toolkit from test certificates
//   - HealthCareProfessionalDelegationCredential and PatientEnrollmentCredential (self-attested, #406)
//   - Presentation Definition validation
//   - Real token introspection with extracted claims
//   - PEP authorization through Knooppunt PDP
//   - Mitz consent checking (mocked)
func Test_PEPAuthorization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker not available, skipping e2e test")
	}

	certsDir, err := filepath.Abs("certs")
	require.NoError(t, err)
	testdataDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	chainPath := filepath.Join(certsDir, "requester-chain.pem")
	keyPath := filepath.Join(certsDir, "requester.key")
	if _, err := os.Stat(chainPath); os.IsNotExist(err) {
		t.Fatalf("Certificate chain not found. Run: cd certs && ./generate-root-ca.sh && ./issue-cert.sh requester 'Test Hospital B.V.' Amsterdam 0 87654321 0")
	}

	pep := harness.StartPEP(t, harness.PEPTestConfig{
		CertsDir:    certsDir,
		TestDataDir: testdataDir,
	})

	createTestPatient(t, pep.HAPIBaseURL)

	subjectName := "requester"
	subjectDID := createSubject(t, pep.NutsAPI, subjectName)
	t.Logf("Created subject DID: %s", subjectDID)

	x509Credential := issueX509Credential(t, chainPath, keyPath, subjectDID)
	storeCredential(t, pep.NutsAPI, subjectName, x509Credential)
	registerOnDiscovery(t, pep.NutsAPI, subjectName)

	pepConfig := harness.PEPConfig{
		FHIRBackendHost:           "host.docker.internal",
		FHIRBackendPort:           pep.HAPIBaseURL.Port(),
		FHIRBasePath:              "/fhir",
		FHIRUpstreamPath:          "/fhir/DEFAULT",
		KnooppuntPDPHost:          "host.docker.internal",
		KnooppuntPDPPort:          pep.KnooppuntURL.Port(),
		NutsNodeHost:              "host.docker.internal",
		NutsNodePort:              pep.KnooppuntURL.Port(),
		DataHolderOrganizationURA: "12345678",
		DataHolderFacilityType:    "Z3",
	}
	pepResult := harness.StartPEPContainer(t, pepConfig)
	pepBaseURL := pepResult.URL
	t.Cleanup(func() {
		logs, _ := pepResult.Container.Logs(t.Context())
		if logs != nil {
			logBytes, _ := io.ReadAll(logs)
			t.Logf("PEP container logs:\n%s", string(logBytes))
			logs.Close()
		}
	})

	authServer := pep.NutsPublicURL.JoinPath("oauth2", subjectName).String()
	const scope = "medicatieoverdracht-gf"

	// Get Bearer token
	bearerToken := requestAccessToken(t, pep.NutsAPI, subjectName, authServer, scope, "Bearer")
	assert.Equal(t, "Bearer", bearerToken.TokenType)

	// Get DPoP token
	dpopToken := requestAccessToken(t, pep.NutsAPI, subjectName, authServer, scope, "DPoP")
	assert.Equal(t, "DPoP", dpopToken.TokenType)
	assert.NotEmpty(t, dpopToken.DPoPKID)

	// Introspect and verify all claims including PatientEnrollmentCredential (#406)
	introspection := introspectToken(t, pep.NutsAPI, dpopToken.AccessToken)
	t.Logf("Introspection response: %+v", introspection)

	assert.True(t, introspection["active"].(bool))
	// From X509Credential
	assert.NotEmpty(t, introspection["organization_ura"])
	assert.NotEmpty(t, introspection["organization_name"])
	// From HealthCareProfessionalDelegationCredential
	assert.NotEmpty(t, introspection["delegation_role_code"])
	assert.NotEmpty(t, introspection["delegation_registered_by"])
	// From PatientEnrollmentCredential (AORTA inschrijftoken equivalent)
	assert.Equal(t, "http://fhir.nl/fhir/NamingSystem/bsn|900186021",
		introspection["patient_enrollment_identifier"],
		"patient_enrollment_identifier should contain the patient BSN")
	assert.NotEmpty(t, introspection["patient_enrollment_registered_by"])
	// DPoP binding
	assert.NotNil(t, introspection["cnf"])

	t.Run("unauthorized request without token", func(t *testing.T) {
		req, err := http.NewRequest("GET", pepBaseURL.JoinPath("fhir", "Patient", "patient-123").String(), nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("authorized request with Bearer token", func(t *testing.T) {
		pep.MockMitz.SetResponse("Permit", "Consent granted")

		targetURL := pepBaseURL.JoinPath("fhir", "MedicationRequest").String() + "?patient=Patient/patient-123"
		req, err := http.NewRequest("GET", targetURL, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+bearerToken.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful authorization with Bearer token: %s", string(body))
		assert.Contains(t, string(body), "Bundle")
	})

	t.Run("authorized request with DPoP token", func(t *testing.T) {
		pep.MockMitz.SetResponse("Permit", "Consent granted")

		targetPath := "/fhir/MedicationRequest?patient=Patient/patient-123"
		dpopURL := "https://" + pepBaseURL.Host + targetPath

		dpopProof := createDPoPProof(t, pep.NutsAPI, dpopToken.DPoPKID, "GET", dpopURL, dpopToken.AccessToken)

		targetURL := pepBaseURL.JoinPath("fhir", "MedicationRequest").String() + "?patient=Patient/patient-123"
		req, err := http.NewRequest("GET", targetURL, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "DPoP "+dpopToken.AccessToken)
		req.Header.Set("DPoP", dpopProof)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful authorization with DPoP token: %s", string(body))
		assert.Contains(t, string(body), "Bundle")
	})

	t.Run("denied request when consent is denied", func(t *testing.T) {
		pep.MockMitz.SetResponse("Deny", "No consent found")

		targetURL := pepBaseURL.JoinPath("fhir", "MedicationRequest").String() + "?patient=Patient/patient-123"
		req, err := http.NewRequest("GET", targetURL, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+bearerToken.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

func createSubject(t *testing.T, nutsAPI func(string) string, subject string) string {
	t.Helper()
	reqBody := map[string]string{"subject": subject}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(
		nutsAPI("/internal/vdr/v2/subject"),
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to create subject: %s", string(respBody))

	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))

	documents := result["documents"].([]any)
	doc := documents[0].(map[string]any)
	return doc["id"].(string)
}

func issueX509Credential(t *testing.T, chainPath, keyPath, subjectDID string) string {
	t.Helper()

	cmd := exec.Command("docker", "run", "--rm",
		"-v", chainPath+":/cert-chain.pem:ro",
		"-v", keyPath+":/cert-key.key:ro",
		"nutsfoundation/go-didx509-toolkit:main",
		"vc", "/cert-chain.pem", "/cert-key.key", "CN=Fake UZI Root CA", subjectDID,
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go-didx509-toolkit failed: %s\nstderr: %s", err, string(exitErr.Stderr))
		}
		t.Fatalf("go-didx509-toolkit failed: %s", err)
	}

	return strings.TrimSpace(string(output))
}

func storeCredential(t *testing.T, nutsAPI func(string) string, holder, credential string) {
	t.Helper()

	body := []byte(`"` + credential + `"`)

	resp, err := http.Post(
		nutsAPI("/internal/vcr/v2/holder/"+holder+"/vc"),
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "Failed to store credential: %s", string(respBody))
}

func registerOnDiscovery(t *testing.T, nutsAPI func(string) string, subject string) {
	t.Helper()

	reqBody := map[string]any{
		"registrationParameters": map[string]string{
			"fhirBaseURL": "http://example.com/fhir",
		},
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(
		nutsAPI("/internal/discovery/v1/bgz-test/"+subject),
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to register on discovery: %s", string(respBody))
}

type tokenResult struct {
	AccessToken string
	DPoPKID     string
	TokenType   string
}

func requestAccessToken(t *testing.T, nutsAPI func(string) string, subject, authServer, scope, tokenType string) tokenResult {
	t.Helper()

	credentials := []any{
		// HealthCareProfessionalDelegationCredential: mandaat/delegation of the healthcare professional
		map[string]any{
			"@context": []string{"https://www.w3.org/2018/credentials/v1"},
			"type":     []string{"VerifiableCredential", "HealthCareProfessionalDelegationCredential"},
			"credentialSubject": map[string]any{
				"registeredBy": "urn:oid:2.16.528.1.1007.3.1.12345",
				"roleCode":     "01.015",
				"facilityType": "Z3",
			},
		},
		// PatientEnrollmentCredential: the VC equivalent of the AORTA inschrijftoken.
		// Passed per-request because the credential is patient-specific (#406).
		map[string]any{
			"@context": []string{"https://www.w3.org/2018/credentials/v1"},
			"type":     []string{"VerifiableCredential", "PatientEnrollmentCredential"},
			"credentialSubject": map[string]any{
				"patientId":    "http://fhir.nl/fhir/NamingSystem/bsn|900186021",
				"registeredBy": "urn:oid:2.16.528.1.1007.3.1.12345",
			},
		},
	}

	tokenEndpoint := nutsAPI("/internal/auth/v2/" + subject + "/request-service-access-token")
	reqBody := map[string]any{
		"authorization_server": authServer,
		"scope":                scope,
		"credentials":          credentials,
		"token_type":           tokenType,
	}
	body, _ := json.Marshal(reqBody)

	req, err := http.NewRequest("POST", tokenEndpoint, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to get access token: %s", string(respBody))

	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))

	tr := tokenResult{
		AccessToken: result["access_token"].(string),
		TokenType:   result["token_type"].(string),
	}
	if dpopKID, ok := result["dpop_kid"].(string); ok {
		tr.DPoPKID = dpopKID
	}
	return tr
}

func createDPoPProof(t *testing.T, nutsAPI func(string) string, kid, method, targetURL, accessToken string) string {
	t.Helper()

	encodedKID := url.QueryEscape(kid)
	dpopEndpoint := nutsAPI("/internal/auth/v2/dpop/" + encodedKID)

	reqBody := map[string]any{
		"htm":   method,
		"htu":   targetURL,
		"token": accessToken,
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(dpopEndpoint, "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to create DPoP proof: %s", string(respBody))

	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	return result["dpop"].(string)
}

func introspectToken(t *testing.T, nutsAPI func(string) string, token string) map[string]any {
	t.Helper()

	resp, err := http.PostForm(
		nutsAPI("/internal/auth/v2/accesstoken/introspect"),
		url.Values{"token": {token}},
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result
}

func createTestPatient(t *testing.T, hapiURL *url.URL) {
	t.Helper()

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

	req, err := http.NewRequest("PUT", hapiURL.JoinPath("DEFAULT", "Patient", "patient-123").String(), strings.NewReader(patientJSON))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/fhir+json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode)
}
