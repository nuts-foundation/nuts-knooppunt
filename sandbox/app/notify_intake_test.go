package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
)

// notificationJSON is a notification as Mitz sends it, written out here and not
// built with the mock's code, so the intake is tested against the shape and not
// against the mock.
func notificationJSON(id, bsn, ura, provision string, at time.Time) string {
	return fmt.Sprintf(`{
  "resourceType": "Bundle", "id": %q, "type": "transaction",
  "entry": [
    {"fullUrl": "urn:uuid:c", "resource": {"resourceType": "Consent", "id": "c", "status": "active",
      "patient": {"reference": "urn:uuid:p"}, "dateTime": %q,
      "provision": {"type": %q, "actor": [{"role": {"coding": [{"system": "http://terminology.hl7.org/CodeSystem/v3-ParticipationType", "code": "CST"}]}, "reference": {"reference": "urn:uuid:o"}}]}},
      "request": {"method": "POST", "url": "Consent"}},
    {"fullUrl": "urn:uuid:p", "resource": {"resourceType": "Patient", "id": "p",
      "identifier": [{"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": %q}]}},
    {"fullUrl": "urn:uuid:o", "resource": {"resourceType": "Organization", "id": "o",
      "identifier": [{"system": "http://fhir.nl/fhir/NamingSystem/ura", "value": %q}]}}
  ]
}`, id, at.Format(time.RFC3339Nano), provision, bsn, ura)
}

// deliverBody posts one notification to the intake and returns what it served.
// It asserts nothing, so a worker goroutine may call it.
func deliverBody(intake http.Handler, contentType, body string) (int, string) {
	req := httptest.NewRequest(http.MethodPost, "/mitz/notify", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	recorder := httptest.NewRecorder()
	intake.ServeHTTP(recorder, req)
	return recorder.Code, recorder.Body.String()
}

func deliver(t *testing.T, intake http.Handler, contentType, body string) int {
	t.Helper()
	status, _ := deliverBody(intake, contentType, body)
	return status
}

// goDeliver delivers on a goroutine of its own and hands back the status.
func goDeliver(intake http.Handler, body string) <-chan int {
	done := make(chan int, 1)
	go func() {
		status, _ := deliverBody(intake, "application/fhir+json", body)
		done <- status
	}()
	return done
}

func awaitStatus(t *testing.T, done <-chan int) int {
	t.Helper()
	select {
	case status := <-done:
		return status
	case <-time.After(5 * time.Second):
		t.Fatal("the delivery did not return")
		return 0
	}
}

type intakeFixture struct {
	cfg    Config
	srv    *httptest.Server
	client *http.Client
	intake http.Handler
	anna   pool.PoolPatient // pool.Patients()[0], the patient the session has open
	second pool.PoolPatient // pool.Patients()[1], a patient nobody has open
	owner  string
}

// newIntakeFixture is a signed-in sandbox with anna open and a copy of De
// Zonnebloem's data stored for that session.
//
// cfg is the Config the handlers were built from, not the one they use: they
// took a copy, and the session store exists only in theirs. What the tests
// share with the handlers is what cfg points to: the runtime state, the
// retrieval store, the runs and the locks. A method of Config that needs the
// session store does nothing when called on this cfg; go through the intake or
// the server instead.
func newIntakeFixture(t *testing.T) intakeFixture {
	t.Helper()
	cfg := retrievalConfig(t)
	cfg.Locks = NewRegistry()
	cfg.Runs = newRunStore(cfg.Locks)
	cfg.runtime = newRuntimeState()
	srv, client, intake := demoServerWithIntake(t, cfg)
	anna, second := pool.Patients()[0], pool.Patients()[1]
	openPatient(t, client, srv, anna.Key)
	owner := lockOwner(&authSession{ID: sessionCookieValue(t, client, srv.URL)})
	require.True(t, cfg.Retrievals.putIfCurrent(owner, anna.Key, cfg.Retrievals.generationOf(anna.Key),
		grantedFrom("Zorgcentrum De Zonnebloem", "Allergies")))
	return intakeFixture{cfg: cfg, srv: srv, client: client, intake: intake, anna: anna, second: second, owner: owner}
}

func (f intakeFixture) events(t *testing.T) []stepEvent {
	t.Helper()
	return sandbox{server: f.srv, client: f.client, key: f.anna.Key}.runEvents(t)
}

func TestIntake_AWithdrawalRemovesTheCopyRecordsTheNotificationAndPublishesAnEvent(t *testing.T) {
	f := newIntakeFixture(t)
	revisionBefore, _ := f.cfg.runtime.recordState(f.anna.Key)

	status := deliver(t, f.intake, "application/fhir+json", notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", t0))

	require.Equal(t, http.StatusNoContent, status)
	require.False(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key))
	require.Equal(t, []string{"Allergies"}, f.cfg.Retrievals.marks(f.anna.Key).Sections)
	notification, ok := f.cfg.runtime.notification(f.anna.Key)
	require.True(t, ok)
	require.Equal(t, "deny", notification.Decision)
	require.True(t, notification.ConsentTime.Equal(t0))
	revisionAfter, _ := f.cfg.runtime.recordState(f.anna.Key)
	require.Greater(t, revisionAfter, revisionBefore)

	var inbound []stepEvent
	for _, event := range f.events(t) {
		if event.Purpose == "consent-notification" {
			inbound = append(inbound, event)
		}
	}
	require.Len(t, inbound, 1)
	event := inbound[0]
	require.Equal(t, "consent", event.GF)
	require.Equal(t, "mitz", event.Actor)
	require.Equal(t, "consent", event.Action)
	require.NotEmpty(t, event.ActionID, "a notification with no pending action forms an action of its own")
	require.NotEmpty(t, event.CallID)
	require.Equal(t, http.MethodPost, event.Request.Method)
	require.Equal(t, "/mitz/notify", event.Request.Path)
	require.Equal(t, http.StatusNoContent, event.Response.Status)
	require.Equal(t, "ok", event.Outcome)
	require.NotNil(t, event.Response.CopyRemoved)
	require.True(t, *event.Response.CopyRemoved)
	require.NotContains(t, fmt.Sprint(event.Request.Body), f.anna.BSN, "the BSN is redacted in an event like in any other")
}

func TestIntake_ARestorationClearsTheMarks(t *testing.T) {
	f := newIntakeFixture(t)
	deliver(t, f.intake, "application/fhir+json", notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", t0))
	status := deliver(t, f.intake, "application/fhir+json", notificationJSON("change-2", f.anna.BSN, plataanURA, "permit", t0.Add(time.Second)))

	require.Equal(t, http.StatusNoContent, status)
	require.Empty(t, f.cfg.Retrievals.marks(f.anna.Key).Sources)
	notification, _ := f.cfg.runtime.notification(f.anna.Key)
	require.Equal(t, "permit", notification.Decision)
	events := f.events(t)
	last := events[len(events)-1]
	require.Equal(t, "consent-notification", last.Purpose)
	require.False(t, *last.Response.CopyRemoved, "a restoration removes nothing")
}

func TestIntake_ADuplicateAndAnOlderNotificationChangeNothing(t *testing.T) {
	f := newIntakeFixture(t)
	deliver(t, f.intake, "application/fhir+json", notificationJSON("change-2", f.anna.BSN, plataanURA, "permit", t0.Add(time.Second)))
	before := len(f.events(t))

	for _, body := range []string{
		notificationJSON("change-2", f.anna.BSN, plataanURA, "permit", t0.Add(time.Second)), // delivered twice
		notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", t0),                    // overtaken
	} {
		require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json", body), "it is acknowledged all the same")
	}

	require.True(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key), "the overtaken withdrawal removed nothing")
	notification, _ := f.cfg.runtime.notification(f.anna.Key)
	require.Equal(t, "permit", notification.Decision)
	require.Len(t, f.events(t), before, "and published nothing")
}

// The valid notification every case starts from is one for anna, whom the
// fixture has open with a copy stored. A case this application wrongly accepted
// would therefore remove that copy and record a notification, which is what the
// assertions after each case look for.
func TestIntake_RejectsEverythingOutsideTheAcceptedShapeAndChangesNothing(t *testing.T) {
	const fhirJSON = "application/fhir+json"
	anna, second := pool.Patients()[0], pool.Patients()[1]
	valid := notificationJSON("change-1", anna.BSN, plataanURA, "deny", t0)
	replaced := func(old, new string) string {
		t.Helper()
		require.Contains(t, valid, old, "the fixture does not contain what this case replaces")
		return strings.Replace(valid, old, new, 1)
	}
	bsnIdentifier := `{"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": "` + anna.BSN + `"}`
	uraIdentifier := `{"system": "http://fhir.nl/fhir/NamingSystem/ura", "value": "` + plataanURA + `"}`
	for name, tc := range map[string]struct {
		contentType string
		body        string
		status      int
		reason      string // what the served body says is wrong
	}{
		"xml":                    {"application/fhir+xml", `<Bundle/>`, http.StatusUnsupportedMediaType, "only application/fhir+json"},
		"no content type":        {"", valid, http.StatusUnsupportedMediaType, "only application/fhir+json"},
		"not json":               {fhirJSON, `{`, http.StatusBadRequest, "not JSON"},
		"not a transaction":      {fhirJSON, replaced(`"transaction"`, `"searchset"`), http.StatusBadRequest, "not a transaction Bundle"},
		"no bundle id":           {fhirJSON, replaced(`"id": "change-1", `, ``), http.StatusBadRequest, "the Bundle has no id"},
		"consent not active":     {fhirJSON, replaced(`"active"`, `"inactive"`), http.StatusBadRequest, `Consent.status is "inactive"`},
		"no provision type":      {fhirJSON, replaced(`"type": "deny", `, ``), http.StatusBadRequest, `Consent.provision.type is ""`},
		"another provision type": {fhirJSON, replaced(`"deny"`, `"maybe"`), http.StatusBadRequest, `Consent.provision.type is "maybe"`},
		"no dateTime":            {fhirJSON, replaced(`"dateTime": "2026-10-12T14:00:00Z",`, ``), http.StatusBadRequest, "Consent.dateTime is not a full timestamp"},
		"a date for a dateTime":  {fhirJSON, replaced(`2026-10-12T14:00:00Z`, `2026-10-12`), http.StatusBadRequest, "Consent.dateTime is not a full timestamp"},
		"two consents": {fhirJSON, replaced(`"entry": [`,
			`"entry": [{"fullUrl": "urn:uuid:c2", "resource": {"resourceType": "Consent", "id": "c2", "status": "active"}},`),
			http.StatusBadRequest, "expected exactly one Consent, found 2"},
		"no custodian":              {fhirJSON, replaced(`"code": "CST"`, `"code": "IRCP"`), http.StatusBadRequest, "expected exactly one custodian actor, found 0"},
		"patient reference dangles": {fhirJSON, replaced(`{"reference": "urn:uuid:p"}`, `{"reference": "urn:uuid:nobody"}`), http.StatusBadRequest, "must resolve to exactly one Patient, found 0"},
		"custodian dangles":         {fhirJSON, replaced(`{"reference": "urn:uuid:o"}`, `{"reference": "urn:uuid:nobody"}`), http.StatusBadRequest, "must resolve to exactly one Organization, found 0"},
		"two patients under one reference": {fhirJSON, replaced(`"entry": [`,
			`"entry": [{"fullUrl": "urn:uuid:p", "resource": {"resourceType": "Patient", "id": "p2", "identifier": [{"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": "`+second.BSN+`"}]}},`),
			http.StatusBadRequest, "must resolve to exactly one Patient, found 2"},
		"patient without a BSN":       {fhirJSON, replaced(`NamingSystem/bsn`, `NamingSystem/other`), http.StatusBadRequest, "the Patient must carry exactly one identifier in http://fhir.nl/fhir/NamingSystem/bsn"},
		"a BSN without a value":       {fhirJSON, replaced(bsnIdentifier, `{"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": ""}`), http.StatusBadRequest, "the Patient must carry exactly one identifier"},
		"two BSNs, one of them empty": {fhirJSON, replaced(bsnIdentifier, bsnIdentifier+`, {"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": ""}`), http.StatusBadRequest, "the Patient must carry exactly one identifier"},
		"two BSNs":                    {fhirJSON, replaced(bsnIdentifier, bsnIdentifier+`, {"system": "http://fhir.nl/fhir/NamingSystem/bsn", "value": "`+second.BSN+`"}`), http.StatusBadRequest, "the Patient must carry exactly one identifier"},
		"two URAs, one of them empty": {fhirJSON, replaced(uraIdentifier, uraIdentifier+`, {"system": "http://fhir.nl/fhir/NamingSystem/ura", "value": ""}`), http.StatusBadRequest, "the Organization must carry exactly one identifier"},
		"organization without a URA":  {fhirJSON, replaced(`NamingSystem/ura`, `NamingSystem/other`), http.StatusBadRequest, "the Organization must carry exactly one identifier in http://fhir.nl/fhir/NamingSystem/ura"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newIntakeFixture(t)
			revision, _ := f.cfg.runtime.recordState(f.anna.Key)
			before := len(f.events(t))

			status, served := deliverBody(f.intake, tc.contentType, tc.body)

			require.Equal(t, tc.status, status)
			require.Contains(t, served, tc.reason)
			require.True(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key))
			require.Empty(t, notifiedPatients(f.cfg.runtime), "nothing is recorded, for anna or for anybody else the body names")
			after, _ := f.cfg.runtime.recordState(f.anna.Key)
			require.Equal(t, revision, after)
			require.Len(t, f.events(t), before)
		})
	}
}

// An accepted consent time is the patient's watermark from then on, and a
// watermark only rises. One far ahead would shut out every genuine notification
// until the process restarts, so the intake refuses a time its own clock has not
// reached, give or take the skew between two machines.
func TestIntake_RefusesAConsentTimeAheadOfItsClock(t *testing.T) {
	f := newIntakeFixture(t)
	now := t0.Add(time.Hour)
	f.cfg.runtime.now = func() time.Time { return now }
	revision, _ := f.cfg.runtime.recordState(f.anna.Key)
	before := len(f.events(t))

	status, served := deliverBody(f.intake, "application/fhir+json",
		notificationJSON("ahead", f.anna.BSN, plataanURA, "deny", now.Add(consentTimeSkew+time.Nanosecond)))

	require.Equal(t, http.StatusBadRequest, status)
	require.Contains(t, served, "Consent.dateTime is ahead of this application's clock")
	require.True(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key), "the retrieved copy stays")
	require.Empty(t, notifiedPatients(f.cfg.runtime))
	after, _ := f.cfg.runtime.recordState(f.anna.Key)
	require.Equal(t, revision, after)
	require.Len(t, f.events(t), before)

	// The edge of the skew is a time another machine's clock can honestly give.
	require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
		notificationJSON("edge", f.anna.BSN, plataanURA, "deny", now.Add(consentTimeSkew))))
	notification, recorded := f.cfg.runtime.notification(f.anna.Key)
	require.True(t, recorded)
	require.True(t, notification.ConsentTime.Equal(now.Add(consentTimeSkew)))
}

// What the bound is for: a refused time leaves no watermark behind, so the
// changes that follow it are heard, before and after a reset.
func TestIntake_ATimeFarAheadDoesNotShutOutLaterNotifications(t *testing.T) {
	f := newIntakeFixture(t)
	now := t0.Add(time.Hour)
	f.cfg.runtime.now = func() time.Time { return now }

	require.Equal(t, http.StatusBadRequest, deliver(t, f.intake, "application/fhir+json",
		notificationJSON("forged", f.anna.BSN, plataanURA, "deny", now.AddDate(50, 0, 0))))

	require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
		notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", now.Add(-time.Minute))))
	notification, recorded := f.cfg.runtime.notification(f.anna.Key)
	require.True(t, recorded, "the genuine withdrawal is heard")
	require.Equal(t, "deny", notification.Decision)
	require.False(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key))

	f.cfg.runtime.reset(f.anna.Key, f.anna.BSN, func() { f.cfg.Retrievals.clearPatient(f.anna.Key) })
	require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
		notificationJSON("change-2", f.anna.BSN, plataanURA, "permit", now)))
	notification, recorded = f.cfg.runtime.notification(f.anna.Key)
	require.True(t, recorded, "and so is the change after the reset")
	require.Equal(t, "permit", notification.Decision)
}

func TestIntake_ResolvesAPatientReferenceByTypeAndIDToo(t *testing.T) {
	f := newIntakeFixture(t)
	body := strings.Replace(notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", t0),
		`{"reference": "urn:uuid:p"}`, `{"reference": "Patient/p"}`, 1)
	require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json", body))
	require.False(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key), "the package's own example refers to its Patient this way")
}

// Each case differs from an accepted notification in one thing only, so each
// rule is what refuses its own case: anna's BSN under another custodian, and De
// Plataan as custodian of a BSN the pool does not have.
func TestIntake_AValidNotificationThatIsNotForThisApplicationIsAcknowledgedAndIgnored(t *testing.T) {
	anna := pool.Patients()[0]
	for name, body := range map[string]string{
		"another organization":   notificationJSON("change-1", anna.BSN, "00000030", "deny", t0),
		"a BSN outside the pool": notificationJSON("change-1", "999911120", plataanURA, "deny", t0),
	} {
		t.Run(name, func(t *testing.T) {
			f := newIntakeFixture(t)
			revision, _ := f.cfg.runtime.recordState(f.anna.Key)
			require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json", body))
			require.True(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key))
			require.Empty(t, notifiedPatients(f.cfg.runtime), "nothing is recorded under any key, an empty one included")
			after, _ := f.cfg.runtime.recordState(f.anna.Key)
			require.Equal(t, revision, after)
		})
	}
}

func TestIntake_RemovesTheCopyOfEverySession(t *testing.T) {
	f := newIntakeFixture(t)
	require.True(t, f.cfg.Retrievals.putIfCurrent("another-owner", f.anna.Key, f.cfg.Retrievals.generationOf(f.anna.Key),
		grantedFrom("Zorgcentrum De Zonnebloem", "Conditions")))
	deliver(t, f.intake, "application/fhir+json", notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", t0))
	require.False(t, f.cfg.Retrievals.holds("another-owner", f.anna.Key))
	require.Equal(t, []string{"Allergies", "Conditions"}, f.cfg.Retrievals.marks(f.anna.Key).Sections)
}

// Review Focus 5: nobody has the patient open.
func TestIntake_WithNobodyHoldingThePatientItIsRecordedAndPublishesNothing(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.Locks = NewRegistry()
	cfg.Runs = newRunStore(cfg.Locks)
	cfg.runtime = newRuntimeState()
	_, _, intake := demoServerWithIntake(t, cfg)
	anna, _ := pool.PatientByKey("anna")

	require.Equal(t, http.StatusNoContent, deliver(t, intake, "application/fhir+json", notificationJSON("change-1", anna.BSN, plataanURA, "deny", t0)))

	notification, ok := cfg.runtime.notification("anna")
	require.True(t, ok, "it is a fact about the patient, whoever is looking")
	require.Equal(t, "deny", notification.Decision)
}

// A notification accepted before a reset, whose event is only published after
// it. The reset is made to land exactly there, between the transition and the
// publication, by the probe at that point.
func TestIntake_AnEventAcceptedBeforeAResetIsNotPublishedAfterIt(t *testing.T) {
	f := newIntakeFixture(t)
	before := len(f.events(t))

	// The control: with nothing in between, this fixture publishes.
	require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
		notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", t0)))
	require.Len(t, f.events(t), before+1)

	setProbe(t, f.cfg.runtime, func(point, patientKey string) {
		if point == probeIntakeAccepted {
			f.cfg.runtime.reset(patientKey, f.anna.BSN, nil)
		}
	})
	require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
		notificationJSON("change-2", f.anna.BSN, plataanURA, "permit", t0.Add(time.Second))))

	require.Len(t, f.events(t), before+1, "accepted before the reset, so not published after it")
	_, notified := f.cfg.runtime.notification(f.anna.Key)
	require.False(t, notified, "and the reset cleared what the notification had recorded")
}

// The check of the reset counter and the append are one step as far as a reset
// is concerned. A recycle leaves a run that was opened during its restore in
// place, so without this a reset could land after the check and the event would
// still reach a run that outlives it.
func TestIntake_AResetCannotLandBetweenTheCheckAndTheAppend(t *testing.T) {
	f := newIntakeFixture(t)
	before := len(f.events(t))
	reached := false
	// deliver runs the intake on this goroutine, so the probe does too.
	setProbe(t, f.cfg.runtime, func(point, _ string) {
		if point != probePublishing {
			return
		}
		reached = true
		if f.cfg.runtime.publication.TryLock() {
			f.cfg.runtime.publication.Unlock()
			t.Error("a reset could make its transition after the counter was checked and before the event was appended")
		}
	})

	require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
		notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", t0)))

	require.True(t, reached, "the event went through the fenced publication")
	require.Len(t, f.events(t), before+1)
}

// Two notifications for one patient handled at the same time, in both orders of
// arrival at the transition. Whichever handler finishes last, the newer
// notification is the one that stands.
func TestIntake_AnOlderAndANewerNotificationHandledAtOnceEndInTheNewerState(t *testing.T) {
	for name, tc := range map[string]struct {
		heldBack    string // which of the two waits before its transition
		copyRemoved []bool // per event published, in order: whether it removed the copy
		copyKept    bool   // whether the session's copy is still there at the end
	}{
		"the older one is overtaken while it waits": {"older", []bool{false}, true},
		"the newer one waits for the older":         {"newer", []bool{true, false}, false},
	} {
		t.Run(name, func(t *testing.T) {
			f := newIntakeFixture(t)
			before := len(f.events(t))
			revision, _ := f.cfg.runtime.recordState(f.anna.Key)
			bodies := map[string]string{
				"older": notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", t0),
				"newer": notificationJSON("change-2", f.anna.BSN, plataanURA, "permit", t0.Add(time.Second)),
			}
			free := map[string]string{"older": "newer", "newer": "older"}[tc.heldBack]
			hold := newHoldPoint(t)
			setProbe(t, f.cfg.runtime, func(point, _ string) {
				if point == probeIntakeArrived {
					hold.pass() // the first handler to get here waits; the other goes through
				}
			})

			held := goDeliver(f.intake, bodies[tc.heldBack])
			hold.awaitArrival(t)
			require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json", bodies[free]))
			hold.release()
			require.Equal(t, http.StatusNoContent, awaitStatus(t, held))

			notification, _ := f.cfg.runtime.notification(f.anna.Key)
			require.Equal(t, "permit", notification.Decision)
			require.True(t, notification.ConsentTime.Equal(t0.Add(time.Second)))
			require.Empty(t, f.cfg.Retrievals.marks(f.anna.Key).Sources)
			require.Equal(t, tc.copyKept, f.cfg.Retrievals.holds(f.owner, f.anna.Key))
			after, _ := f.cfg.runtime.recordState(f.anna.Key)
			require.Equal(t, revision+uint64(len(tc.copyRemoved)), after, "one advance per notification that was accepted")
			var removed []bool
			for _, event := range f.events(t)[before:] {
				require.Equal(t, "consent-notification", event.Purpose)
				removed = append(removed, *event.Response.CopyRemoved)
			}
			require.Equal(t, tc.copyRemoved, removed)
		})
	}
}

func TestIntake_ANotificationOfAPendingChangeIsHandedToItsAction(t *testing.T) {
	f := newIntakeFixture(t)
	f.cfg.runtime.openPending(f.anna.Key, pendingConsent{actionID: "action-a", runID: "run", changeID: "change-1"})
	before := len(f.events(t))

	deliver(t, f.intake, "application/fhir+json", notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", t0))

	require.Len(t, f.events(t), before, "the action publishes it, after its own event")
	handed := f.cfg.runtime.closePending(f.anna.Key, "change-1")
	require.Len(t, handed, 1)
	require.Equal(t, "consent-notification", handed[0].Purpose)
}

func TestIntake_ANotificationOfAnotherChangeIsNotAdoptedByAPendingAction(t *testing.T) {
	f := newIntakeFixture(t)
	f.cfg.runtime.openPending(f.anna.Key, pendingConsent{actionID: "action-b", runID: "run", changeID: "change-2"})
	before := len(f.events(t))

	deliver(t, f.intake, "application/fhir+json", notificationJSON("change-1", f.anna.BSN, plataanURA, "deny", t0))

	require.Empty(t, f.cfg.runtime.closePending(f.anna.Key, "change-2"), "the later action did not cause it")
	events := f.events(t)
	require.Len(t, events, before+1, "it is published on its own")
	require.NotEqual(t, "action-b", events[len(events)-1].ActionID)
}

func TestIntake_RefusesABodyOverTheLimit(t *testing.T) {
	f := newIntakeFixture(t)
	big := `{"resourceType":"Bundle","x":"` + strings.Repeat("a", 1<<20) + `"}`
	require.Equal(t, http.StatusRequestEntityTooLarge, deliver(t, f.intake, "application/fhir+json", big))
}
