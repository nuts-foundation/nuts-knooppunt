package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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

func postForm(t *testing.T, client *http.Client, srv *httptest.Server, path string, form url.Values) *http.Response {
	t.Helper()
	res, err := client.Post(srv.URL+path, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	return res
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
