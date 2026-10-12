package main

import (
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	dezimock "github.com/nuts-foundation/nuts-knooppunt/mock-components/dezi"
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
		require.Contains(t, body, "Access granted by Zorgcentrum De Zonnebloem", "the source's own PEP decided this")
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
		require.Contains(t, body, "Access denied by Zorgcentrum De Zonnebloem", "the source declined, and the screen says so")
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
	syncDirectory(t, h)

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
	server, client := fullChainSandbox(t, cfg, c.MockDezi)
	openPatient(t, client, server, c.patient.Key)
	return sandbox{server: server, client: client, key: c.patient.Key, ura: c.SunflowerURA}
}

var deziRequestHandle = regexp.MustCompile(`name="request" value="([^"]+)"`)

// fullChainSandbox starts a sandbox that signs in through the mock Dezi's real
// authorization code flow. demoServer cannot: it points the sandbox at a fake
// that hands out the fixture attestation for any code.
//
// The sandbox's address is reserved before its handler is built, because the
// callback has to be the same exact URL in three places: the sandbox's own
// configuration, the mock's allowlist, and the redirect the browser follows.
func fullChainSandbox(t *testing.T, cfg Config, dezi harness.MockDezi) (*httptest.Server, *http.Client) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	base := "http://" + listener.Addr().String()
	callback := base + "/demo/auth/callback"

	authorization := httptest.NewServer(dezimock.NewMux(dezi.Signer, dezimock.DrElAmrani, []string{callback}))
	t.Cleanup(authorization.Close)
	t.Setenv("SANDBOX_PUBLIC_URL", base)
	t.Setenv("DEZI_INTERNAL_BASE_URL", authorization.URL)
	t.Setenv("DEZI_PUBLIC_AUTHORIZE_URL", authorization.URL+"/authorize")

	server := httptest.NewUnstartedServer(NewMux(cfg))
	require.NoError(t, server.Listener.Close())
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	// The sandbox sends the browser to Dezi.
	res, err := client.PostForm(server.URL+"/demo/login", nil)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	authorizeURL := res.Header.Get("Location")
	require.True(t, strings.HasPrefix(authorizeURL, authorization.URL+"/authorize?"), "login must redirect to the mock Dezi, got %s", authorizeURL)

	// Dezi shows its consent page, which carries the request handle.
	status, page := getPageWithClient(t, client, authorizeURL)
	require.Equal(t, http.StatusOK, status, "body: %s", page)
	handle := deziRequestHandle.FindStringSubmatch(page)
	require.Len(t, handle, 2, "the consent page must carry a request handle")

	// The practitioner agrees, and Dezi sends the browser back with a code.
	res, err = client.PostForm(authorization.URL+"/authorize", url.Values{"request": {handle[1]}})
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	back := res.Header.Get("Location")
	require.True(t, strings.HasPrefix(back, callback+"?"), "Dezi must redirect to the sandbox callback, got %s", back)

	res, err = client.Get(back)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo/ehr", res.Header.Get("Location"), "a successful callback lands on the EHR home")
	return server, client
}

// retrieve opens the retrieve page and confirms the source it offers. Opening it
// runs discovery against the real NVI and directory; confirming it is where the
// sandbox asks the Nuts node for a token.
//
// The sandbox signs in through the mock Dezi the harness runs in process, so the
// attestation it sends as id_token carries a signature and a jku the node can
// resolve.
func (s sandbox) retrieve(t *testing.T) (int, string) {
	t.Helper()
	confirmation := sourceConfirmation(t, s.client, s.server, s.key, s.ura)
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
