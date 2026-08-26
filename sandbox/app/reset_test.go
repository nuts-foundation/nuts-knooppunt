package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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

func postForm(t *testing.T, srv *httptest.Server, path string, form url.Values) *http.Response {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Post(srv.URL+path, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	return res
}

func TestReset_RunsWhenUnlocked(t *testing.T) {
	cfg, resets, _ := fakeConfig()
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)

	res := postForm(t, srv, "/demo/reset", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=reset-done", res.Header.Get("Location"))
	require.Equal(t, []string{"reset"}, *resets)
}

func TestReset_RefusesWhenLockedWithoutOverride(t *testing.T) {
	cfg, resets, _ := fakeConfig()
	require.True(t, cfg.Locks.Lock("pool-02", "session-x"))
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)

	res := postForm(t, srv, "/demo/reset", nil)
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
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Post(srv.URL+"/demo/reset", "application/x-www-form-urlencoded", strings.NewReader("override=%zz"))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	require.Empty(t, *resets, "a request we could not parse must not trigger a reset")
}

func TestReset_ProceedsWithOverride(t *testing.T) {
	cfg, resets, _ := fakeConfig()
	require.True(t, cfg.Locks.Lock("pool-02", "session-x"))
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)

	res := postForm(t, srv, "/demo/reset", url.Values{"override": {"true"}})
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=reset-done", res.Header.Get("Location"))
	require.Equal(t, []string{"reset"}, *resets)
}

func TestRecycle_RefusesLockedPatient(t *testing.T) {
	cfg, _, recycles := fakeConfig()
	anna := pool.Patients()[0].Key
	require.True(t, cfg.Locks.Lock(anna, "session-x"))
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)

	res := postForm(t, srv, "/demo/patients/"+anna+"/recycle", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)
	require.Empty(t, *recycles)
}

func TestRecycle_SucceedsWhenAnotherPatientLocked(t *testing.T) {
	cfg, _, recycles := fakeConfig()
	patients := pool.Patients()
	anna, other := patients[0].Key, patients[1].Key
	require.True(t, cfg.Locks.Lock(other, "session-x"))
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)

	res := postForm(t, srv, "/demo/patients/"+anna+"/recycle", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=recycle-done", res.Header.Get("Location"))
	require.Equal(t, []string{anna}, *recycles)
}

func TestRecycle_UnknownPatientIs404(t *testing.T) {
	cfg, _, _ := fakeConfig()
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)

	res := postForm(t, srv, "/demo/patients/nope/recycle", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestLockAndReleaseEndpoints(t *testing.T) {
	cfg, _, _ := fakeConfig()
	anna := pool.Patients()[0].Key
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)

	// Lock succeeds.
	res := postForm(t, srv, "/demo/patients/"+anna+"/lock", url.Values{"owner": {"s1"}})
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	// A different owner cannot steal it.
	res2 := postForm(t, srv, "/demo/patients/"+anna+"/lock", url.Values{"owner": {"s2"}})
	defer res2.Body.Close()
	require.Equal(t, http.StatusConflict, res2.StatusCode)

	// The owner releases it.
	res3 := postForm(t, srv, "/demo/patients/"+anna+"/release", url.Values{"owner": {"s1"}})
	defer res3.Body.Close()
	require.Equal(t, http.StatusOK, res3.StatusCode)
	require.False(t, cfg.Locks.IsLocked(anna))
}

func TestListPatientsReflectsPoolAndLocks(t *testing.T) {
	cfg, _, _ := fakeConfig()
	anna := pool.Patients()[0].Key
	require.True(t, cfg.Locks.Lock(anna, "s1"))
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)

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
