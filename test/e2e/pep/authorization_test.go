package pep

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

	createTestPatient(t, pep.HAPIBaseURL, "patient-123", "900186021")

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

	// Regression for #492: POST _search with form-encoded body must be authorized
	// based on parameters in the body (not the empty query string) and the body
	// must survive nginx's internalRedirect to the FHIR backend. HAPI FHIR clients
	// send Content-Type with a charset suffix, so that variant is asserted here.
	//
	// The HAPI container is reused across go test runs, so the patient/medication
	// are seeded with per-run unique IDs (same BSN, so the enrollment credential
	// still matches) and cleaned up in t.Cleanup. Filtering by the unique patient
	// reference isolates the assertion from any stale data from prior runs.
	t.Run("authorized POST _search with form-encoded body and charset", func(t *testing.T) {
		pep.MockMitz.SetResponse("Permit", "Consent granted")

		suffix := randomSuffix(t)
		patientID := "pep-post-search-patient-" + suffix
		medID := "pep-post-search-med-" + suffix

		createTestPatient(t, pep.HAPIBaseURL, patientID, "900186021")
		createTestMedicationRequest(t, pep.HAPIBaseURL, medID, patientID)
		t.Cleanup(func() {
			deleteHAPIResource(t, pep.HAPIBaseURL, "MedicationRequest", medID)
			deleteHAPIResource(t, pep.HAPIBaseURL, "Patient", patientID)
		})

		form := url.Values{}
		form.Set("patient", "Patient/"+patientID)

		targetURL := pepBaseURL.JoinPath("fhir", "MedicationRequest", "_search").String()
		req, err := http.NewRequest("POST", targetURL, strings.NewReader(form.Encode()))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+bearerToken.AccessToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, resp.StatusCode,
			"POST _search must be allowed: PDP needs to see the body-derived patient parameter. Response: %s", string(body))
		assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "application/fhir+json"),
			"Response Content-Type should be FHIR JSON, got %q", resp.Header.Get("Content-Type"))

		var bundle struct {
			ResourceType string `json:"resourceType"`
			Entry        []struct {
				Resource struct {
					ResourceType string `json:"resourceType"`
					ID           string `json:"id"`
				} `json:"resource"`
			} `json:"entry"`
		}
		require.NoError(t, json.Unmarshal(body, &bundle), "response must be a FHIR Bundle: %s", string(body))
		assert.Equal(t, "Bundle", bundle.ResourceType)

		// Filter is on a per-run unique patient reference. Body preservation through
		// internalRedirect is the only way HAPI can narrow to exactly this one entry;
		// an empty or dropped body would yield either an error or an unrelated page.
		require.Len(t, bundle.Entry, 1, "expected exactly 1 MedicationRequest, got %d: %s", len(bundle.Entry), string(body))
		assert.Equal(t, "MedicationRequest", bundle.Entry[0].Resource.ResourceType)
		assert.Equal(t, medID, bundle.Entry[0].Resource.ID)
	})

	t.Run("denied POST _search when body is empty (no patient derivable)", func(t *testing.T) {
		// Mitz is irrelevant here: the policy must already deny on missing patient context.
		// This locks in fail-closed behavior on the body-derived path.
		pep.MockMitz.SetResponse("Permit", "Consent granted")

		targetURL := pepBaseURL.JoinPath("fhir", "MedicationRequest", "_search").String()
		req, err := http.NewRequest("POST", targetURL, strings.NewReader(""))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+bearerToken.AccessToken)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusForbidden, resp.StatusCode,
			"empty body means no patient context; policy must deny")
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

// createTestPatient creates (or replaces) a Patient in HAPI. If bsn is empty,
// the identifier block is omitted so the helper can also seed a bare patient
// whose BSN is irrelevant to the test.
func createTestPatient(t *testing.T, hapiURL *url.URL, id, bsn string) {
	t.Helper()

	identifierBlock := ""
	if bsn != "" {
		identifierBlock = fmt.Sprintf(`,
		"identifier": [{
			"system": "http://fhir.nl/fhir/NamingSystem/bsn",
			"value": %q
		}]`, bsn)
	}

	patientJSON := fmt.Sprintf(`{
		"resourceType": "Patient",
		"id": %q%s,
		"name": [{
			"family": "Test",
			"given": ["Patient"]
		}]
	}`, id, identifierBlock)

	req, err := http.NewRequest("PUT", hapiURL.JoinPath("DEFAULT", "Patient", id).String(), strings.NewReader(patientJSON))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/fhir+json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode)
}

func createTestMedicationRequest(t *testing.T, hapiURL *url.URL, id, patientID string) {
	t.Helper()

	medJSON := fmt.Sprintf(`{
		"resourceType": "MedicationRequest",
		"id": %q,
		"status": "active",
		"intent": "order",
		"medicationCodeableConcept": {"text": "test medication"},
		"subject": {"reference": "Patient/%s"}
	}`, id, patientID)

	req, err := http.NewRequest("PUT", hapiURL.JoinPath("DEFAULT", "MedicationRequest", id).String(), strings.NewReader(medJSON))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/fhir+json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, resp.StatusCode)
}

func deleteHAPIResource(t *testing.T, hapiURL *url.URL, resourceType, id string) {
	t.Helper()

	req, err := http.NewRequest("DELETE", hapiURL.JoinPath("DEFAULT", resourceType, id).String(), nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	// HAPI returns 200 or 204 on success, 404 if already deleted — all acceptable for cleanup.
	require.Contains(t, []int{http.StatusOK, http.StatusNoContent, http.StatusNotFound}, resp.StatusCode)
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	var b [6]byte
	_, err := rand.Read(b[:])
	require.NoError(t, err)
	return hex.EncodeToString(b[:])
}
