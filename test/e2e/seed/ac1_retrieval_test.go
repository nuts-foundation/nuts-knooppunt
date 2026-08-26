package seed

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// ac1EnvVar opts this test in. AC1 is a statement about a real `docker compose`
// deployment, so unlike the other e2e tests this one drives the composed stack
// rather than assembling components through the harness — a harness-built stack
// would wire the components differently and prove something else.
//
// Run it with:
//
//	docker compose up -d --wait
//	KNPT_E2E_COMPOSE=1 go test ./test/e2e/seed/ -run TestAC1 -v
const ac1EnvVar = "KNPT_E2E_COMPOSE"

// Addresses of the composed stack as seen from the host. Inside the compose
// network these services are reachable as knooppunt:8081, pep-zonnebloem:8080
// and so on; from here they are published on localhost.
const (
	composeKnooppuntInternal = "http://localhost:8081"
	composeKnooppuntPublic   = "http://localhost:8080"
	composeHAPI              = "http://localhost:7050/fhir"
	composePEPZonnebloem     = "http://localhost:9080"
)

const bgzScope = "medicatieoverdracht-gf"

// TestAC1_FindableAddressableRetrievable is the acceptance test for issue #542
// AC1: "a fresh local or hosted deployment yields a findable, addressable and
// retrievable patient without manual setup".
//
// Each subtest covers one of the three verbs, plus a negative case proving the
// PEP is genuinely in the request path.
func TestAC1_FindableAddressableRetrievable(t *testing.T) {
	if os.Getenv(ac1EnvVar) == "" {
		t.Skipf("set %s=1 and run `docker compose up -d --wait` to run the AC1 acceptance test", ac1EnvVar)
	}

	anna, ok := pool.PatientByKey("anna")
	require.True(t, ok, "the pool must contain the canonical patient 'anna'")

	// Fails loudly when the committed test certificates expire, rather than
	// surfacing later as an opaque 401 from the token request.
	t.Run("test certificates are still valid", func(t *testing.T) {
		for _, org := range []string{"plataan", "zonnebloem"} {
			notAfter := certNotAfter(t, filepath.Join("..", "pep", "certs", org+".pem"))
			assert.Truef(t, notAfter.After(time.Now()),
				"certificate for %s expired on %s — re-issue it, see test/e2e/pep/certs/README.md", org, notAfter)
		}
	})

	// Findable: the NVI answers "which organizations hold data about this BSN?".
	t.Run("findable via NVI", func(t *testing.T) {
		nviBaseURL, err := url.Parse(composeKnooppuntInternal + "/nvi")
		require.NoError(t, err)

		entries := searchNVIByBSN(t, nviBaseURL, zonnebloemURA(), anna.BSN)
		require.NotEmpty(t, entries,
			"NVI should return a localization List for BSN %s under custodian %s", anna.BSN, zonnebloemURA())
	})

	// Addressable: the mCSD query directory resolves the custodian to an address.
	//
	// This is the regression guard for the endpoint repointing: before it, the
	// seeded Endpoint pointed straight at HAPI on :7050, which resolves from a
	// browser but bypasses the PEP — and therefore the whole authorization chain.
	t.Run("addressable via mCSD, and the address is the PEP", func(t *testing.T) {
		queryBaseURL, err := url.Parse(composeHAPI + "/knpt-mcsd-query")
		require.NoError(t, err)
		queryClient := fhirclient.New(queryBaseURL, http.DefaultClient, nil)

		org, err := searchOrg(queryClient, zonnebloemURA())
		require.NoError(t, err)
		require.NotNil(t, org, "Zonnebloem (URA %s) should be in the query directory", zonnebloemURA())
		require.NotEmpty(t, org.Endpoint, "Zonnebloem should publish an Endpoint")

		address := resolveEndpointAddress(t, queryClient, *org.Endpoint[0].Reference)

		assert.NotContains(t, address, ":7050",
			"the published address must not point straight at HAPI: that bypasses the PEP, so the patient would be addressable but not authorized")
		assert.Contains(t, address, "pep-zonnebloem",
			"the published address should resolve to Zonnebloem's PEP")
	})

	// Retrievable: an authorized read through the PEP → PDP → Mitz chain.
	//
	// No credential is issued here: the seed already stored De Plataan's
	// X509Credential in its wallet and registered it on discovery. That is
	// precisely what makes this an AC1 test ("without manual setup") rather than
	// a re-run of the PEP e2e flow.
	t.Run("retrievable through the PEP with an access token", func(t *testing.T) {
		token := requestPlataanAccessToken(t, anna.BSN)

		targetURL := composePEPZonnebloem + "/fhir/MedicationRequest?patient=Patient/" + anna.ZonnebloemPatientID
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, targetURL, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		require.Equalf(t, http.StatusOK, resp.StatusCode,
			"authorized retrieval must succeed.\n"+
				"401 → the seed did not store De Plataan's credential (check the init-credentials logs)\n"+
				"403 → the PDP denied: check KNPT_PDP_PIP_URL points at the tenant holding the patient, and that mitzmock is up\n"+
				"response: %s", string(body))

		var bundle fhir.Bundle
		require.NoError(t, json.Unmarshal(body, &bundle), "response must be a FHIR Bundle: %s", string(body))
		assert.NotEmpty(t, bundle.Entry,
			"the Bundle should contain the seeded MedicationRequests for %s", anna.ZonnebloemPatientID)
	})

	// Without this, a 200 above could just be HAPI answering directly.
	t.Run("unauthorized request is rejected by the PEP", func(t *testing.T) {
		targetURL := composePEPZonnebloem + "/fhir/MedicationRequest?patient=Patient/" + anna.ZonnebloemPatientID
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, targetURL, nil)
		require.NoError(t, err)

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode,
			"a tokenless request must be rejected, otherwise the PEP is not actually in the path")
	})
}

// requestPlataanAccessToken asks De Plataan's Nuts subject for an access token
// against Zonnebloem's authorization server, using the X509Credential the seed
// stored in De Plataan's wallet plus the two per-request self-attested
// credentials the BGZ policy requires.
func requestPlataanAccessToken(t *testing.T, patientBSN string) string {
	t.Helper()

	authServer := composeKnooppuntPublic + "/nuts/oauth2/" + zonnebloemURA()

	reqBody := map[string]any{
		"authorization_server": authServer,
		"scope":                bgzScope,
		"token_type":           "Bearer",
		"credentials": []any{
			// Delegation of the healthcare professional (mandaat).
			map[string]any{
				"@context": []string{"https://www.w3.org/2018/credentials/v1"},
				"type":     []string{"VerifiableCredential", "HealthCareProfessionalDelegationCredential"},
				"credentialSubject": map[string]any{
					"registeredBy": "http://fhir.nl/fhir/NamingSystem/uzi-nr-pers|000012345",
					"roleCode":     "http://fhir.nl/fhir/NamingSystem/role-code-pers|01.015",
					"facilityType": "Z3",
				},
			},
			// The VC equivalent of the AORTA inschrijftoken; patient-specific, so
			// it is passed per request rather than stored in the wallet (#406).
			map[string]any{
				"@context": []string{"https://www.w3.org/2018/credentials/v1"},
				"type":     []string{"VerifiableCredential", "PatientEnrollmentCredential"},
				"credentialSubject": map[string]any{
					"patientId":    "http://fhir.nl/fhir/NamingSystem/bsn|" + patientBSN,
					"registeredBy": "urn:oid:2.16.528.1.1007.3.1.12345",
				},
			},
		},
	}
	body, err := json.Marshal(reqBody)
	require.NoError(t, err)

	endpoint := composeKnooppuntInternal + "/nuts/internal/auth/v2/" + plataan.URA + "/request-service-access-token"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(string(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equalf(t, http.StatusOK, resp.StatusCode,
		"failed to get an access token as De Plataan.\n"+
			"This usually means the seed did not store the X509Credential or did not register on discovery — check the init-credentials service logs.\n"+
			"response: %s", string(respBody))

	var result struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal(respBody, &result))
	require.NotEmpty(t, result.AccessToken)
	return result.AccessToken
}

// resolveEndpointAddress follows an Organization.endpoint reference and returns
// the Endpoint's address.
func resolveEndpointAddress(t *testing.T, client fhirclient.Client, reference string) string {
	t.Helper()

	var endpoint fhir.Endpoint
	require.NoErrorf(t, client.ReadWithContext(t.Context(), reference, &endpoint),
		"failed to resolve endpoint reference %s", reference)
	require.NotEmpty(t, endpoint.Address)
	return endpoint.Address
}

// certNotAfter returns the expiry of a PEM-encoded certificate.
func certNotAfter(t *testing.T, path string) time.Time {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoErrorf(t, err, "unable to read certificate %s", path)

	block, _ := pem.Decode(data)
	require.NotNilf(t, block, "%s is not PEM-encoded", path)

	certificate, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	return certificate.NotAfter
}
