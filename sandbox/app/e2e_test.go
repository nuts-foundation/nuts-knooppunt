package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/stretchr/testify/require"
)

// The chain end to end, with nothing faked between the sandbox and the data: the
// NVI answers which organization holds this patient's records, the directory
// answers where and behind which authorization server, a Nuts node issues the
// token, and the source's own policy enforcement point decides whether to serve.
//
// Every other test on this path replaces one of those legs. That is what makes
// them fast and what makes them unable to notice when the legs stop fitting
// together, which is the failure this one exists to catch.
type fullChain struct {
	harness.Details
	server *httptest.Server
	client *http.Client
	key    string
}

// startFullChain brings up the exchange and points a signed-in sandbox at it.
func startFullChain(t *testing.T) fullChain {
	t.Helper()
	// The one leg still faked, and the node is right to refuse it: the sandbox
	// signs in against a fixture attestation whose signature is the word
	// "signature" and whose header carries no jku (dezi_fixture_test.go). That is
	// fine for every other test here, because this backend never verifies one,
	// and it is not fine for a real Nuts node, which fetches the signing key at
	// the jku before it will accept the id_token. Finishing this means running
	// mock-components/dezi in process with a real key, serving its JWK set over
	// TLS, trusting that certificate in the node and naming it in
	// NUTS_VCR_DEZI_ALLOWEDJKU. Everything up to that point is exercised: see
	// https://github.com/nuts-foundation/nuts-knooppunt/issues/591.
	t.Skip("needs a Dezi that signs with a resolvable jku; see issue #591")

	h := harness.StartWithNuts(t)
	nutsAPI := func(path string) string {
		return h.KnooppuntInternalBaseURL.JoinPath("/nuts", path).String()
	}
	certs := harness.PEPCertsDir(t)

	// De Plataan asks. Its wallet holds the X509 credential carrying URA
	// 00000010, which is the organization the source's policy reads.
	harness.RegisterRequester(t, nutsAPI, "plataan",
		certs+"/plataan-chain.pem", certs+"/plataan.key")
	// The source answers. Its subject is its URA, which is what the oauth-nuts
	// endpoint in the directory names.
	harness.CreateSubject(t, nutsAPI, h.SunflowerURA)

	// The localization records that make the source findable. vectors.Load puts
	// the clinical data and the directories in place; the NVI is seeded
	// separately because it is written through the Knooppunt rather than into
	// HAPI, so it needs the node to be up.
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))

	// The source's data, behind its own policy enforcement point.
	pep := harness.StartPEPContainer(t, harness.PEPConfig{
		FHIRBackendHost:           "host.docker.internal",
		FHIRBackendPort:           h.HAPIBaseURL.Port(),
		FHIRBasePath:              "/fhir",
		FHIRUpstreamPath:          "/fhir/" + sunflower.PatientsHAPITenant().Name,
		KnooppuntPDPHost:          "host.docker.internal",
		KnooppuntPDPPort:          h.KnooppuntInternalBaseURL.Port(),
		NutsNodeHost:              "host.docker.internal",
		NutsNodePort:              h.KnooppuntInternalBaseURL.Port(),
		DataHolderOrganizationURA: h.SunflowerURA,
		DataHolderFacilityType:    "Z3",
	})

	// The seeded endpoint points straight at HAPI, because the address has to be
	// right for whichever deployment loaded the fixtures. Here the PEP is the
	// address, and it is only known once its container has a port.
	publishEndpoint(t, h, pep.URL.JoinPath("fhir").String())
	syncMCSD(t, h)

	t.Setenv("KNOOPPUNT_INTERNAL_URL", h.KnooppuntInternalBaseURL.String())
	cfg := Config{Locks: NewRegistry(), Retrievals: newRetrievalStore()}
	cfg.nviLookup, cfg.nviRegister = nviFuncs(h.KnooppuntInternalBaseURL, pool.PlataanClientID)
	cfg.nviLocalize = nviLocalizeFunc(h.KnooppuntInternalBaseURL)
	cfg.mcsdResolve = mcsdResolveFunc(h.HAPIBaseURL)
	t.Setenv("DEZI_INTERNAL_BASE_URL", fakeDezi(t).URL)

	server := httptest.NewServer(NewMux(cfg))
	t.Cleanup(server.Close)
	client := signInViaDezi(t, server)

	anna, ok := pool.PatientByKey("anna")
	require.True(t, ok)
	openPatient(t, client, server, anna.Key)
	return fullChain{Details: h, server: server, client: client, key: anna.Key}
}

// publishEndpoint rewrites the source's FHIR endpoint in its own directory, so
// the addressing step resolves to the policy enforcement point rather than to
// the data behind it.
func publishEndpoint(t *testing.T, h harness.Details, address string) {
	t.Helper()
	endpoint := sunflower.Endpoints()[0]
	endpoint.Address = address
	body, err := json.Marshal(endpoint)
	require.NoError(t, err)

	target := h.SunflowerFHIRBaseURL.JoinPath("Endpoint", *endpoint.Id).String()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, target, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/fhir+json")

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Less(t, res.StatusCode, 300, "publishing the endpoint has to succeed for this test to mean anything")
}

func (c fullChain) retrieve(t *testing.T) (int, string) {
	t.Helper()
	return postFormAndRead(t, c.client, c.server, "/demo/ehr/patients/"+c.key+"/retrieve",
		sourceConfirmation(t, c.client, c.server, c.key, c.SunflowerURA))
}

func (c fullChain) record(t *testing.T) string {
	t.Helper()
	_, body := getBody(t, c.client, c.server, "/demo/ehr/patients/"+c.key)
	return body
}

// The happy path. One confirmation, and the practitioner ends up looking at
// another organization's data with that organization named on it.
func TestAcceptance_RetrievesThroughTheSourcePEP(t *testing.T) {
	chain := startFullChain(t)

	status, body := chain.retrieve(t)

	require.Equal(t, http.StatusOK, status, "body: %s", body)
	require.Contains(t, body, "Access granted", "the source's own PEP decided this")
	require.NotContains(t, body, "· 403", "and no authorized query may come back refused")
	require.Contains(t, chain.record(t), "Sunflower Care Home",
		"the retrieved data reaches the record with its source named")
}

// The denial. The source's policy asks Mitz, Mitz says no, and the practitioner
// gets a refusal and nothing else.
func TestAcceptance_ARefusedRetrievalExposesNoSourceData(t *testing.T) {
	chain := startFullChain(t)
	chain.MockMitzXACML.SetResponse("Deny", "No consent found")

	status, body := chain.retrieve(t)

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Access refused", "the source declined, and the screen says so")
	require.NotContains(t, chain.record(t), "Sunflower Care Home",
		"a refusal may leave no trace of the source on the record")
}
