package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
)

func getBody(t *testing.T, client *http.Client, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	res, err := client.Get(srv.URL + path)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

// categoriesByBSN drives the NVI fake per patient, so a test can make one row
// fail while the others succeed.
func categoriesByBSN(m map[string][]string, failFor map[string]bool) func(context.Context, string) ([]string, error) {
	return func(_ context.Context, bsn string) ([]string, error) {
		if failFor[bsn] {
			return nil, errors.New("connection refused")
		}
		return m[bsn], nil
	}
}

func TestPatientList_ShowsLocalOnlyForAnUnsharedPatient(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(nil, nil)
	srv, client := demoServer(t, cfg)

	status, body := getBody(t, client, srv, "/demo/ehr")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Jansen, Anna")
	// The chip markup, not the words: the footer below the list explains what
	// "Local only" and "Status unknown" mean, so asserting the prose passes even
	// when no chip renders at all.
	require.Contains(t, body, `<span class="stat-chip local">`)
	require.NotContains(t, body, `<span class="stat-chip gf">`)
}

func TestPatientList_ShowsSharedForARegisteredPatient(t *testing.T) {
	anna := pool.Patients()[0]
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(map[string][]string{anna.BSN: {nvi.CategoryCondition}}, nil)
	srv, client := demoServer(t, cfg)

	_, body := getBody(t, client, srv, "/demo/ehr")

	require.Contains(t, body, `<span class="stat-chip gf">`)
	require.Contains(t, body, `<span class="stat-chip local">`, "the other pool patients are still unshared")
}

// One failing row must degrade to unknown while the others still render their
// real status. A fake that fails for every BSN would not prove that.
func TestPatientList_OneFailingRowDoesNotAffectTheOthers(t *testing.T) {
	patients := pool.Patients()
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(
		map[string][]string{patients[1].BSN: {nvi.CategoryCondition}},
		map[string]bool{patients[0].BSN: true},
	)
	srv, client := demoServer(t, cfg)

	status, body := getBody(t, client, srv, "/demo/ehr")

	require.Equal(t, http.StatusOK, status, "the list must render when one row's NVI call fails")
	require.Equal(t, 1, strings.Count(body, `<span class="stat-chip unknown">`), "exactly the failing row degrades")
	require.Equal(t, 1, strings.Count(body, `<span class="stat-chip gf">`))
	require.Equal(t, len(patients)-2, strings.Count(body, `<span class="stat-chip local">`))
}

func TestPatientList_MarksAPatientLockedByAnotherSession(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(nil, nil)
	require.True(t, cfg.Locks.Lock(pool.Patients()[0].Key, "someone-else"))
	srv, client := demoServer(t, cfg)

	_, body := getBody(t, client, srv, "/demo/ehr")

	require.Contains(t, body, `<span class="stat-chip busy">`)
}

func TestPatientList_RequiresASession(t *testing.T) {
	cfg, _, _ := fakeConfig()
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	res, err := client.Get(srv.URL + "/demo/ehr")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo/login", res.Header.Get("Location"))
}

func TestOpenPatient_TakesTheLockAndRedirects(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(nil, nil)
	anna := pool.Patients()[0].Key
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/ehr/patients/"+anna+"/open", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo/ehr/patients/"+anna, res.Header.Get("Location"))
	require.True(t, cfg.Locks.IsLocked(anna))
}

func TestOpenPatient_ReleasesThePreviouslyOpenedPatient(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(nil, nil)
	first, second := pool.Patients()[0].Key, pool.Patients()[1].Key
	srv, client := demoServer(t, cfg)

	postForm(t, client, srv, "/demo/ehr/patients/"+first+"/open", nil).Body.Close()
	postForm(t, client, srv, "/demo/ehr/patients/"+second+"/open", nil).Body.Close()

	require.False(t, cfg.Locks.IsLocked(first), "one session runs one demo at a time")
	require.True(t, cfg.Locks.IsLocked(second))
}

func TestOpenPatient_RefusesAPatientAnotherSessionHolds(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(nil, nil)
	anna := pool.Patients()[0].Key
	require.True(t, cfg.Locks.Lock(anna, "someone-else"))
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/ehr/patients/"+anna+"/open", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo/ehr?notice=patient-busy", res.Header.Get("Location"))
	require.True(t, cfg.Locks.HeldBy(anna, "someone-else"),
		"a refused open must leave the holder's lock exactly as it was")
}

// A refused open must not cost the caller the patient it already had.
//
// This is the case that tells Switch apart from a naive "release mine, then
// lock the target": with nothing held, releasing first is a no-op and both
// implementations look identical, which is why every other test here passes
// under either one. Here the session holds the first patient, the second is
// taken, and the release-first version would have thrown the first away for an
// acquire that then failed.
func TestOpenPatient_ARefusedSwitchKeepsThePatientAlreadyHeld(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(nil, nil)
	held, taken := pool.Patients()[0].Key, pool.Patients()[1].Key
	require.True(t, cfg.Locks.Lock(taken, "someone-else"))
	srv, client := demoServer(t, cfg)

	first := postForm(t, client, srv, "/demo/ehr/patients/"+held+"/open", nil)
	require.NoError(t, first.Body.Close())
	require.Equal(t, http.StatusSeeOther, first.StatusCode)

	res := postForm(t, client, srv, "/demo/ehr/patients/"+taken+"/open", nil)
	defer res.Body.Close()

	require.Equal(t, "/demo/ehr?notice=patient-busy", res.Header.Get("Location"))
	require.True(t, cfg.Locks.IsLocked(held),
		"the failed switch must not have released the patient this session was working on")
	require.True(t, cfg.Locks.HeldBy(taken, "someone-else"),
		"and the other session keeps what it held")
}

func TestPatientRecord_ShowsNotFindableYetWhenUnshared(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(nil, nil)
	anna := pool.Patients()[0].Key
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+anna+"/open", nil).Body.Close()

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna)

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Not findable yet")
	require.Contains(t, body, "Share via the generic functions")
}

func TestPatientRecord_ShowsRegisteredCategoriesWhenShared(t *testing.T) {
	anna := pool.Patients()[0]
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(
		map[string][]string{anna.BSN: {nvi.CategoryCondition, nvi.CategoryMedicationRequest}}, nil)
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return true, nil }
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key)

	require.Contains(t, body, "Shared via GF")
	require.Contains(t, body, nvi.CategoryMedicationRequest)
	require.NotContains(t, body, "BGZ (patient summary)",
		"there is no BGZ code; the record must name what was registered")
}

func TestPatientRecord_UnknownPatientIs404(t *testing.T) {
	cfg, _, _ := fakeConfig()
	srv, client := demoServer(t, cfg)

	status, _ := getBody(t, client, srv, "/demo/ehr/patients/nope")

	require.Equal(t, http.StatusNotFound, status)
}

func sharedNotSubscribed(t *testing.T) (Config, pool.PoolPatient) {
	t.Helper()
	anna := pool.Patients()[0]
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(map[string][]string{anna.BSN: {nvi.CategoryCondition}}, nil)
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return false, nil }
	return cfg, anna
}

// The state the design's no-rollback decision depends on: registered, no
// subscription, visible after leaving the confirmation page, repairable on its
// own.
func TestPatientRecord_ShowsTheMissingSubscriptionState(t *testing.T) {
	cfg, anna := sharedNotSubscribed(t)
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key)

	require.Contains(t, body, "consent subscription missing")
	require.Contains(t, body, "/demo/ehr/patients/"+anna.Key+"/subscribe")
}

// The inverse, and the case the whole reconciliation exists to produce: a lost
// POST, the user returns, the lookup finds the subscription after all, and the
// record must stop offering a retry.
//
// Asserting the presence of "Shared via GF" cannot show this, because that text
// is a prefix of all three chips. Only the absence of the missing state and of
// the retry form distinguishes it.
func TestPatientRecord_SubscribedShowsNoRetry(t *testing.T) {
	anna := pool.Patients()[0]
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(map[string][]string{anna.BSN: {nvi.CategoryCondition}}, nil)
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return true, nil }
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key)

	require.Contains(t, body, `<span class="stat-chip gf">`)
	require.NotContains(t, body, "consent subscription missing")
	require.NotContains(t, body, "subscription status unknown")
	require.NotContains(t, body, "/subscribe")
}

// With no mock configured the sandbox cannot ask, and "cannot ask" must not
// render as "not subscribed" — that state offers a retry, and against a real
// Mitz retrying an existing subscription would duplicate it. MITZMOCK_URL is
// unset in every deployment that has not wired the mock, so this is the default
// path, not an edge case.
func TestPatientRecord_UnconfiguredLookupShowsUnknownNotMissing(t *testing.T) {
	anna := pool.Patients()[0]
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(map[string][]string{anna.BSN: {nvi.CategoryCondition}}, nil)
	cfg.mitzSubscribed = nil
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key)

	require.Contains(t, body, `<span class="stat-chip unknown">`,
		"the chip class, not its label: NVIUnknown is false here, so this class can only be the subscription one")
	require.NotContains(t, body, "consent subscription missing")
	require.Contains(t, body, "/subscribe",
		"unknown keeps the retry, so the presenter is not stranded with no way forward")
	// A fragment that sits on one source line: the template wraps its prose, so
	// a longer phrase would straddle a newline and never match.
	require.Contains(t, body, "could not be asked whether a subscription already exists",
		"the unknown state must admit that a retry is not known to be safe")
}

func TestSubscribe_RetriesOnlyTheMitzStep(t *testing.T) {
	cfg, anna := sharedNotSubscribed(t)
	var subscribed []string
	cfg.mitzSubscribe = func(_ context.Context, bsn string) error {
		subscribed = append(subscribed, bsn)
		return nil
	}
	var registered int
	cfg.nviRegister = func(context.Context, string, []string) error { registered++; return nil }
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()

	res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/subscribe", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, []string{anna.BSN}, subscribed)
	require.Zero(t, registered, "the retry must not redo the NVI work")
}

func TestSubscribe_RejectsASessionWithoutTheLock(t *testing.T) {
	cfg, anna := sharedNotSubscribed(t)
	cfg.mitzSubscribe = func(context.Context, string) error { return nil }
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/subscribe", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusConflict, res.StatusCode)
}

type shareCalls struct {
	registeredBSN  string
	registeredCats []string
	registerCount  int
	subscribeCount int
}

func shareConfig(t *testing.T) (Config, *shareCalls, pool.PoolPatient) {
	t.Helper()
	anna := pool.Patients()[0]
	calls := &shareCalls{}
	cfg, _, _ := fakeConfig()
	cfg.nviCategories = categoriesByBSN(nil, nil)
	cfg.nviRegister = func(_ context.Context, bsn string, cats []string) error {
		calls.registeredBSN, calls.registeredCats = bsn, cats
		calls.registerCount++
		return nil
	}
	cfg.mitzSubscribe = func(context.Context, string) error { calls.subscribeCount++; return nil }
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return false, nil }
	return cfg, calls, anna
}

func openAndShare(t *testing.T, cfg Config, key string) (int, string) {
	t.Helper()
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+key+"/open", nil).Body.Close()

	res := postForm(t, client, srv, "/demo/ehr/patients/"+key+"/share", nil)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

func TestShare_RegistersTheDerivedCategoriesAndConfirmsBothSteps(t *testing.T) {
	cfg, calls, anna := shareConfig(t)

	_, body := openAndShare(t, cfg, anna.Key)

	require.Equal(t, anna.BSN, calls.registeredBSN)
	require.Equal(t, anna.PlataanCategories(), calls.registeredCats,
		"the handler must send exactly the categories De Plataan holds")
	require.Contains(t, body, "Localization records published")
	require.Contains(t, body, "Mitz subscription started")
}

func TestShare_CardNamesTheRegisteredCategories(t *testing.T) {
	cfg, _, anna := shareConfig(t)

	_, body := openAndShare(t, cfg, anna.Key)

	for _, category := range anna.PlataanCategories() {
		require.Contains(t, body, category)
	}
	require.NotContains(t, body, "BGZ (patient summary)")
}

func TestShare_MitzFailureIsShownWithoutFakingTheNVIResult(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	cfg.mitzSubscribe = func(context.Context, string) error { return errors.New("mitz unreachable") }

	_, body := openAndShare(t, cfg, anna.Key)

	require.Contains(t, body, "Localization records published")
	require.Contains(t, body, "Mitz subscription failed")
	require.NotContains(t, body, "Mitz subscription started")
}

// An error the reconciliation query says did commit is a lost response, not a
// failure, and must not invite a retry that could duplicate against real Mitz.
func TestShare_LostMitzResponseIsReportedAsUnknownNotFailed(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	cfg.mitzSubscribe = func(context.Context, string) error { return context.DeadlineExceeded }
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return true, nil }

	_, body := openAndShare(t, cfg, anna.Key)

	require.Contains(t, body, "Mitz subscription started")
	require.NotContains(t, body, "Mitz subscription failed")
}

func TestShare_MitzIsSkippedWhenTheNVIStepFailed(t *testing.T) {
	cfg, calls, anna := shareConfig(t)
	cfg.nviRegister = func(context.Context, string, []string) error { return errors.New("nvi unreachable") }

	_, body := openAndShare(t, cfg, anna.Key)

	require.Zero(t, calls.subscribeCount)
	require.Contains(t, body, "Localization records may be incomplete")
	require.Contains(t, body, "Not attempted")
	require.NotContains(t, body, "restores a consistent state",
		"a persistent failure is not repaired by retrying; the card must not promise it")
}

// A page whose lock expired, or that belongs to a different patient, must not be
// able to write. The stale tab is the realistic concurrency case.
func TestShare_RejectsASubmissionWithoutTheLock(t *testing.T) {
	cfg, calls, anna := shareConfig(t)
	srv, client := demoServer(t, cfg)

	res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusConflict, res.StatusCode)
	require.Zero(t, calls.registerCount)
}

func TestShare_RejectsASubmissionForADifferentPatient(t *testing.T) {
	cfg, calls, _ := shareConfig(t)
	first, second := pool.Patients()[0].Key, pool.Patients()[1].Key
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+first+"/open", nil).Body.Close()

	res := postForm(t, client, srv, "/demo/ehr/patients/"+second+"/share", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusConflict, res.StatusCode)
	require.Zero(t, calls.registerCount)
}

func TestShare_RejectsASubmissionAfterTheLeaseExpired(t *testing.T) {
	cfg, calls, anna := shareConfig(t)
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()

	// The presenter walked away; the lease ran out while the page stayed open.
	cfg.Locks.ReleaseAll()

	res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusConflict, res.StatusCode)
	require.Zero(t, calls.registerCount)
}

func TestShare_RejectsCrossSiteSubmissions(t *testing.T) {
	cfg, calls, anna := shareConfig(t)
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/demo/ehr/patients/"+anna.Key+"/share", nil)
	require.NoError(t, err)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	res, err := client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusForbidden, res.StatusCode)
	require.Zero(t, calls.registerCount)
}

// Two submits for the same patient must serialize rather than interleave.
func TestShare_ConcurrentSubmitsSerialize(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	// Every shared counter goes atomic and nothing calls require off the test
	// goroutine: shareConfig's nviRegister AND mitzSubscribe closures both
	// increment plain ints, and postForm calls require, all of which race when
	// two handlers run at once.
	cfg.mitzSubscribe = func(context.Context, string) error { return nil }
	var inFlight, maxInFlight, total int32
	cfg.nviRegister = func(context.Context, string, []string) error {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			observed := atomic.LoadInt32(&maxInFlight)
			if current <= observed || atomic.CompareAndSwapInt32(&maxInFlight, observed, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		atomic.AddInt32(&total, 1)
		return nil
	}
	srv, client := demoServer(t, cfg)
	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := client.Post(srv.URL+"/demo/ehr/patients/"+anna.Key+"/share",
				"application/x-www-form-urlencoded", nil)
			if err == nil {
				_ = res.Body.Close()
			}
		}()
	}
	wg.Wait()

	require.Equal(t, int32(2), atomic.LoadInt32(&total), "both submits should have reached the NVI")

	require.Equal(t, int32(1), atomic.LoadInt32(&maxInFlight),
		"the per-patient mutex must keep two share submits from overlapping")
}
