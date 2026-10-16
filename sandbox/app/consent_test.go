package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
)

// consentMock stands in for the mock Mitz's control API, over HTTP, so that the
// sandbox's own two clients and the capture transport are on the path: the
// change's event is produced the way a running stack produces it. A change it
// stores is notified to the intake when deliver is set, before the call is
// answered, as the real mock does.
type consentMock struct {
	server *httptest.Server

	mu          sync.Mutex
	decision    map[string]string
	calls       []string  // "bsn decision", in the order they were stored
	clock       time.Time // the changedAt of the latest change
	failSet     bool      // the PUT answers 503 and stores nothing
	loseSet     bool      // the PUT stores, notifies, and then answers 503
	failState   bool      // the GET answers 503
	deliver     func(changeID, bsn, provision string, at time.Time)
	answerAfter <-chan struct{} // non-nil: the notification goes out on a goroutine of its own, and the PUT answers once this is closed
	arrived     chan struct{}   // non-nil: one value per PUT, sent when it arrives
	held        chan struct{}   // non-nil: a PUT waits here until letGo

	// closing is closed when the test is over. Every wait in a handler also
	// waits on it, so no handler outlives the test whatever went wrong.
	closing    chan struct{}
	deliveries sync.WaitGroup
	release    sync.Once
}

func newConsentMock(t *testing.T) *consentMock {
	t.Helper()
	mock := &consentMock{decision: map[string]string{}, clock: t0, closing: make(chan struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /mock/consent/{bsn}", mock.put)
	mux.HandleFunc("GET /mock/consent/{bsn}", mock.get)
	mock.server = httptest.NewServer(mux)
	t.Cleanup(mock.server.Close)
	return mock
}

// shutDown ends every wait in the mock's handlers and waits for the deliveries
// it started. The fixture registers it after the sandbox's server exists, so it
// runs before that server closes: a consent action still waiting on a held PUT
// would otherwise keep the sandbox's Close waiting, and this would never run.
func (m *consentMock) shutDown() {
	m.release.Do(func() { close(m.closing) })
	m.deliveries.Wait()
}

// wait blocks until signal is closed or sent on, the request is gone, or the
// test is over.
func (m *consentMock) wait(r *http.Request, signal <-chan struct{}) {
	select {
	case <-signal:
	case <-r.Context().Done():
	case <-m.closing:
	}
}

// with changes the mock's behaviour under its lock.
func (m *consentMock) with(change func(m *consentMock)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	change(m)
}

// hold makes every PUT report its arrival and then wait for letGo.
func (m *consentMock) hold() {
	m.with(func(m *consentMock) { m.arrived, m.held = make(chan struct{}, 8), make(chan struct{}) })
}

// letGo releases every PUT that is held, and every later one.
func (m *consentMock) letGo() {
	m.mu.Lock()
	held := m.held
	m.held = nil
	m.mu.Unlock()
	if held != nil {
		close(held)
	}
}

func (m *consentMock) awaitArrival(t *testing.T) {
	t.Helper()
	m.mu.Lock()
	arrived := m.arrived
	m.mu.Unlock()
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("no consent change reached the mock")
	}
}

func (m *consentMock) callsMade() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.calls...)
}

// changedAt is the timestamp of the latest change the mock stored.
func (m *consentMock) changedAt() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.clock
}

func (m *consentMock) put(w http.ResponseWriter, r *http.Request) {
	bsn := r.PathValue("bsn")
	var request struct {
		Decision string `json:"decision"`
		ChangeID string `json:"changeId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	m.mu.Lock()
	arrived, held := m.arrived, m.held
	m.mu.Unlock()
	if arrived != nil {
		arrived <- struct{}{}
	}
	if held != nil {
		m.wait(r, held)
	}

	m.mu.Lock()
	fail, lose, deliver, answerAfter := m.failSet, m.loseSet, m.deliver, m.answerAfter
	var at time.Time
	if !fail {
		m.clock = m.clock.Add(time.Second)
		at = m.clock
		m.decision[bsn] = request.Decision
		m.calls = append(m.calls, bsn+" "+request.Decision)
	}
	m.mu.Unlock()

	if deliver != nil && !fail {
		provision := "permit"
		if request.Decision == "Deny" {
			provision = "deny"
		}
		if answerAfter != nil {
			m.deliveries.Go(func() { deliver(request.ChangeID, bsn, provision, at) })
			m.wait(r, answerAfter)
		} else {
			deliver(request.ChangeID, bsn, provision, at)
		}
	}
	if fail || lose {
		http.Error(w, "the answer is lost", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"bsn": bsn, "decision": request.Decision, "changeId": request.ChangeID, "changedAt": at})
}

func (m *consentMock) get(w http.ResponseWriter, r *http.Request) {
	bsn := r.PathValue("bsn")
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failState {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	body := map[string]any{"bsn": bsn, "decision": "Permit", "overridden": false}
	if decision, ok := m.decision[bsn]; ok {
		body["decision"], body["overridden"], body["changedAt"] = decision, true, m.clock
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(body)
}

type consentFixture struct {
	intakeFixture
	mock *consentMock
}

// newConsentFixture is the intake fixture with the mock's control API behind the
// sandbox's real clients, delivering each change to the intake as a real stack
// does. configure runs before the handlers are built: they copy the Config, so
// nothing assigned to it afterwards reaches them.
func newConsentFixture(t *testing.T, notified bool, configure ...func(*Config)) consentFixture {
	t.Helper()
	mock := newConsentMock(t)
	mockURL, err := url.Parse(mock.server.URL)
	require.NoError(t, err)
	cfg := retrievalConfig(t)
	cfg.Locks = NewRegistry()
	cfg.Runs = newRunStore(cfg.Locks)
	cfg.runtime = newRuntimeState()
	cfg.mitzSetConsent, cfg.mitzConsentState = mitzSetConsentFunc(mockURL), mitzConsentStateFunc(mockURL)
	for _, option := range configure {
		option(&cfg)
	}
	srv, client, intake := demoServerWithIntake(t, cfg)
	// After the sandbox's server, so before it at cleanup. A hold point a test
	// creates later is released before this in turn, which lets a delivery that
	// is parked there finish before shutDown waits for it.
	t.Cleanup(mock.shutDown)
	if notified {
		mock.with(func(m *consentMock) {
			m.deliver = func(changeID, bsn, provision string, at time.Time) {
				deliverBody(intake, "application/fhir+json", notificationJSON(changeID, bsn, plataanURA, provision, at))
			}
		})
	}
	anna, second := pool.Patients()[0], pool.Patients()[1]
	openPatient(t, client, srv, anna.Key)
	owner := lockOwner(&authSession{ID: sessionCookieValue(t, client, srv.URL)})
	require.True(t, cfg.Retrievals.putIfCurrent(owner, anna.Key, cfg.Retrievals.generationOf(anna.Key),
		grantedFrom("Zorgcentrum De Zonnebloem", "Allergies")))
	return consentFixture{intakeFixture{cfg: cfg, srv: srv, client: client, intake: intake, anna: anna, second: second, owner: owner}, mock}
}

func (f consentFixture) record() string { return "/demo/ehr/patients/" + f.anna.Key }

func (f consentFixture) change(t *testing.T, decision string) (int, string) {
	t.Helper()
	res := postForm(t, f.client, f.srv, f.record()+"/consent", url.Values{"decision": {decision}})
	defer res.Body.Close()
	return res.StatusCode, res.Header.Get("Location")
}

// changeResult is what a submission made on a worker goroutine came to. A
// worker does not assert; the test goroutine does, on this.
type changeResult struct {
	status   int
	location string
	err      error
}

// goChange submits the form on a goroutine of its own.
func (f consentFixture) goChange(decision string) <-chan changeResult {
	done := make(chan changeResult, 1)
	go func() {
		res, err := f.client.Post(f.srv.URL+f.record()+"/consent", "application/x-www-form-urlencoded",
			strings.NewReader(url.Values{"decision": {decision}}.Encode()))
		if err != nil {
			done <- changeResult{err: err}
			return
		}
		res.Body.Close()
		done <- changeResult{status: res.StatusCode, location: res.Header.Get("Location")}
	}()
	return done
}

func awaitChange(t *testing.T, done <-chan changeResult) changeResult {
	t.Helper()
	select {
	case result := <-done:
		require.NoError(t, result.err)
		return result
	case <-time.After(10 * time.Second):
		t.Fatal("the submission did not return")
		return changeResult{}
	}
}

// awaitDone waits for wait to return, and fails the test when it does not.
func awaitDone(t *testing.T, wait func(), what string) {
	t.Helper()
	done := make(chan struct{})
	go func() { wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal(what + " did not finish")
	}
}

func purposesOf(events []stepEvent) []string {
	purposes := []string{}
	for _, event := range events {
		purposes = append(purposes, event.Purpose)
	}
	return purposes
}

func TestConsent_AWithdrawalThatIsNotifiedRemovesTheCopyAndSaysSo(t *testing.T) {
	f := newConsentFixture(t, true)

	status, location := f.change(t, "deny")

	require.Equal(t, http.StatusSeeOther, status)
	require.Equal(t, f.record()+"?notice=consent-withdrawn", location)
	require.Equal(t, []string{f.anna.BSN + " Deny"}, f.mock.callsMade())
	require.False(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key))
}

func TestConsent_TheActionPublishesItsChangeAndThenItsNotification(t *testing.T) {
	f := newConsentFixture(t, true)
	before := len(f.events(t))

	f.change(t, "deny")

	events := f.events(t)[before:]
	require.Equal(t, []string{"consent-change", "consent-notification"}, purposesOf(events),
		"the change first, although the notification arrived while the change was still waiting")
	change, notification := events[0], events[1]
	require.Equal(t, "consent", change.GF)
	require.Equal(t, http.MethodPut, change.Request.Method)
	require.Equal(t, "ok", change.Outcome)
	require.NotEmpty(t, change.ActionID)
	require.Equal(t, change.ActionID, notification.ActionID, "one action")
	require.Equal(t, "consent", change.Action)
	require.Equal(t, "consent", notification.Action)
	require.Less(t, change.Seq, notification.Seq)
}

// Review Focus 3: nobody subscribed, so nothing is notified.
func TestConsent_AWithdrawalThatIsNotNotifiedKeepsTheCopyAndSaysSo(t *testing.T) {
	f := newConsentFixture(t, false)

	status, location := f.change(t, "deny")

	require.Equal(t, http.StatusSeeOther, status)
	require.Equal(t, f.record()+"?notice=consent-withdrawn-unnotified", location)
	require.True(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key), "the notification is what removes the copy, and none came")
	require.Equal(t, []string{f.anna.BSN + " Deny"}, f.mock.callsMade(), "consent changed at Mitz all the same")
}

func TestConsent_ARestorationHasItsOwnNotices(t *testing.T) {
	notified := newConsentFixture(t, true)
	notified.change(t, "deny")
	_, location := notified.change(t, "permit")
	require.Equal(t, notified.record()+"?notice=consent-restored", location)
	require.Empty(t, notified.cfg.Retrievals.marks(notified.anna.Key).Sources)

	silent := newConsentFixture(t, false)
	_, location = silent.change(t, "permit")
	require.Equal(t, silent.record()+"?notice=consent-restored-unnotified", location)
}

// Review Focus 4: the same click twice.
func TestConsent_ASecondIdenticalWithdrawalKeepsTheMarksAndEndsTheSame(t *testing.T) {
	f := newConsentFixture(t, true)
	f.change(t, "deny")
	marks := f.cfg.Retrievals.marks(f.anna.Key)
	require.NotEmpty(t, marks.Sections)

	_, location := f.change(t, "deny")

	require.Equal(t, f.record()+"?notice=consent-withdrawn", location)
	require.Equal(t, marks, f.cfg.Retrievals.marks(f.anna.Key))
	require.Equal(t, uint64(2), f.cfg.runtime.receivedCount(f.anna.Key), "the second is a newer notification, not a duplicate")
}

func TestConsent_AFailedCallIsReconciledAgainstWhatMitzNowAnswers(t *testing.T) {
	t.Run("the change did not happen", func(t *testing.T) {
		f := newConsentFixture(t, true)
		f.mock.with(func(m *consentMock) { m.failSet = true })
		_, location := f.change(t, "deny")
		require.Equal(t, f.record()+"?notice=consent-failed", location)
		require.True(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key))
		require.Empty(t, f.mock.callsMade())
	})
	t.Run("the change happened and the answer was lost", func(t *testing.T) {
		f := newConsentFixture(t, false)
		f.mock.with(func(m *consentMock) { m.loseSet = true })
		_, location := f.change(t, "deny")
		require.Equal(t, f.record()+"?notice=consent-withdrawn-unnotified", location)
		require.Equal(t, []string{f.anna.BSN + " Deny"}, f.mock.callsMade())
	})
	t.Run("Mitz cannot be asked", func(t *testing.T) {
		f := newConsentFixture(t, true)
		f.mock.with(func(m *consentMock) { m.failSet, m.failState = true, true })
		_, location := f.change(t, "deny")
		require.Equal(t, f.record()+"?notice=consent-unknown", location)
	})
}

// The mock stored the change and notified, and then its answer was lost. Both
// events are in the run, the failed change first.
func TestConsent_AFailedCallStillPublishesItsEventsInOrder(t *testing.T) {
	f := newConsentFixture(t, true)
	before := len(f.events(t))
	f.mock.with(func(m *consentMock) { m.loseSet = true })

	_, location := f.change(t, "deny")

	require.Equal(t, f.record()+"?notice=consent-withdrawn", location, "the sandbox received the notification itself")
	events := f.events(t)[before:]
	require.Equal(t, []string{"consent-change", "consent-notification"}, purposesOf(events))
	require.Equal(t, "error", events[0].Outcome)
	require.Equal(t, http.StatusServiceUnavailable, events[0].Response.Status)
	require.Equal(t, events[0].ActionID, events[1].ActionID, "handed to the action and published by it, although its call failed")
}

// A state read that answers the opposite of what was asked still tells the
// sandbox a timestamp Mitz issued. A later reset that cannot reach the mock has
// to know it, or the notification of that earlier change would be accepted
// after the reset.
func TestConsent_AFailedChangeStillRemembersTheTimestampItRead(t *testing.T) {
	f := newConsentFixture(t, false)
	issued := t0.Add(time.Hour)
	f.mock.with(func(m *consentMock) {
		m.decision[f.anna.BSN], m.clock = "Permit", issued // an earlier change the sandbox never heard of
		m.failSet = true
	})

	_, location := f.change(t, "deny")
	require.Equal(t, f.record()+"?notice=consent-failed", location)

	// A reset that got no watermark from the mock falls back on what the sandbox saw.
	f.cfg.runtime.reset(f.anna.Key, f.anna.BSN, nil)
	require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
		notificationJSON("earlier", f.anna.BSN, plataanURA, "permit", issued)))
	require.Empty(t, notifiedPatients(f.cfg.runtime), "that change was made before the reset, and the sandbox had read its timestamp")
}

func TestConsent_Guards(t *testing.T) {
	t.Run("a decision that is neither", func(t *testing.T) {
		f := newConsentFixture(t, true)
		status, body := postFormAndRead(t, f.client, f.srv, f.record()+"/consent", url.Values{"decision": {"maybe"}})
		require.Equal(t, http.StatusBadRequest, status)
		require.Contains(t, body, `decision must be "deny" or "permit"`)
		require.Empty(t, f.mock.callsMade())
	})
	t.Run("cross-site", func(t *testing.T) {
		f := newConsentFixture(t, true)
		req, err := http.NewRequest(http.MethodPost, f.srv.URL+f.record()+"/consent", strings.NewReader("decision=deny"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		res, err := f.client.Do(req)
		require.NoError(t, err)
		res.Body.Close()
		require.Equal(t, http.StatusForbidden, res.StatusCode)
		require.Empty(t, f.mock.callsMade())
	})
	t.Run("an unknown patient", func(t *testing.T) {
		f := newConsentFixture(t, true)
		status, body := postFormAndRead(t, f.client, f.srv, "/demo/ehr/patients/nobody/consent", url.Values{"decision": {"deny"}})
		require.Equal(t, http.StatusNotFound, status)
		require.Contains(t, body, "unknown patient: nobody")
	})
	t.Run("a patient this session does not hold", func(t *testing.T) {
		f := newConsentFixture(t, true)
		status, body := postFormAndRead(t, f.client, f.srv, "/demo/ehr/patients/"+f.second.Key+"/consent", url.Values{"decision": {"deny"}})
		require.Equal(t, http.StatusConflict, status)
		require.Contains(t, body, "this session does not hold patient "+f.second.Key)
		require.Empty(t, f.mock.callsMade())
	})
	t.Run("no mock configured", func(t *testing.T) {
		cfg := retrievalConfig(t)
		srv, client := demoServer(t, cfg)
		anna := pool.Patients()[0]
		openPatient(t, client, srv, anna.Key)
		res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/consent", url.Values{"decision": {"deny"}})
		res.Body.Close()
		require.Equal(t, http.StatusSeeOther, res.StatusCode)
		require.Equal(t, "/demo/ehr/patients/"+anna.Key+"?notice=mitz-disabled", res.Header.Get("Location"))
	})
}

// An action holds its patient's mutex while it calls Mitz, and that is what
// makes a second submission for the same patient wait its turn. The test reads
// the mutex; it does not wait to see that the second one stays away.
func TestConsent_TwoSubmissionsForOnePatientRunInTurn(t *testing.T) {
	f := newConsentFixture(t, true)
	f.mock.hold()

	first := f.goChange("deny")
	f.mock.awaitArrival(t)
	if lock := f.cfg.runtime.consentLock(f.anna.Key); lock.TryLock() {
		lock.Unlock()
		t.Fatal("the patient's consent mutex is free while an action for that patient is calling Mitz")
	}
	second := f.goChange("permit")

	f.mock.letGo()
	require.Equal(t, http.StatusSeeOther, awaitChange(t, first).status)
	require.Equal(t, http.StatusSeeOther, awaitChange(t, second).status)
	require.Equal(t, []string{f.anna.BSN + " Deny", f.anna.BSN + " Permit"}, f.mock.callsMade())
}

// A reset takes the gate for writing (Task 12), so this is what makes it wait
// for an action that is changing Mitz.
func TestConsent_ARunningActionHoldsTheGateForReading(t *testing.T) {
	f := newConsentFixture(t, true)
	f.mock.hold()

	running := f.goChange("deny")
	f.mock.awaitArrival(t)
	if f.cfg.runtime.gate.TryLock() {
		f.cfg.runtime.gate.Unlock()
		t.Fatal("a reset could restore Mitz while an action is changing it")
	}

	f.mock.letGo()
	require.Equal(t, http.StatusSeeOther, awaitChange(t, running).status)
}

// Submissions queued for one patient wait for its mutex without holding the
// gate, so a reset is delayed by the one action that is running and by none of
// those behind it. Two probes look at the locks from inside the action: where
// it queues it holds nothing, and when its turn has come it holds its patient's
// mutex and still not the gate. Together they pin the order of the two
// acquisitions, whichever way a change would swap them.
func TestConsent_AnActionTakesItsPatientsMutexBeforeTheGate(t *testing.T) {
	f := newConsentFixture(t, true)
	gate, patient := &f.cfg.runtime.gate, f.cfg.runtime.consentLock(f.anna.Key)
	var queued, turns atomic.Int32
	setProbe(t, f.cfg.runtime, func(point, _ string) {
		switch point {
		case probeConsentQueued:
			queued.Add(1)
			if !gate.TryLock() {
				t.Error("an action that has not had its turn yet already holds the gate")
			} else {
				gate.Unlock()
			}
		case probeConsentTurn:
			turns.Add(1)
			if patient.TryLock() {
				patient.Unlock()
				t.Error("the action's turn has come and it does not hold its patient's mutex")
			}
			if !gate.TryLock() {
				t.Error("the action took the gate before its patient's mutex was its own")
			} else {
				gate.Unlock()
			}
		}
	})

	status, _ := f.change(t, "deny")

	require.Equal(t, http.StatusSeeOther, status)
	require.Equal(t, int32(1), queued.Load(), "the action passed the point where it queues")
	require.Equal(t, int32(1), turns.Load(), "and the point where its turn has come")
}

// queuedSignal returns a channel that gets a value each time an action reaches
// the point where it queues: admitted once, and from there on only waiting.
func queuedSignal(t *testing.T, state *runtimeState) <-chan struct{} {
	t.Helper()
	queued := make(chan struct{}, 8)
	setProbe(t, state, func(point, _ string) {
		if point == probeConsentQueued {
			queued <- struct{}{}
		}
	})
	return queued
}

func awaitSignal(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal(what)
	}
}

func TestConsent_ASubmissionThatWaitedThroughAResetChangesNothing(t *testing.T) {
	f := newConsentFixture(t, true)
	queued := queuedSignal(t, f.cfg.runtime)
	f.cfg.runtime.gate.Lock() // a reset is running
	endReset := sync.OnceFunc(f.cfg.runtime.gate.Unlock)
	t.Cleanup(endReset)

	waiting := f.goChange("deny")
	awaitSignal(t, queued, "the submission was not admitted")

	// The reset ends every run and lease, then lets go.
	f.cfg.Runs.clear()
	f.cfg.Locks.ReleaseAll()
	endReset()

	result := awaitChange(t, waiting)
	require.Equal(t, http.StatusSeeOther, result.status)
	require.Equal(t, "/demo/ehr?notice=patient-not-open", result.location)
	require.Empty(t, f.mock.callsMade(), "the submission was admitted to a world the reset replaced")
}

func TestConsent_ASubmissionThatWaitedWhileThePatientWasSwitchedChangesNothing(t *testing.T) {
	f := newConsentFixture(t, true)
	queued := queuedSignal(t, f.cfg.runtime)
	lock := f.cfg.runtime.consentLock(f.anna.Key)
	lock.Lock() // an earlier action for anna is running
	endEarlier := sync.OnceFunc(lock.Unlock)
	t.Cleanup(endEarlier)

	waiting := f.goChange("deny")
	awaitSignal(t, queued, "the submission was not admitted")

	openPatient(t, f.client, f.srv, f.second.Key) // the session moves on; anna's lease and run go
	endEarlier()

	result := awaitChange(t, waiting)
	require.Equal(t, http.StatusSeeOther, result.status)
	require.Equal(t, "/demo/ehr?notice=patient-not-open", result.location)
	require.Empty(t, f.mock.callsMade())
}

// A notification can arrive after its action has ended: the mock answered, the
// action closed its entry, and only then is the notification's event let go. By
// then a later action for the same patient is waiting on Mitz. The late event
// belongs to neither: it is published once, on its own.
func TestConsent_ANotificationArrivingAfterItsActionEndedIsPublishedOnceOnItsOwn(t *testing.T) {
	f := newConsentFixture(t, true)
	before := len(f.events(t))
	hold := newHoldPoint(t)
	setProbe(t, f.cfg.runtime, func(point, _ string) {
		if point == probeIntakeAccepted {
			hold.pass() // the first notification waits here: accepted, its event not yet handed anywhere
		}
	})

	// The first action. Mitz answers once the notification is at the hold point.
	f.mock.with(func(m *consentMock) { m.answerAfter = hold.arrived })
	_, location := f.change(t, "deny")
	require.Equal(t, f.record()+"?notice=consent-withdrawn", location, "the notification was received, though its event is still on its way")

	// A second action for the same patient is now waiting on Mitz.
	f.mock.with(func(m *consentMock) { m.answerAfter = nil })
	f.mock.hold()
	second := f.goChange("permit")
	f.mock.awaitArrival(t)

	// The first notification's event is let go while the second action is pending.
	hold.release()
	awaitDone(t, f.mock.deliveries.Wait, "the late delivery")

	f.mock.letGo()
	require.Equal(t, f.record()+"?notice=consent-restored", awaitChange(t, second).location)

	events := f.events(t)[before:]
	require.Equal(t, []string{"consent-change", "consent-notification", "consent-change", "consent-notification"}, purposesOf(events),
		"each notification once: neither dropped nor published by two paths")
	firstChange, late, secondChange, own := events[0], events[1], events[2], events[3]
	require.NotEmpty(t, late.ActionID)
	require.NotEqual(t, firstChange.ActionID, late.ActionID, "its own action had ended")
	require.NotEqual(t, secondChange.ActionID, late.ActionID, "and the later action did not cause it")
	require.True(t, *late.Response.CopyRemoved)
	require.Equal(t, secondChange.ActionID, own.ActionID, "the later action's notification is its own")
	require.False(t, *own.Response.CopyRemoved)
}

func TestConsent_TheRevisionAdvancesWhateverTheOutcome(t *testing.T) {
	f := newConsentFixture(t, false)
	f.mock.with(func(m *consentMock) { m.failSet = true })
	before, _ := f.cfg.runtime.recordState(f.anna.Key)
	f.change(t, "deny")
	after, _ := f.cfg.runtime.recordState(f.anna.Key)
	require.Greater(t, after, before, "other open pages of this record must learn that the card may have changed")
}
