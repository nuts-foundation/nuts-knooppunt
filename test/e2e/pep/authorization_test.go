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
// credential validation.
//
// This test validates the FULL credential flow:
//   - X509Credential issued via go-didx509-toolkit from test certificates
//   - NutsEmployeeCredential and HealthcareProviderRoleTypeCredential (self-attested)
//   - Real Presentation Definition validation
//   - Real token introspection with extracted claims
//   - PEP authorization through Knooppunt PDP
//   - Mitz consent checking (mocked)
func Test_PEPAuthorization(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	// Check if Docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker not available, skipping e2e test")
	}

	// Get absolute paths to test resources
	certsDir, err := filepath.Abs("certs")
	require.NoError(t, err)
	testdataDir, err := filepath.Abs("testdata")
	require.NoError(t, err)

	// Verify certificate files exist
	chainPath := filepath.Join(certsDir, "requester-chain.pem")
	keyPath := filepath.Join(certsDir, "requester.key")
	if _, err := os.Stat(chainPath); os.IsNotExist(err) {
		t.Fatalf("Certificate chain not found. Run: cd certs && ./generate-root-ca.sh && ./issue-cert.sh requester 'Test Hospital B.V.' Amsterdam 0 87654321 0")
	}

	// Start the PEP test harness (Knooppunt with Nuts + PDP + mock MITZ + HAPI)
	pep := harness.StartPEP(t, harness.PEPTestConfig{
		CertsDir:    certsDir,
		TestDataDir: testdataDir,
	})
	t.Logf("Knooppunt started at: %s (with embedded Nuts node at /nuts)", pep.KnooppuntURL)

	// Create a test patient in HAPI
	createTestPatient(t, pep.HAPIBaseURL)

	// Create subject (DID) in embedded Nuts node
	subjectName := "requester"
	subjectDID := createSubject(t, pep.NutsAPI, subjectName)
	t.Logf("Created subject DID: %s", subjectDID)

	// Issue X509Credential using go-didx509-toolkit
	x509Credential := issueX509Credential(t, chainPath, keyPath, subjectDID)
	t.Logf("X509Credential issued (first 100 chars): %s...", x509Credential[:min(100, len(x509Credential))])

	// Store X509Credential in wallet
	storeCredential(t, pep.NutsAPI, subjectName, x509Credential)
	t.Log("X509Credential stored in wallet")

	// Register on Discovery Service (before requesting token)
	registerOnDiscovery(t, pep.NutsAPI, subjectName)
	t.Log("Registered on Discovery Service")

	// Start PEP container pointing to Knooppunt (which provides both PDP and Nuts APIs)
	pepConfig := harness.PEPConfig{
		FHIRBackendHost:           "host.docker.internal",
		FHIRBackendPort:           pep.HAPIBaseURL.Port(),
		FHIRBasePath:              "/fhir",         // incoming path clients use
		FHIRUpstreamPath:          "/fhir/DEFAULT", // HAPI multi-tenant partition
		KnooppuntPDPHost:          "host.docker.internal",
		KnooppuntPDPPort:          pep.KnooppuntURL.Port(),
		NutsNodeHost:              "host.docker.internal",
		NutsNodePort:              pep.KnooppuntURL.Port(), // Nuts APIs are at /nuts on Knooppunt
		DataHolderOrganizationURA: "12345678",
		DataHolderFacilityType:    "Z3",
	}
	pepResult := harness.StartPEPContainer(t, pepConfig)
	pepBaseURL := pepResult.URL
	// Add cleanup to print logs on failure
	t.Cleanup(func() {
		logs, _ := pepResult.Container.Logs(t.Context())
		if logs != nil {
			logBytes, _ := io.ReadAll(logs)
			t.Logf("PEP container logs:\n%s", string(logBytes))
			logs.Close()
		}
	})
	t.Logf("PEP started at: %s", pepBaseURL)

	// Request both Bearer and DPoP tokens to test both paths
	// The authorization server URL uses the public Nuts URL (the node talks to itself)
	authServer := pep.NutsPublicURL.JoinPath("oauth2", subjectName).String()

	// Get Bearer token for some tests
	bearerToken := requestAccessToken(t, pep.NutsAPI, subjectName, authServer, "bgz", "Bearer")
	t.Logf("Bearer token obtained: %s...", bearerToken.AccessToken[:min(50, len(bearerToken.AccessToken))])
	assert.Equal(t, "Bearer", bearerToken.TokenType)

	// Get DPoP token for DPoP tests
	dpopToken := requestAccessToken(t, pep.NutsAPI, subjectName, authServer, "bgz", "DPoP")
	t.Logf("DPoP token obtained: %s...", dpopToken.AccessToken[:min(50, len(dpopToken.AccessToken))])
	t.Logf("DPoP key ID: %s", dpopToken.DPoPKID)
	assert.Equal(t, "DPoP", dpopToken.TokenType)
	assert.NotEmpty(t, dpopToken.DPoPKID, "DPoP token should have dpop_kid")

	// Introspect DPoP token to verify claims are extracted
	introspection := introspectToken(t, pep.NutsAPI, dpopToken.AccessToken)
	t.Logf("Introspection response: %+v", introspection)

	// Verify the introspection contains expected claims (using exact PDP field names from PD)
	assert.True(t, introspection["active"].(bool), "Token should be active")
	// From X509Credential
	assert.NotEmpty(t, introspection["subject_organization_id"], "subject_organization_id claim should be present")
	assert.NotEmpty(t, introspection["subject_organization"], "subject_organization claim should be present")
	// From NutsEmployeeCredential
	assert.NotEmpty(t, introspection["subject_id"], "subject_id claim should be present")
	assert.NotEmpty(t, introspection["subject_role"], "subject_role claim should be present")
	// From HealthcareProviderRoleTypeCredential
	assert.NotEmpty(t, introspection["subject_facility_type"], "subject_facility_type claim should be present")

	// Verify DPoP token has cnf claim (proof-of-possession binding)
	assert.NotNil(t, introspection["cnf"], "DPoP token should have cnf claim")

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

		targetURL := pepBaseURL.JoinPath("fhir", "Condition").String() + "?patient=Patient/patient-123"
		req, err := http.NewRequest("GET", targetURL, nil)
		require.NoError(t, err)

		req.Header.Set("Authorization", "Bearer "+bearerToken.AccessToken)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected successful authorization with Bearer token")
		assert.Contains(t, string(body), "Bundle")
	})

	t.Run("authorized request with DPoP token", func(t *testing.T) {
		pep.MockMitz.SetResponse("Permit", "Consent granted")

		// The PEP constructs the DPoP validation URL with https:// scheme (assuming TLS in production)
		// So the proof must use https:// even though we're testing over http://
		targetPath := "/fhir/Condition?patient=Patient/patient-123"
		dpopURL := "https://" + pepBaseURL.Host + targetPath
		t.Logf("DPoP proof URL: %s", dpopURL)

		// Create DPoP proof using embedded Nuts node (with the same key that bound the token)
		dpopProof := createDPoPProof(t, pep.NutsAPI, dpopToken.DPoPKID, "GET", dpopURL, dpopToken.AccessToken)
		t.Logf("DPoP proof created: %s...", dpopProof[:min(50, len(dpopProof))])

		targetURL := pepBaseURL.JoinPath("fhir", "Condition").String() + "?patient=Patient/patient-123"
		req, err := http.NewRequest("GET", targetURL, nil)
		require.NoError(t, err)

		// Use DPoP authorization scheme with proof header
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

		targetURL := pepBaseURL.JoinPath("fhir", "Condition").String() + "?patient=Patient/patient-123"
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

	employeeCredential := map[string]any{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
			"https://nuts.nl/credentials/v1",
		},
		"type": []string{"VerifiableCredential", "NutsEmployeeCredential"},
		"credentialSubject": map[string]any{
			"identifier": "urn:oid:2.16.528.1.1007.3.1.12345",
			"name":       "Dr. Jan de Vries",
			"roleName":   "Medisch Specialist",
		},
	}

	providerTypeCredential := map[string]any{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
		},
		"type": []string{"VerifiableCredential", "HealthcareProviderRoleTypeCredential"},
		"credentialSubject": map[string]any{
			"roleCodeNL": "Z3",
		},
	}

	tokenEndpoint := nutsAPI("/internal/auth/v2/" + subject + "/request-service-access-token")
	reqBody := map[string]any{
		"authorization_server": authServer,
		"scope":                scope,
		"credentials":          []any{employeeCredential, providerTypeCredential},
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
