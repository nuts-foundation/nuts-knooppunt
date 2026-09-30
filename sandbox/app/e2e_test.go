package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/stretchr/testify/require"
)

// The chain end to end, with every leg between the sandbox and the data real:
// the NVI answers which organization holds this patient's records, the directory
// answers where and behind which authorization server, a Nuts node issues the
// token, and the source's own policy enforcement point decides whether to serve.
// Dezi and Mitz are the mocks at its edges.
//
// Every other test on this path replaces one of those legs. That is what makes
// them fast and what makes them unable to notice when the legs stop fitting
// together, which is the failure this one exists to catch. The chain is built
// once; each case signs in to a sandbox of its own.
func TestAcceptance_FullChain(t *testing.T) {
	chain := startFullChain(t)

	// The happy path. One confirmation, and the practitioner ends up looking at
	// another organization's data with that organization named on it.
	t.Run("retrieves through the source PEP", func(t *testing.T) {
		sandbox := chain.openSandbox(t)

		status, body := sandbox.retrieve(t)

		require.Equal(t, http.StatusOK, status, "body: %s", body)
		require.Contains(t, body, "Access granted by Sunflower Care Home", "the source's own PEP decided this")
		// That verdict is the Patient search's alone. The source decides every
		// clinical search separately, and one it refused shows only as data that
		// never arrived, so each category it holds for this patient has to be on
		// the record.
		record := sandbox.record(t)
		for _, line := range sourceData {
			require.Contains(t, record, line.title, "the %s search was served and its data reached the record", line.section)
		}
	})

	// The denial. The source's policy asks Mitz, Mitz says no, and the
	// practitioner gets a refusal and nothing else.
	t.Run("a refused retrieval exposes no source data", func(t *testing.T) {
		chain.MockMitzXACML.SetResponse("Deny", "No consent found")
		t.Cleanup(func() { chain.MockMitzXACML.SetResponse("Permit", "Consent granted") })
		sandbox := chain.openSandbox(t)
		questionsBefore := chain.MockMitzXACML.RequestCount()

		status, body := sandbox.retrieve(t)

		require.Equal(t, http.StatusOK, status, "body: %s", body)
		require.Contains(t, body, "Access denied by Sunflower Care Home", "the source declined, and the screen says so")
		// Any 401 or 403 on the Patient search renders this screen, an expired
		// token as much as a consent refusal. This one has to be Mitz's.
		require.Greater(t, chain.MockMitzXACML.RequestCount(), questionsBefore, "the source's policy asked Mitz")
		require.Contains(t, chain.MockMitzXACML.GetLastRequestXML(), chain.patient.BSN, "about this patient")
		// The source's name stays on the page, where the viewer draws who was
		// asked. What a refusal may not leave is any of its data.
		record := sandbox.record(t)
		for _, line := range sourceData {
			require.NotContains(t, record, line.title, "a refusal may leave none of the source's %s on the record", line.section)
		}
	})
}

// sourceData is one line per data category the source holds for anna, none of
// which De Plataan holds itself, so any of them on the record is data that came
// from the source. The happy path requires all of them and the refusal none,
// from the same list, so a fixture change cannot quietly empty the refusal's
// check.
var sourceData = []struct{ section, title string }{
	{"Allergies", "Penicilline"},
	{"Medication", "Metoprolol"},
	{"Conditions", "Diabetes mellitus type 2"},
}

// fullChain is the exchange, up and addressable: the directories, the NVI, the
// Nuts node and the source's policy enforcement point in front of its data.
type fullChain struct {
	harness.Details
	patient pool.PoolPatient
}

func startFullChain(t *testing.T) fullChain {
	t.Helper()
	// The sandbox reads the first four to decide which wallet, scope, facility
	// type and client id it asks with, and the seed reads the last to decide
	// which authorization server it publishes. This chain provisions the
	// defaults, so a developer with any of them exported would test a chain
	// other than the one built here. authorizeSandbox clears the first three for
	// the same reason.
	for _, key := range []string{
		"SANDBOX_NUTS_SUBJECT",
		"SANDBOX_BGZ_SCOPE",
		"SANDBOX_FACILITY_TYPE",
		"SANDBOX_NVI_CLIENT_ID",
		sunflower.AuthorizationServerEnvVar,
	} {
		t.Setenv(key, "")
	}

	// The source's data sits in this tenant, behind the PEP below, and it is
	// where the source's policy decision point looks the patient up.
	dataHolder := sunflower.PatientsHAPITenant()
	h := harness.StartWithNuts(t, dataHolder)
	nutsAPI := func(path string) string {
		return h.KnooppuntInternalBaseURL.JoinPath("/nuts", path).String()
	}
	certs := harness.PEPCertsDir(t)

	// De Plataan asks, from the wallet the sandbox requests its token with. It
	// holds the X509 credential carrying URA 00000010, which is the organization
	// the source's policy reads.
	harness.RegisterRequester(t, nutsAPI, nutsConfigFromEnv().Subject,
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
		FHIRUpstreamPath:          "/fhir/" + dataHolder.Name,
		KnooppuntPDPHost:          "host.docker.internal",
		KnooppuntPDPPort:          h.KnooppuntInternalBaseURL.Port(),
		NutsNodeHost:              "host.docker.internal",
		NutsNodePort:              h.KnooppuntInternalBaseURL.Port(),
		DataHolderOrganizationURA: h.SunflowerURA,
		DataHolderFacilityType:    "Z3",
	})

	// The harness seeded the source's FHIR endpoint straight at HAPI. Here the
	// PEP is the address, and it is only known once its container has a port, so
	// the directories are seeded again with it, the way docker compose seeds
	// them. Load only upserts, and it leaves the NVI alone.
	t.Setenv(sunflower.EndpointAddressEnvVar, pep.URL.JoinPath("fhir").String())
	_, err := vectors.Load(h.HAPIBaseURL)
	require.NoError(t, err)
	syncMCSD(t, h)

	// What the sandbox reads at startup, pointed at this chain.
	t.Setenv("KNOOPPUNT_INTERNAL_URL", h.KnooppuntInternalBaseURL.String())
	t.Setenv("HAPI_BASE_URL", h.HAPIBaseURL.String())
	t.Setenv("MITZMOCK_URL", h.MockMitzXACML.GetURL())

	anna, ok := pool.PatientByKey("anna")
	require.True(t, ok)
	return fullChain{Details: h, patient: anna}
}

// sandbox is one practitioner's session in the sandbox, with the patient open.
type sandbox struct {
	server *httptest.Server
	client *http.Client
	key    string
	ura    string
}

// openSandbox starts the sandbox wired the way main wires it, against this
// chain, and opens the patient in a session of its own, so no case reads a
// record another one retrieved.
func (c fullChain) openSandbox(t *testing.T) sandbox {
	t.Helper()
	cfg, err := NewConfigFromEnv(os.Getenv)
	require.NoError(t, err)
	server, client := demoServer(t, cfg)
	openPatient(t, client, server, c.patient.Key)
	return sandbox{server: server, client: client, key: c.patient.Key, ura: c.SunflowerURA}
}

// retrieve opens the retrieve page and confirms the source it offers. Opening it
// runs discovery against the real NVI and directory; confirming it is where the
// sandbox asks the Nuts node for a token.
//
// That request is the one leg that cannot pass yet, and the node is right to
// refuse it: the sandbox signs in against a fixture attestation whose signature
// is the word "signature" and whose header carries no jku
// (dezi_fixture_test.go). That is fine for every other test here, because this
// backend never verifies one, and it is not fine for a real Nuts node, which
// fetches the signing key at the jku before it will accept the id_token.
// Finishing this means running mock-components/dezi in process with a real key,
// serving its JWK set over TLS, trusting that certificate in the node and
// naming it in NUTS_VCR_DEZI_ALLOWEDJKU:
// https://github.com/nuts-foundation/nuts-knooppunt/issues/591. Everything
// before the confirmation runs.
func (s sandbox) retrieve(t *testing.T) (int, string) {
	t.Helper()
	confirmation := sourceConfirmation(t, s.client, s.server, s.key, s.ura)
	t.Skip("needs a Dezi that signs with a resolvable jku; see issue #591")
	return postFormAndRead(t, s.client, s.server, "/demo/ehr/patients/"+s.key+"/retrieve", confirmation)
}

// record renders the patient's record. It has to come back as the record for
// anything missing from it to mean something: the client follows no redirects,
// so a lost session would otherwise read as a record without the source.
func (s sandbox) record(t *testing.T) string {
	t.Helper()
	status, body := getBody(t, s.client, s.server, "/demo/ehr/patients/"+s.key)
	require.Equal(t, http.StatusOK, status, "body: %s", body)
	return body
}
