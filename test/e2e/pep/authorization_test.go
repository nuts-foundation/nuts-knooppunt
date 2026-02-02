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
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	httpComp "github.com/nuts-foundation/nuts-knooppunt/component/http"
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/nuts-foundation/nuts-knooppunt/test/mitzmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
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
	caPath := filepath.Join(certsDir, "ca.pem")
	if _, err := os.Stat(chainPath); os.IsNotExist(err) {
		t.Fatalf("Certificate chain not found. Run: cd certs && ./generate-root-ca.sh && ./issue-cert.sh requester 'Test Hospital B.V.' Amsterdam 0 87654321 0")
	}

	// Create mock XACML Mitz server (for consent checking)
	mockMitz := mitzmock.NewClosedQuestionService(t)

	// Start HAPI FHIR server
	hapiBaseURL := startHAPI(t)

	// Create a test patient in HAPI
	createTestPatient(t, hapiBaseURL)

	// Start real Nuts node with our test configuration
	nutsPorts := startNutsNode(t, certsDir, testdataDir, caPath)
	t.Logf("Nuts node started at internal: %s, public: %s", nutsPorts.InternalURL, nutsPorts.PublicURL)

	// Wait for Nuts node to be fully ready
	waitForNutsNode(t, nutsPorts.InternalURL)

	// Create subject (DID) in Nuts node
	subjectName := "requester"
	subjectDID := createSubject(t, nutsPorts.InternalURL, subjectName)
	t.Logf("Created subject DID: %s", subjectDID)

	// Issue X509Credential using go-didx509-toolkit
	x509Credential := issueX509Credential(t, chainPath, keyPath, subjectDID)
	t.Logf("X509Credential issued (first 100 chars): %s...", x509Credential[:min(100, len(x509Credential))])

	// Store X509Credential in wallet
	storeCredential(t, nutsPorts.InternalURL, subjectName, x509Credential)
	t.Log("X509Credential stored in wallet")

	// Register on Discovery Service (before requesting token)
	registerOnDiscovery(t, nutsPorts.InternalURL, subjectName)
	t.Log("Registered on Discovery Service")

	// Start Knooppunt PDP with real MITZ mock
	httpConfig := httpComp.TestConfig()
	knooppuntURL := startKnooppunt(t, httpConfig, hapiBaseURL, mockMitz)
	t.Logf("Knooppunt PDP started at: %s", knooppuntURL)

	// Start PEP container pointing to real Nuts node
	pepConfig := harness.PEPConfig{
		FHIRBackendHost:           "host.docker.internal",
		FHIRBackendPort:           hapiBaseURL.Port(),
		FHIRBasePath:              "/fhir",         // incoming path clients use
		FHIRUpstreamPath:          "/fhir/DEFAULT", // HAPI multi-tenant partition
		KnooppuntPDPHost:          "host.docker.internal",
		KnooppuntPDPPort:          knooppuntURL.Port(),
		NutsNodeHost:              "host.docker.internal",
		NutsNodePort:              nutsPorts.InternalURL.Port(),
		DataHolderOrganizationURA: "12345678",
		DataHolderFacilityType:    "Z3",
		// All claims come from credentials, DPoP validation is enabled
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
	// For self-referential token requests (same node as requester and auth server),
	// use the internal container URL since the node talks to itself
	authServer := "http://localhost:8080/oauth2/" + subjectName

	// Get Bearer token for some tests
	bearerToken := requestAccessToken(t, nutsPorts.InternalURL, subjectName, authServer, "bgz", "Bearer")
	t.Logf("Bearer token obtained: %s...", bearerToken.AccessToken[:min(50, len(bearerToken.AccessToken))])
	assert.Equal(t, "Bearer", bearerToken.TokenType)

	// Get DPoP token for DPoP tests
	dpopToken := requestAccessToken(t, nutsPorts.InternalURL, subjectName, authServer, "bgz", "DPoP")
	t.Logf("DPoP token obtained: %s...", dpopToken.AccessToken[:min(50, len(dpopToken.AccessToken))])
	t.Logf("DPoP key ID: %s", dpopToken.DPoPKID)
	assert.Equal(t, "DPoP", dpopToken.TokenType)
	assert.NotEmpty(t, dpopToken.DPoPKID, "DPoP token should have dpop_kid")

	// Introspect DPoP token to verify claims are extracted
	introspection := introspectToken(t, nutsPorts.InternalURL, dpopToken.AccessToken)
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
		mockMitz.SetResponse("Permit", "Consent granted")

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
		mockMitz.SetResponse("Permit", "Consent granted")

		// The PEP constructs the DPoP validation URL with https:// scheme (assuming TLS in production)
		// So the proof must use https:// even though we're testing over http://
		targetPath := "/fhir/Condition?patient=Patient/patient-123"
		dpopURL := "https://" + pepBaseURL.Host + targetPath
		t.Logf("DPoP proof URL: %s", dpopURL)

		// Create DPoP proof using Nuts node (with the same key that bound the token)
		dpopProof := createDPoPProof(t, nutsPorts.InternalURL, dpopToken.DPoPKID, "GET", dpopURL, dpopToken.AccessToken)
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
		mockMitz.SetResponse("Deny", "No consent found")

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

// startHAPI starts a HAPI FHIR server container for testing
func startHAPI(t *testing.T) *url.URL {
	t.Helper()
	ctx := t.Context()

	// Use the same image as the harness - it has DEFAULT partition pre-configured
	hapiReq := testcontainers.ContainerRequest{
		Image:        "ghcr.io/nuts-foundation/fake-nvi:latest",
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"hapi.fhir.fhir_version":                                    "R4",
			"hapi.fhir.partitioning.allow_references_across_partitions": "false",
			"hapi.fhir.server_id_strategy":                              "UUID",
			"hapi.fhir.client_id_strategy":                              "ANY",
			"hapi.fhir.store_meta_source_information":                   "SOURCE_URI",
			"hapi.fhir.delete_expunge_enabled":                          "true",
			"hapi.fhir.allow_multiple_delete":                           "true",
		},
		WaitingFor: wait.ForHTTP("/fhir/DEFAULT/Account").WithPort("8080").WithStartupTimeout(120 * time.Second),
	}

	hapiContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: hapiReq,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { hapiContainer.Terminate(ctx) })

	host, err := hapiContainer.Host(ctx)
	require.NoError(t, err)
	port, err := hapiContainer.MappedPort(ctx, "8080")
	require.NoError(t, err)

	return &url.URL{Scheme: "http", Host: host + ":" + port.Port(), Path: "/fhir"}
}

type nutsNodePorts struct {
	InternalURL *url.URL
	PublicURL   *url.URL
}

func startNutsNode(t *testing.T, certsDir, testdataDir, caPath string) nutsNodePorts {
	t.Helper()
	ctx := t.Context()

	// Use master tag which has did:x509 support for storing X509Credentials
	// TODO: Pin to a specific version once a release includes did:x509 support
	// Using master is not ideal as it can cause flaky tests when upstream changes
	nutsReq := testcontainers.ContainerRequest{
		Image:        "nutsfoundation/nuts-node:master",
		ExposedPorts: []string{"8080/tcp", "8081/tcp"},
		Env: map[string]string{
			"NUTS_URL":                             "http://localhost:8080",
			"NUTS_VERBOSITY":                       "debug",
			"NUTS_STRICTMODE":                      "false",
			"NUTS_INTERNALRATELIMITER":             "false",
			"NUTS_HTTP_PUBLIC_ADDRESS":             ":8080",
			"NUTS_HTTP_INTERNAL_ADDRESS":           ":8081",
			"NUTS_AUTH_CONTRACTVALIDATORS":         "dummy", // Use dummy for testing
			"NUTS_POLICY_DIRECTORY":                "/opt/nuts/policies",
			"NUTS_DISCOVERY_DEFINITIONS_DIRECTORY": "/opt/nuts/discovery",
			"NUTS_DISCOVERY_SERVER_IDS":            "bgz-test",
			"NUTS_VDR_DIDMETHODS":                  "web",
		},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      filepath.Join(testdataDir, "accesspolicy.json"),
				ContainerFilePath: "/opt/nuts/policies/accesspolicy.json",
				FileMode:          0644,
			},
			{
				HostFilePath:      filepath.Join(testdataDir, "discovery.json"),
				ContainerFilePath: "/opt/nuts/discovery/bgz-test.json",
				FileMode:          0644,
			},
			{
				HostFilePath:      caPath,
				ContainerFilePath: "/etc/ssl/certs/Fake_UZI_Root_CA.pem",
				FileMode:          0644,
			},
		},
		WaitingFor: wait.ForHTTP("/status").WithPort("8081").WithStartupTimeout(60 * time.Second),
	}

	nutsContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: nutsReq,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { nutsContainer.Terminate(ctx) })

	host, err := nutsContainer.Host(ctx)
	require.NoError(t, err)
	internalPort, err := nutsContainer.MappedPort(ctx, "8081")
	require.NoError(t, err)
	publicPort, err := nutsContainer.MappedPort(ctx, "8080")
	require.NoError(t, err)

	return nutsNodePorts{
		InternalURL: &url.URL{Scheme: "http", Host: host + ":" + internalPort.Port()},
		PublicURL:   &url.URL{Scheme: "http", Host: host + ":" + publicPort.Port()},
	}
}

func waitForNutsNode(t *testing.T, nutsURL *url.URL) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 30; i++ {
		resp, err := client.Get(nutsURL.JoinPath("/status").String())
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("Nuts node did not become ready in time")
}

func createSubject(t *testing.T, nutsURL *url.URL, subject string) string {
	t.Helper()
	reqBody := map[string]string{"subject": subject}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(
		nutsURL.JoinPath("/internal/vdr/v2/subject").String(),
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to create subject: %s", string(respBody))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	documents := result["documents"].([]interface{})
	doc := documents[0].(map[string]interface{})
	return doc["id"].(string)
}

func issueX509Credential(t *testing.T, chainPath, keyPath, subjectDID string) string {
	t.Helper()

	// Use go-didx509-toolkit Docker image to issue credential
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

func storeCredential(t *testing.T, nutsURL *url.URL, holder, credential string) {
	t.Helper()

	// Credential is a JWT string, wrap it in quotes for JSON
	body := []byte(`"` + credential + `"`)

	resp, err := http.Post(
		nutsURL.JoinPath("/internal/vcr/v2/holder", holder, "vc").String(),
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "Failed to store credential: %s", string(respBody))
}

func registerOnDiscovery(t *testing.T, nutsURL *url.URL, subject string) {
	t.Helper()

	reqBody := map[string]interface{}{
		"registrationParameters": map[string]string{
			"fhirBaseURL": "http://example.com/fhir",
		},
	}
	body, _ := json.Marshal(reqBody)

	resp, err := http.Post(
		nutsURL.JoinPath("/internal/discovery/v1/bgz-test", subject).String(),
		"application/json",
		bytes.NewReader(body),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "Failed to register on discovery: %s", string(respBody))
}

// tokenResult holds the access token and optional DPoP key ID from a token request
type tokenResult struct {
	AccessToken string
	DPoPKID     string // Empty for Bearer tokens
	TokenType   string // "Bearer" or "DPoP"
}

func requestAccessToken(t *testing.T, nutsURL *url.URL, subject, authServer, scope, tokenType string) tokenResult {
	t.Helper()

	// Create self-attested credentials
	// The Nuts node will populate issuer, credentialSubject.id, issuanceDate, id, and proof automatically
	// Note: self-asserted credentials MUST NOT contain credentialSubject.id

	// NutsEmployeeCredential for employee identity
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

	// HealthcareProviderRoleTypeCredential for facility type
	providerTypeCredential := map[string]any{
		"@context": []string{
			"https://www.w3.org/2018/credentials/v1",
		},
		"type": []string{"VerifiableCredential", "HealthcareProviderRoleTypeCredential"},
		"credentialSubject": map[string]any{
			"roleCodeNL": "Z3", // Ziekenhuis (hospital)
		},
	}

	// Request token with X509Credential (from wallet) + self-attested credentials
	tokenEndpoint := nutsURL.JoinPath("/internal/auth/v2", subject, "request-service-access-token").String()
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

// createDPoPProof creates a DPoP proof using the Nuts node's key
// This delegates proof creation to the Nuts node, which has access to the private key
func createDPoPProof(t *testing.T, nutsURL *url.URL, kid, method, targetURL, accessToken string) string {
	t.Helper()

	// The kid must be URL-encoded for the path parameter
	// The Nuts node manually unescapes it (see dpop.go line 59)
	encodedKID := url.QueryEscape(kid)
	dpopEndpoint := nutsURL.String() + "/internal/auth/v2/dpop/" + encodedKID

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

func introspectToken(t *testing.T, nutsURL *url.URL, token string) map[string]interface{} {
	t.Helper()

	resp, err := http.PostForm(
		nutsURL.JoinPath("/internal/auth/v2/accesstoken/introspect").String(),
		url.Values{"token": {token}},
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
	return result
}

func startKnooppunt(t *testing.T, httpConfig httpComp.Config, hapiURL *url.URL, mockMitz *mitzmock.ClosedQuestionService) *url.URL {
	t.Helper()

	config := cmd.Config{
		HTTP: httpConfig,
		PDP: pdp.Config{
			Enabled: true,
			PIP: pdp.PIPConfig{
				URL: hapiURL.String() + "/DEFAULT",
			},
		},
		MITZ: mitz.Config{
			MitzBase:      mockMitz.GetURL(),
			GatewaySystem: "test-gateway",
			SourceSystem:  "test-source",
		},
	}

	var errChan = make(chan error, 1)
	go func() {
		if err := cmd.Start(t.Context(), config); err != nil {
			errChan <- err
		}
	}()

	baseURL, _ := url.Parse(httpConfig.InternalInterface.BaseURL)

	// Wait for Knooppunt to be ready
	client := &http.Client{Timeout: 5 * time.Second}
	for i := 0; i < 30; i++ {
		select {
		case err := <-errChan:
			t.Fatalf("failed to start knooppunt: %v", err)
		default:
		}

		resp, err := client.Get(baseURL.JoinPath("/status").String())
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return baseURL
		}
		time.Sleep(time.Second)
	}
	t.Fatal("Knooppunt did not become ready in time")
	return nil
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

