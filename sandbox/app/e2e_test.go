package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"slices"
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

		// The proof the consent evidence rests on: the trace id the sandbox put on
		// each source request came through the source's PEP and the Knooppunt's
		// PDP to the closed question, where the mock recorded it.
		questions := chain.MockMitzXACML.Decisions()
		require.NotEmpty(t, questions, "the source's policy asked Mitz")
		for _, question := range questions {
			require.NotEmptyf(t, question.TraceID, "closed question %d reached Mitz without the request's trace id", question.Seq)
			require.Equal(t, chain.patient.BSN, question.PatientID)
			require.Equal(t, "00000010", question.RequesterURA)
			require.Equal(t, chain.SunflowerURA, question.DataHolderURA)
		}
		checks := 0
		for _, event := range sandbox.runEvents(t) {
			if event.Purpose == "consent-check" {
				checks++
				require.Equal(t, "ok", event.Outcome)
				require.Equal(t, "Permit", event.Response.ConsentDecision)
				require.NotEmpty(t, event.ParentCallID)
			}
		}
		require.Equal(t, len(questions), checks, "every closed question of this chain became a consent-check event")
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

	// E8, end to end. Each step is asserted where it happens: removal by the
	// notification before any new retrieval, because a refused retrieval
	// replaces the stored copy by itself and would satisfy a later check.
	t.Run("consent withdrawn, refused, restored", func(t *testing.T) {
		sandbox := chain.openSandbox(t)
		// Leave the mock as the other cases expect it, silently and whatever
		// state this case ends in.
		t.Cleanup(func() { clearConsentAt(t, chain.MockMitzXACML.GetURL(), chain.patient.BSN) })

		// 1. Share, so De Plataan holds a consent subscription whose endpoint is
		//    the Knooppunt, and retrieve.
		status, body := sandbox.share(t)
		require.Equal(t, http.StatusOK, status, "body: %s", body)
		require.Contains(t, body, "Consent subscription", "the share reached Mitz")
		require.Contains(t, body, "the Knooppunt forwards the notification to this EHR", "and the card says where its notifications go")
		status, body = sandbox.retrieve(t)
		require.Equal(t, http.StatusOK, status, "body: %s", body)
		require.Contains(t, body, "Access granted by Zorgcentrum De Zonnebloem")
		for _, line := range sourceData {
			require.Contains(t, sandbox.record(t), line.title)
		}

		// 2. Withdraw. Before any new retrieval: the notification came through
		//    the Knooppunt and removed the copy.
		location := sandbox.consent(t, "deny")
		require.Equal(t, "/demo/ehr/patients/"+sandbox.key+"?notice=consent-withdrawn", location,
			"the sandbox itself received the notification")
		record := sandbox.record(t)
		require.Contains(t, record, "has withdrawn consent for data sharing")
		require.Contains(t, record, "No longer available · consent withdrawn")
		for _, line := range sourceData {
			require.NotContains(t, record, line.title, "the notification removed the source's %s", line.section)
		}
		purposes := purposesOf(sandbox.runEvents(t))
		change, notified := slices.Index(purposes, "consent-change"), slices.Index(purposes, "consent-notification")
		require.GreaterOrEqual(t, change, 0)
		require.Greater(t, notified, change, "the change is published before its notification")

		// 3. Retrieve again: refused at a fresh consent check, with the
		//    notification named as context.
		body = chain.requireRefusedAtConsent(t, sandbox)
		require.Contains(t, body, "De Plataan received a Mitz notification at")
		for _, line := range sourceData {
			require.NotContains(t, sandbox.record(t), line.title)
		}

		// 4. Restore: the record says so, and a retrieval brings the data back.
		location = sandbox.consent(t, "permit")
		require.Equal(t, "/demo/ehr/patients/"+sandbox.key+"?notice=consent-restored", location)
		require.Contains(t, sandbox.record(t), "has given consent for data sharing again")
		status, body = sandbox.retrieve(t)
		require.Equal(t, http.StatusOK, status, "body: %s", body)
		require.Contains(t, body, "Access granted by Zorgcentrum De Zonnebloem")
		record = sandbox.record(t)
		for _, line := range sourceData {
			require.Contains(t, record, line.title, "the source's %s is back", line.section)
		}
		require.NotContains(t, record, "No longer available")
	})

	// The relay target fails: enforcement does not depend on the notification.
	// The refusal is required to carry the same evidence as the one above.
	t.Run("a lost notification leaves the refusal intact", func(t *testing.T) {
		sandbox := chain.openSandbox(t)
		t.Cleanup(func() { clearConsentAt(t, chain.MockMitzXACML.GetURL(), chain.patient.BSN) })
		sandbox.share(t)
		status, body := sandbox.retrieve(t)
		require.Equal(t, http.StatusOK, status, "body: %s", body)
		require.Contains(t, body, "Access granted by Zorgcentrum De Zonnebloem")

		chain.NotifyIntake.Attach(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "the EHR is down", http.StatusServiceUnavailable)
		}))

		location := sandbox.consent(t, "deny")
		require.Equal(t, "/demo/ehr/patients/"+sandbox.key+"?notice=consent-withdrawn-unnotified", location,
			"consent changed at Mitz and nothing arrived here")
		record := sandbox.record(t)
		for _, line := range sourceData {
			require.Contains(t, record, line.title, "without a notification the retrieved %s stays", line.section)
		}
		require.NotContains(t, record, `class="mitz-note"`)

		body = chain.requireRefusedAtConsent(t, sandbox)
		require.NotContains(t, body, "received a Mitz notification", "there was none to name")
		for _, line := range sourceData {
			require.NotContains(t, sandbox.record(t), line.title, "and the refusal left none of the source's %s", line.section)
		}
	})

	// Recycle restores the configured answer for that patient and touches
	// nothing of another: not its answer at Mitz, and not what a colleague in
	// the same sandbox has open and retrieved.
	t.Run("recycle restores one patient and leaves another alone", func(t *testing.T) {
		mock := chain.MockMitzXACML.GetURL()
		other := pool.Patients()[1]
		require.NotEqual(t, chain.patient.Key, other.Key)
		// What the source holds for the pool's second patient and De Plataan does
		// not (test/testdata/vectors/pool).
		const otherAllergy = "Amoxicilline"
		t.Cleanup(func() {
			clearConsentAt(t, mock, chain.patient.BSN)
			clearConsentAt(t, mock, other.BSN)
		})

		sandbox := chain.openSandbox(t)
		sandbox.share(t)
		require.Equal(t, "/demo/ehr/patients/"+sandbox.key+"?notice=consent-withdrawn", sandbox.consent(t, "deny"))

		// A colleague with the other patient open: shared, retrieved, and with a
		// consent change of that patient's own. A permit, so that its
		// notification leaves the retrieved data where it is.
		colleague := sandbox.colleague(t, other)
		colleague.share(t)
		status, body := colleague.retrieve(t)
		require.Equal(t, http.StatusOK, status, "body: %s", body)
		require.Contains(t, body, "Access granted by Zorgcentrum De Zonnebloem")
		require.Equal(t, "/demo/ehr/patients/"+other.Key+"?notice=consent-restored", colleague.consent(t, "permit"))
		// The run's id is read once. Every render of the record adds status reads
		// to the run, so its events are compared by id, around the recycle and
		// with no render in between.
		colleagueRun := colleague.runID(t)
		before := colleague.record(t)
		require.Contains(t, before, otherAllergy, "the colleague holds the source's data about the other patient")
		require.Contains(t, before, "has given consent for data sharing again")
		eventsBefore := colleague.eventsOf(t, colleagueRun)
		require.NotEmpty(t, eventsBefore)

		// Recycle refuses a patient somebody holds.
		res := postForm(t, sandbox.client, sandbox.server, "/demo/patients/"+sandbox.key+"/release", nil)
		require.NoError(t, res.Body.Close())
		res = postForm(t, sandbox.client, sandbox.server, "/demo/patients/"+sandbox.key+"/recycle", nil)
		require.NoError(t, res.Body.Close())
		require.Equal(t, http.StatusSeeOther, res.StatusCode)
		require.Equal(t, "/demo?notice=recycle-done", res.Header.Get("Location"))

		require.Equal(t, eventsBefore, colleague.eventsOf(t, colleagueRun), "the colleague's run still exists and holds exactly what it held")
		require.Equal(t, consentAnswer{Decision: "Permit"}, consentAt(t, mock, chain.patient.BSN), "the recycled patient is back at the configured answer")
		require.Equal(t, consentAnswer{Decision: "Permit", Overridden: true}, consentAt(t, mock, other.BSN),
			"another patient's consent at Mitz is untouched")
		after := colleague.record(t) // 200: the colleague's session and lease are alive
		require.Contains(t, after, otherAllergy, "what the colleague retrieved about the other patient stays")
		require.Contains(t, after, "has given consent for data sharing again", "and so does that patient's notification")
		require.Equal(t, colleagueRun, colleague.runID(t), "and the record still names the same run")
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
	// Where this chain serves the sandbox's notification intake: the listener the
	// harness reserved for it, which openSandbox attaches the handler to.
	intakeURL, err := url.Parse(h.NotifyIntake.URL)
	require.NoError(t, err)
	t.Setenv("SANDBOX_MITZ_NOTIFY_LISTEN_ADDR", intakeURL.Host)

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
	// signIn signs another practitioner in to this same sandbox. Nil for a
	// sandbox that was not opened through the full chain.
	signIn func(*testing.T) *http.Client
}

// openSandbox starts the sandbox wired the way main wires it, against this
// chain, and opens the patient in a session of its own, so no case reads a
// record another one retrieved.
func (c fullChain) openSandbox(t *testing.T) sandbox {
	t.Helper()
	cfg, err := NewConfigFromEnv(os.Getenv)
	require.NoError(t, err)
	server, client, intake, signIn := fullChainSandbox(t, cfg, c.MockDezi)
	// The Knooppunt forwards to one address; this case's sandbox is the one
	// that receives from now on.
	c.NotifyIntake.Attach(intake)
	t.Cleanup(func() { c.NotifyIntake.Attach(nil) })
	openPatient(t, client, server, c.patient.Key)
	return sandbox{server: server, client: client, key: c.patient.Key, ura: c.SunflowerURA, signIn: signIn}
}

// colleague opens another pool patient in a second practitioner session on the
// same sandbox, so both sessions share its stores. Opening a second sandbox
// would not do: each has stores of its own, and nothing one does could reach
// the other.
func (s sandbox) colleague(t *testing.T, patient pool.PoolPatient) sandbox {
	t.Helper()
	client := s.signIn(t)
	openPatient(t, client, s.server, patient.Key)
	return sandbox{server: s.server, client: client, key: patient.Key, ura: s.ura, signIn: s.signIn}
}

var deziRequestHandle = regexp.MustCompile(`name="request" value="([^"]+)"`)

// fullChainSandbox starts a sandbox that signs in through the mock Dezi's real
// authorization code flow. demoServer cannot: it points the sandbox at a fake
// that hands out the fixture attestation for any code.
//
// The sandbox's address is reserved before its handler is built, because the
// callback has to be the same exact URL in three places: the sandbox's own
// configuration, the mock's allowlist, and the redirect the browser follows.
func fullChainSandbox(t *testing.T, cfg Config, dezi harness.MockDezi) (*httptest.Server, *http.Client, http.Handler, func(*testing.T) *http.Client) {
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

	app, intake := newHandlers(cfg)
	server := httptest.NewUnstartedServer(app)
	require.NoError(t, server.Listener.Close())
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)

	// signIn takes one practitioner through the mock Dezi's authorization code
	// flow and returns that session's client. Each call is a session of its own.
	signIn := func(t *testing.T) *http.Client {
		t.Helper()
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
		return client
	}
	return server, signIn(t), intake, signIn
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

// share shares the patient through the generic functions, as the record's
// button does: localization records and a consent subscription.
func (s sandbox) share(t *testing.T) (int, string) {
	t.Helper()
	return postFormAndRead(t, s.client, s.server, "/demo/ehr/patients/"+s.key+"/share", nil)
}

// consent changes consent through the demo's control and returns where it
// redirected to.
func (s sandbox) consent(t *testing.T, decision string) string {
	t.Helper()
	res := postForm(t, s.client, s.server, "/demo/ehr/patients/"+s.key+"/consent", url.Values{"decision": {decision}})
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	return res.Header.Get("Location")
}

// requireRefusedAtConsent retrieves, and requires the refusal to be Mitz's and
// to be observed as such: one closed question, about this patient and between
// these two organizations, answered Deny; the Patient search answered 403 by
// the source's PEP and no clinical search sent after it; and Mitz's answer
// attached to that search as its consent check. It returns the page.
func (c fullChain) requireRefusedAtConsent(t *testing.T, s sandbox) string {
	t.Helper()
	questionsBefore := len(c.MockMitzXACML.Decisions())
	eventsBefore := len(s.runEvents(t))

	status, body := s.retrieve(t)

	require.Equal(t, http.StatusOK, status, "body: %s", body)
	require.Contains(t, body, "Access denied by Zorgcentrum De Zonnebloem")
	questions := c.MockMitzXACML.Decisions()[questionsBefore:]
	require.Len(t, questions, 1, "the Patient search was refused and nothing further was asked")
	require.Equal(t, "Deny", questions[0].Decision)
	require.Equal(t, c.patient.BSN, questions[0].PatientID)
	require.Equal(t, "00000010", questions[0].RequesterURA)
	require.Equal(t, c.SunflowerURA, questions[0].DataHolderURA)
	var patientSearch, consentCheck *stepEvent
	for _, event := range s.runEvents(t)[eventsBefore:] {
		switch {
		case event.GF == "exchange":
			require.Nil(t, patientSearch, "no clinical search was sent after the refusal")
			patientSearch = &event
		case event.Purpose == "consent-check":
			consentCheck = &event
		}
	}
	require.NotNil(t, patientSearch)
	require.Equal(t, "Patient", patientSearch.ResourceType)
	require.Equal(t, http.StatusForbidden, patientSearch.Response.Status, "the source's PEP refused it")
	require.NotNil(t, consentCheck, "the answer Mitz gave about this request was observed")
	require.Equal(t, patientSearch.CallID, consentCheck.ParentCallID)
	require.Equal(t, "Deny", consentCheck.Response.ConsentDecision)
	require.Equal(t, "deny", consentCheck.Outcome)
	return body
}

// clearConsentAt restores the mock's configured answer for one patient,
// silently, as a recycle does. The cases use it to leave the mock the way the
// next case expects it.
func clearConsentAt(t *testing.T, mock, bsn string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, mock+"/mock/consent/"+bsn, nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusOK, res.StatusCode)
}

type consentAnswer struct {
	Decision   string `json:"decision"`
	Overridden bool   `json:"overridden"`
}

func consentAt(t *testing.T, mock, bsn string) consentAnswer {
	t.Helper()
	res, err := http.Get(mock + "/mock/consent/" + bsn)
	require.NoError(t, err)
	defer res.Body.Close()
	var answer consentAnswer
	require.NoError(t, json.NewDecoder(res.Body).Decode(&answer))
	return answer
}
