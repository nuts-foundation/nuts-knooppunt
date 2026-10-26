package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
)

// fakeConfig returns a Config whose reset/recycle functions record calls instead
// of touching a real HAPI/Knooppunt, so the handler behavior can be tested.
func fakeConfig() (Config, *[]string, *[]string) {
	var resets []string
	var recycles []string
	cfg := Config{
		KnooppuntInternalURL: "http://knooppunt:8081",
		HAPIBaseURL:          "http://hapi:7050/fhir",
		Locks:                NewRegistry(),
		resetGlobal: func(context.Context) error {
			resets = append(resets, "reset")
			return nil
		},
		recyclePatient: func(_ context.Context, key string) error {
			recycles = append(recycles, key)
			return nil
		},
	}
	return cfg, &resets, &recycles
}

// demoServer starts a sandbox against a fake Dezi and returns it with a
// signed-in client. Every demo control below requires a session, so a test that
// skipped signing in would only ever see the redirect to /demo/login.
func demoServer(t *testing.T, cfg Config) (*httptest.Server, *http.Client) {
	t.Helper()
	t.Setenv("DEZI_INTERNAL_BASE_URL", fakeDezi(t).URL)
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)
	return srv, signInViaDezi(t, srv)
}

// demoServerWithIntake is demoServer plus the notification intake built from
// the same runtime state, which is what main serves on its private listener.
func demoServerWithIntake(t *testing.T, cfg Config) (*httptest.Server, *http.Client, http.Handler) {
	t.Helper()
	t.Setenv("DEZI_INTERNAL_BASE_URL", fakeDezi(t).URL)
	app, intake := newHandlers(cfg)
	srv := httptest.NewServer(app)
	t.Cleanup(srv.Close)
	return srv, signInViaDezi(t, srv), intake
}

func postForm(t *testing.T, client *http.Client, srv *httptest.Server, path string, form url.Values) *http.Response {
	t.Helper()
	res, err := client.Post(srv.URL+path, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	return res
}

// postFormAndRead is postForm for the routes that render a page rather than
// redirect.
func postFormAndRead(t *testing.T, client *http.Client, srv *httptest.Server, path string, form url.Values) (int, string) {
	t.Helper()
	res := postForm(t, client, srv, path, form)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

func TestReset_RunsWhenUnlocked(t *testing.T) {
	cfg, resets, _ := fakeConfig()
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/reset", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=reset-done", res.Header.Get("Location"))
	require.Equal(t, []string{"reset"}, *resets)
}

func TestReset_RefusesWhenLockedWithoutOverride(t *testing.T) {
	cfg, resets, _ := fakeConfig()
	require.True(t, cfg.Locks.Lock("pool-02", "session-x"))
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/reset", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	loc := res.Header.Get("Location")
	require.Contains(t, loc, "notice=reset-locked")
	require.Contains(t, loc, "pool-02")
	require.Empty(t, *resets, "reset must not run while a patient is locked")
}

// A malformed form body must be rejected rather than silently read as "override
// not passed": for a reset that difference decides whether the dataset is wiped,
// so the caller has to be told the request was not understood.
func TestReset_MalformedFormIsRejected(t *testing.T) {
	cfg, resets, _ := fakeConfig()
	srv, client := demoServer(t, cfg)

	res, err := client.Post(srv.URL+"/demo/reset", "application/x-www-form-urlencoded", strings.NewReader("override=%zz"))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	require.Empty(t, *resets, "a request we could not parse must not trigger a reset")
}

func TestReset_ProceedsWithOverride(t *testing.T) {
	cfg, resets, _ := fakeConfig()
	require.True(t, cfg.Locks.Lock("pool-02", "session-x"))
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/reset", url.Values{"override": {"true"}})
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=reset-done", res.Header.Get("Location"))
	require.Equal(t, []string{"reset"}, *resets)
}

func TestReset_PartialResetIsReportedNotFailed(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.resetGlobal = func(context.Context) error {
		return fmt.Errorf("%w: Mitz subscriptions were not cleared", vectors.ErrPartialReset)
	}
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/reset", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=reset-partial", res.Header.Get("Location"))
}

// The recycle half of the split handleRecycle makes: fixtures restored, the
// consent subscription not cleared, which is neither a failure nor a clean run.
func TestRecycle_PartialRecycleIsReportedNotFailed(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.recyclePatient = func(context.Context, string) error {
		return fmt.Errorf("%w: the consent subscription was not cleared", vectors.ErrPartialReset)
	}
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/patients/"+pool.Patients()[0].Key+"/recycle", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=recycle-partial", res.Header.Get("Location"))
	_, body := getBody(t, client, srv, res.Header.Get("Location"))
	require.Contains(t, body, "Patient restored, but some state could not be cleared. Check the sandbox logs.")
	require.NotContains(t, body, "the consent subscription could not be cleared")
}

func TestRecycle_RefusesLockedPatient(t *testing.T) {
	cfg, _, recycles := fakeConfig()
	anna := pool.Patients()[0].Key
	require.True(t, cfg.Locks.Lock(anna, "session-x"))
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/patients/"+anna+"/recycle", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)
	require.Empty(t, *recycles)
}

func TestRecycle_SucceedsWhenAnotherPatientLocked(t *testing.T) {
	cfg, _, recycles := fakeConfig()
	patients := pool.Patients()
	anna, other := patients[0].Key, patients[1].Key
	require.True(t, cfg.Locks.Lock(other, "session-x"))
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/patients/"+anna+"/recycle", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=recycle-done", res.Header.Get("Location"))
	require.Equal(t, []string{anna}, *recycles)
}

// The lock check happens before the restore, which takes a while. A session
// that opens the patient during the restore must keep its run and its lock.
func TestRecycle_KeepsRunStartedDuringRestore(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.Runs = newRunStore(cfg.Locks)
	anna := pool.Patients()[0].Key
	other := &authSession{ID: "opened-during-recycle", ExpiresAt: time.Now().Add(time.Hour)}
	opened := make(chan error, 1)
	cfg.recyclePatient = func(context.Context, string) error {
		opened <- cfg.startPatientRun(other, anna, true)
		return nil
	}
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/patients/"+anna+"/recycle", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=recycle-done", res.Header.Get("Location"))
	require.NoError(t, <-opened)
	require.True(t, cfg.Locks.HeldBy(anna, lockOwner(other)), "the recycle released a lock it never checked")
	require.NotEmpty(t, cfg.Runs.current(lockOwner(other), anna), "the recycle removed a run it never checked")
}

func TestRecycle_UnknownPatientIs404(t *testing.T) {
	cfg, _, _ := fakeConfig()
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/patients/nope/recycle", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestLockAndReleaseEndpoints(t *testing.T) {
	cfg, _, _ := fakeConfig()
	anna := pool.Patients()[0].Key
	srv, client := demoServer(t, cfg)

	// A second practitioner, signed in separately. Ownership is derived from the
	// session, so this is the only way to be a different owner: the request body
	// no longer decides it.
	other := signInViaDezi(t, srv)

	res := postForm(t, client, srv, "/demo/patients/"+anna+"/lock", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	res2 := postForm(t, other, srv, "/demo/patients/"+anna+"/lock", nil)
	defer res2.Body.Close()
	require.Equal(t, http.StatusConflict, res2.StatusCode, "another session must not steal a held lock")

	res3 := postForm(t, other, srv, "/demo/patients/"+anna+"/release", nil)
	defer res3.Body.Close()
	require.Equal(t, http.StatusOK, res3.StatusCode)
	require.True(t, cfg.Locks.IsLocked(anna), "another session must not be able to release a lock it does not hold")

	res4 := postForm(t, client, srv, "/demo/patients/"+anna+"/release", nil)
	defer res4.Body.Close()
	require.Equal(t, http.StatusOK, res4.StatusCode)
	require.False(t, cfg.Locks.IsLocked(anna), "the holder releases its own lock")
}

// The four destructive controls were reachable without a session, so any
// browser on a shared deployment could reset the dataset under a running demo
// or reclaim its patient. The cross-site check they each carry says nothing
// about who the caller is.
func TestDemoControlsRequireASession(t *testing.T) {
	cfg, resets, recycles := fakeConfig()
	anna := pool.Patients()[0].Key
	srv, _ := demoServer(t, cfg)
	anonymous := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	for _, path := range []string{
		"/demo/reset",
		"/demo/patients/" + anna + "/recycle",
		"/demo/patients/" + anna + "/lock",
		"/demo/patients/" + anna + "/release",
	} {
		t.Run(path, func(t *testing.T) {
			res := postForm(t, anonymous, srv, path, nil)
			defer res.Body.Close()
			require.Equal(t, http.StatusSeeOther, res.StatusCode)
			require.Equal(t, "/demo/login", res.Header.Get("Location"))
		})
	}

	require.Empty(t, *resets, "an anonymous request must not have reset anything")
	require.Empty(t, *recycles, "an anonymous request must not have recycled anything")
	require.False(t, cfg.Locks.IsLocked(anna), "an anonymous request must not have taken a lock")
}

// Reset invalidates every session, and lock ownership is derived from the
// session, so a lock surviving a reset is one nobody can release: it would
// refuse recycle and the next non-override reset until the TTL ran out. Override
// reset is precisely the path that produces them, because it is the one that
// proceeds while locks are held.
func TestOverrideResetReleasesTheLocksItOrphans(t *testing.T) {
	cfg, resets, _ := fakeConfig()
	anna := pool.Patients()[0].Key
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/patients/"+anna+"/lock", nil)
	res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.True(t, cfg.Locks.IsLocked(anna))

	res2 := postForm(t, client, srv, "/demo/reset", url.Values{"override": {"true"}})
	res2.Body.Close()
	require.Equal(t, http.StatusSeeOther, res2.StatusCode)
	require.Equal(t, "/demo?notice=reset-done", res2.Header.Get("Location"))
	require.Len(t, *resets, 1)

	require.False(t, cfg.Locks.IsLocked(anna), "an override reset must not leave a lock its owner can no longer release")

	// And the patient is genuinely reusable, not merely reported unlocked.
	next := signInViaDezi(t, srv)
	res3 := postForm(t, next, srv, "/demo/patients/"+anna+"/lock", nil)
	defer res3.Body.Close()
	require.Equal(t, http.StatusOK, res3.StatusCode, "the next demo must be able to claim the patient")
}

// Ownership must come from the session and nowhere else. Every other lock test
// posts an empty body, so none of them would notice an implementation that read
// the session as a default but still honoured a supplied owner: that would let
// one session release another's lock by echoing back the owner /lock returned.
func TestLockIgnoresAnOwnerSuppliedByTheRequest(t *testing.T) {
	cfg, _, _ := fakeConfig()
	anna := pool.Patients()[0].Key
	srv, client := demoServer(t, cfg)
	other := signInViaDezi(t, srv)

	res := postForm(t, client, srv, "/demo/patients/"+anna+"/lock", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	var locked struct {
		Owner string `json:"owner"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&locked))
	require.NotEmpty(t, locked.Owner)

	// The holder's own owner value, replayed by a different session.
	res2 := postForm(t, other, srv, "/demo/patients/"+anna+"/release", url.Values{"owner": {locked.Owner}})
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)
	require.True(t, cfg.Locks.IsLocked(anna),
		"a supplied owner must not let another session release a lock it does not hold")
}

// The owner is echoed back in the lock response. Returning the session ID itself
// would put the cookie value, which is the credential, into a response body.
func TestLockOwnerIsNotTheSessionCookie(t *testing.T) {
	cfg, _, _ := fakeConfig()
	anna := pool.Patients()[0].Key
	srv, client := demoServer(t, cfg)
	raw := sessionCookieValue(t, client, srv.URL)

	res := postForm(t, client, srv, "/demo/patients/"+anna+"/lock", nil)
	defer res.Body.Close()
	var locked struct {
		Owner string `json:"owner"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&locked))
	require.NotEmpty(t, locked.Owner)
	require.NotEqual(t, raw, locked.Owner, "the lock owner must not be the session cookie value")
	require.NotContains(t, raw, locked.Owner, "the owner must not be a prefix of the session cookie either")
}

func TestLogout_ReleasesTheSessionsPatientLock(t *testing.T) {
	cfg, _, _ := fakeConfig()
	anna := pool.Patients()[0].Key
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/patients/"+anna+"/lock", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	out := postForm(t, client, srv, "/demo/logout", nil)
	defer out.Body.Close()

	require.False(t, cfg.Locks.IsLocked(anna), "signing out must not strand the patient for the lock TTL")
}

func TestListPatientsReflectsPoolAndLocks(t *testing.T) {
	cfg, _, _ := fakeConfig()
	anna := pool.Patients()[0].Key
	require.True(t, cfg.Locks.Lock(anna, "s1"))
	srv, _ := demoServer(t, cfg)

	// Deliberately anonymous: the pool listing is a read, and unlike the four
	// destructive controls it carries no session guard.
	res, err := http.Get(srv.URL + "/demo/patients")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var got []patientStatus
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.Len(t, got, len(pool.Patients()))

	byKey := map[string]patientStatus{}
	for _, p := range got {
		byKey[p.Key] = p
		require.NotEmpty(t, p.BSN)
		require.NotEmpty(t, p.Name)
	}
	require.True(t, byKey[anna].Locked, "locked patient must report locked")
}

// patientConsent is everything a reset has to clear or advance for one patient.
type patientConsent struct {
	notified bool
	marks    removal
	resets   uint64
	revision uint64
}

func patientConsentOf(cfg Config, patientKey string) patientConsent {
	_, notified := cfg.runtime.notification(patientKey)
	revision, _ := cfg.runtime.recordState(patientKey)
	return patientConsent{notified: notified, marks: cfg.Retrievals.marks(patientKey),
		resets: cfg.runtime.resetsOf(patientKey), revision: revision}
}

// served posts a form and returns the status, the redirect's location and the
// body, so a test can pin a refusal's reason and not only its status.
func served(t *testing.T, f consentFixture, path string, form url.Values) (int, string, string) {
	t.Helper()
	res := postForm(t, f.client, f.srv, path, form)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, res.Header.Get("Location"), string(body)
}

func TestReset_ClearsTheConsentStateAndRaisesTheWatermark(t *testing.T) {
	f := newConsentFixture(t, true)
	f.change(t, "deny")
	before := patientConsentOf(f.cfg, f.anna.Key)
	require.True(t, before.notified)
	require.NotEmpty(t, before.marks.Sources)
	changed := f.mock.changedAt()

	// What the mock's consent cleanup answers is later than every change stored
	// before it. The fixture's resetGlobal does not call the mock, so the test
	// reports the watermark the way vectors.ResetGlobal would.
	watermark := changed.Add(time.Hour)
	f.cfg.runtime.noteMockWatermark("", watermark)
	status, location, _ := served(t, f, "/demo/reset", url.Values{"override": {"true"}})
	require.Equal(t, http.StatusSeeOther, status)
	require.Equal(t, "/demo?notice=reset-done", location)

	after := patientConsentOf(f.cfg, f.anna.Key)
	require.False(t, after.notified)
	require.Empty(t, after.marks.Sources)
	require.Equal(t, before.resets+1, after.resets)
	require.Greater(t, after.revision, before.revision, "the revision advances and is never set back")

	// A change made before the reset that the sandbox never heard of. Its
	// timestamp is later than anything the sandbox saw and earlier than the
	// cleanup's watermark, so only the mock's watermark can tell that it is old.
	unseen := changed.Add(time.Minute)
	require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
		notificationJSON("unseen", f.anna.BSN, plataanURA, "deny", unseen)))
	require.Empty(t, notifiedPatients(f.cfg.runtime), "a change made before the reset is ignored however late its notification is handled")

	// A change made after the reset is accepted.
	require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
		notificationJSON("after", f.anna.BSN, plataanURA, "deny", watermark.Add(time.Second))))
	require.Equal(t, []string{f.anna.Key}, notifiedPatients(f.cfg.runtime))
}

// The stores are cleared and the counters advance on every outcome of the
// restore. No watermark comes from the mock in these cases, so the floor is
// what the sandbox itself saw of the patient.
func TestReset_ClearsTheConsentStateWhateverTheRestoreCameTo(t *testing.T) {
	for name, tc := range map[string]struct {
		restore  error
		status   int
		location string // of the redirect, when there is one
		reason   string // in the body, when there is none
	}{
		"clean":   {nil, http.StatusSeeOther, "/demo?notice=reset-done", ""},
		"partial": {fmt.Errorf("%w: the mock is unreachable", vectors.ErrPartialReset), http.StatusSeeOther, "/demo?notice=reset-partial", ""},
		"failed":  {errors.New("reload fixtures: hapi is down"), http.StatusInternalServerError, "", "reset failed: reload fixtures: hapi is down"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newConsentFixture(t, true, func(cfg *Config) {
				cfg.resetGlobal = func(context.Context) error { return tc.restore }
			})
			f.change(t, "deny")
			changed := f.mock.changedAt()
			before := map[string]patientConsent{}
			for _, patient := range pool.Patients() {
				before[patient.Key] = patientConsentOf(f.cfg, patient.Key)
			}
			require.True(t, before[f.anna.Key].notified)
			require.NotEmpty(t, before[f.anna.Key].marks.Sources)

			status, location, body := served(t, f, "/demo/reset", url.Values{"override": {"true"}})

			require.Equal(t, tc.status, status)
			require.Equal(t, tc.location, location)
			require.Contains(t, body, tc.reason)
			require.Empty(t, notifiedPatients(f.cfg.runtime), "a reset that failed may still have changed the mock, so nothing derived from it is kept")
			for _, patient := range pool.Patients() {
				after := patientConsentOf(f.cfg, patient.Key)
				require.Emptyf(t, after.marks.Sources, "%s", patient.Key)
				require.Equalf(t, before[patient.Key].resets+1, after.resets, "%s: the reset counter moves on every outcome", patient.Key)
				require.Greaterf(t, after.revision, before[patient.Key].revision, "%s: the revision advances and never goes back", patient.Key)
			}
			require.False(t, f.cfg.Retrievals.holds(f.owner, f.anna.Key))

			require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
				notificationJSON("replayed", f.anna.BSN, plataanURA, "deny", changed)))
			require.Empty(t, notifiedPatients(f.cfg.runtime), "the change the sandbox knew of is not accepted again")
			require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
				notificationJSON("newer", f.anna.BSN, plataanURA, "deny", changed.Add(time.Nanosecond))))
			require.Equal(t, []string{f.anna.Key}, notifiedPatients(f.cfg.runtime))
		})
	}
}

func TestRecycle_ClearsThatPatientsConsentStateWhateverTheRestoreCameTo(t *testing.T) {
	for name, tc := range map[string]struct {
		restore  error
		status   int
		location string
		reason   string
	}{
		"clean":   {nil, http.StatusSeeOther, "/demo?notice=recycle-done", ""},
		"partial": {fmt.Errorf("%w: the mock is unreachable", vectors.ErrPartialReset), http.StatusSeeOther, "/demo?notice=recycle-partial", ""},
		"failed":  {errors.New("put fixtures: hapi is down"), http.StatusInternalServerError, "", "recycle failed: put fixtures: hapi is down"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newConsentFixture(t, true, func(cfg *Config) {
				cfg.recyclePatient = func(context.Context, string) error { return tc.restore }
			})
			f.change(t, "deny")
			// Another pool patient with consent state of its own, and a copy that
			// another session retrieved about that patient.
			require.Equal(t, http.StatusNoContent, deliver(t, f.intake, "application/fhir+json",
				notificationJSON("other", f.second.BSN, plataanURA, "permit", t0.Add(time.Minute))))
			require.True(t, f.cfg.Retrievals.putIfCurrent("another-owner", f.second.Key, f.cfg.Retrievals.generationOf(f.second.Key),
				grantedFrom("Zorgcentrum De Zonnebloem", "Conditions")))
			before, otherBefore := patientConsentOf(f.cfg, f.anna.Key), patientConsentOf(f.cfg, f.second.Key)
			require.True(t, before.notified && otherBefore.notified)
			require.NotEmpty(t, before.marks.Sources)

			// Recycle refuses a patient somebody holds; let anna go first.
			status, _, _ := served(t, f, "/demo/patients/"+f.anna.Key+"/release", nil)
			require.Less(t, status, 400, "releasing the patient")

			status, location, body := served(t, f, "/demo/patients/"+f.anna.Key+"/recycle", nil)

			require.Equal(t, tc.status, status)
			require.Equal(t, tc.location, location)
			require.Contains(t, body, tc.reason)
			after := patientConsentOf(f.cfg, f.anna.Key)
			require.False(t, after.notified)
			require.Empty(t, after.marks.Sources)
			require.Equal(t, before.resets+1, after.resets)
			require.Greater(t, after.revision, before.revision)

			require.Equal(t, otherBefore, patientConsentOf(f.cfg, f.second.Key), "another patient's notification, counters and revision are left alone")
			require.True(t, f.cfg.Retrievals.holds("another-owner", f.second.Key), "and so is what another session retrieved about that patient")
		})
	}
}

// The handler clears a patient's retrievals and marks inside that patient's
// reset transition and nowhere after it. A withdrawal that is accepted once the
// transition is over therefore keeps the marks it made. The probe makes one
// arrive exactly there, while the handler is still running.
func TestReset_ClearsNothingOnceAPatientsTransitionIsOver(t *testing.T) {
	f := newConsentFixture(t, true)
	f.change(t, "deny")
	watermark := f.mock.changedAt().Add(time.Hour)
	f.cfg.runtime.noteMockWatermark("", watermark)
	var once sync.Once
	setProbe(t, f.cfg.runtime, func(point, patientKey string) {
		if point != probeResetApplied || patientKey != f.anna.Key {
			return
		}
		once.Do(func() {
			// Another session stores a copy, and consent is withdrawn by a change
			// made after the reset.
			if !f.cfg.Retrievals.putIfCurrent("another-owner", f.anna.Key, f.cfg.Retrievals.generationOf(f.anna.Key),
				grantedFrom("Zorgcentrum De Zonnebloem", "Conditions")) {
				t.Error("the copy was not stored")
			}
			status, _ := deliverBody(f.intake, "application/fhir+json",
				notificationJSON("after", f.anna.BSN, plataanURA, "deny", watermark.Add(time.Second)))
			if status != http.StatusNoContent {
				t.Errorf("the intake answered %d", status)
			}
		})
	})

	status, _, _ := served(t, f, "/demo/reset", url.Values{"override": {"true"}})
	require.Equal(t, http.StatusSeeOther, status)

	require.Equal(t, []string{"Conditions"}, f.cfg.Retrievals.marks(f.anna.Key).Sections,
		"what a withdrawal after the patient's reset removed stays marked")
	require.Equal(t, []string{f.anna.Key}, notifiedPatients(f.cfg.runtime))
}

// Both handlers hold the gate for writing while Mitz is being restored. A
// consent action holds it for reading while it changes Mitz (Task 10,
// TestConsent_ARunningActionHoldsTheGateForReading), so the two never overlap:
// a reset waits for a running action, and an action waits for a reset. Each
// restore looks at the gate from the inside.
func TestResetAndRecycle_HoldTheGateForWritingAroundTheRestore(t *testing.T) {
	var state *runtimeState
	var duringReset, duringRecycle atomic.Bool
	heldForWriting := func() bool {
		if state.gate.TryRLock() {
			state.gate.RUnlock()
			return false
		}
		return true
	}
	f := newConsentFixture(t, true, func(cfg *Config) {
		state = cfg.runtime
		cfg.resetGlobal = func(context.Context) error { duringReset.Store(heldForWriting()); return nil }
		cfg.recyclePatient = func(context.Context, string) error { duringRecycle.Store(heldForWriting()); return nil }
	})

	status, _, _ := served(t, f, "/demo/patients/"+f.second.Key+"/recycle", nil) // nobody holds the second patient
	require.Equal(t, http.StatusSeeOther, status)
	require.True(t, duringRecycle.Load(), "a consent action could change Mitz while the patient is being restored")

	status, _, _ = served(t, f, "/demo/reset", url.Values{"override": {"true"}})
	require.Equal(t, http.StatusSeeOther, status)
	require.True(t, duringReset.Load(), "a consent action could change Mitz while it is being restored")
}
