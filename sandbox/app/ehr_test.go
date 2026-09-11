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
	return getPageWithClient(t, client, srv.URL+path)
}

func postAndRead(t *testing.T, client *http.Client, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	res := postForm(t, client, srv, path, nil)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

// openPatient claims the patient for the client's session, as the row's Open
// button does, and requires the redirect to the record that says the lock was
// taken. A refused open would otherwise surface later as a puzzling 409.
func openPatient(t *testing.T, client *http.Client, srv *httptest.Server, key string) {
	t.Helper()
	res := postForm(t, client, srv, "/demo/ehr/patients/"+key+"/open", nil)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equalf(t, "/demo/ehr/patients/"+key, res.Header.Get("Location"), "opening %s must take the lock", key)
}

// categoriesByBSN drives the NVI fake per patient, so a test can make one row
// fail while the others succeed. One List per category, which is what the real
// registration writes.
func categoriesByBSN(m map[string][]string, failFor map[string]bool) func(context.Context, string) (nviRecords, error) {
	return func(_ context.Context, bsn string) (nviRecords, error) {
		if failFor[bsn] {
			return nviRecords{}, errors.New("connection refused")
		}
		return nviRecords{Count: len(m[bsn]), Categories: m[bsn]}, nil
	}
}

func TestPatientList_ShowsLocalOnlyForAnUnsharedPatient(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.nviLookup = categoriesByBSN(nil, nil)
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
	cfg.nviLookup = categoriesByBSN(map[string][]string{anna.BSN: {nvi.CategoryCondition}}, nil)
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
	cfg.nviLookup = categoriesByBSN(
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
	cfg.nviLookup = categoriesByBSN(nil, nil)
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
	cfg.nviLookup = categoriesByBSN(nil, nil)
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
	cfg.nviLookup = categoriesByBSN(nil, nil)
	first, second := pool.Patients()[0].Key, pool.Patients()[1].Key
	srv, client := demoServer(t, cfg)

	openPatient(t, client, srv, first)
	openPatient(t, client, srv, second)

	require.False(t, cfg.Locks.IsLocked(first), "one session runs one demo at a time")
	require.True(t, cfg.Locks.IsLocked(second))
}

func TestOpenPatient_RefusesAPatientAnotherSessionHolds(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.nviLookup = categoriesByBSN(nil, nil)
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

// The case that tells Switch apart from "release mine, then lock the target":
// with nothing held the two behave identically, so every other open test passes
// under either. Here the session holds a patient and the target is taken, and
// the release-first version would have thrown the held patient away for an
// acquire that then failed.
func TestOpenPatient_ARefusedSwitchKeepsThePatientAlreadyHeld(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.nviLookup = categoriesByBSN(nil, nil)
	held, taken := pool.Patients()[0].Key, pool.Patients()[1].Key
	require.True(t, cfg.Locks.Lock(taken, "someone-else"))
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, held)

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
	cfg.nviLookup = categoriesByBSN(nil, nil)
	anna := pool.Patients()[0].Key
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna)

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna)

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Not findable yet")
	require.Contains(t, body, "Share via the generic functions")
}

func TestPatientRecord_ShowsRegisteredCategoriesWhenShared(t *testing.T) {
	anna := pool.Patients()[0]
	cfg, _, _ := fakeConfig()
	cfg.nviLookup = categoriesByBSN(
		map[string][]string{anna.BSN: {nvi.CategoryCondition, nvi.CategoryMedicationRequest}}, nil)
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return true, nil }
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

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
	cfg.nviLookup = categoriesByBSN(map[string][]string{anna.BSN: {nvi.CategoryCondition}}, nil)
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return false, nil }
	return cfg, anna
}

// The state the design's no-rollback decision depends on: registered, no
// subscription, visible after leaving the confirmation page, repairable on its
// own.
func TestPatientRecord_ShowsTheMissingSubscriptionState(t *testing.T) {
	cfg, anna := sharedNotSubscribed(t)
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key)

	require.Contains(t, body, "consent subscription missing")
	require.Contains(t, body, "/demo/ehr/patients/"+anna.Key+"/subscribe")
}

// The inverse, and the case the reconciliation exists to produce: a lost POST,
// the user returns, the lookup finds the subscription after all, and the record
// must stop offering a retry. "Shared via GF" is a prefix of all three chips, so
// only the absence of the missing state and of the retry form shows this.
func TestPatientRecord_SubscribedShowsNoRetry(t *testing.T) {
	anna := pool.Patients()[0]
	cfg, _, _ := fakeConfig()
	cfg.nviLookup = categoriesByBSN(map[string][]string{anna.BSN: {nvi.CategoryCondition}}, nil)
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return true, nil }
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key)

	require.Contains(t, body, `<span class="stat-chip gf">`)
	require.NotContains(t, body, "consent subscription missing")
	require.NotContains(t, body, "subscription status unknown")
	require.NotContains(t, body, "/subscribe")
}

// With no mock configured the sandbox cannot ask, and "cannot ask" must not
// render as "not subscribed": that state offers a retry, and against a real Mitz
// retrying an existing subscription would duplicate it. MITZMOCK_URL is unset in
// every deployment without the mock wired, so this is the default path.
func TestPatientRecord_UnconfiguredLookupShowsUnknownNotMissing(t *testing.T) {
	anna := pool.Patients()[0]
	cfg, _, _ := fakeConfig()
	cfg.nviLookup = categoriesByBSN(map[string][]string{anna.BSN: {nvi.CategoryCondition}}, nil)
	cfg.mitzSubscribed = nil
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

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
	openPatient(t, client, srv, anna.Key)

	res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/subscribe", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, []string{anna.BSN}, subscribed)
	require.Zero(t, registered, "the retry must not redo the NVI work")
	// The redirect target, not just its status: the notice is the only thing the
	// practitioner reads.
	require.Equal(t, "/demo/ehr/patients/"+anna.Key+"?notice=mitz-retry-done",
		res.Header.Get("Location"), "a successful retry must report success")
}

// A failed retry must not assert a failure it did not establish. The share flow
// reconciles a failed POST against the lookup for exactly this reason; this
// branch used to skip that and tell the presenter to try again, on the screen
// whose own subtext says a retry may duplicate against the national Mitz.
func TestSubscribe_RetryOutcomesFollowTheLookup(t *testing.T) {
	// Two answers per case, in order: the preflight before the write and the
	// reconciliation after it. The notice turns on the difference between them,
	// so a fake answering both from one value cannot tell "absent, then present"
	// (this call committed) from "present, then present" (it was already there).
	type answer struct {
		subscribed bool
		err        error
	}
	unreachable := errors.New("unreachable")
	for name, tc := range map[string]struct {
		answers []answer
		want    string
	}{
		"absent before, present after: this call committed": {
			[]answer{{false, nil}, {true, nil}}, "mitz-retry-done",
		},
		"present before and after: the write failed and changed nothing": {
			[]answer{{true, nil}, {true, nil}}, "mitz-retry-existing",
		},
		"preflight failed, reconciliation found one: exists, since when unknown": {
			[]answer{{false, unreachable}, {true, nil}}, "mitz-retry-registered",
		},
		"reconciliation confirms it did not": {
			[]answer{{false, nil}, {false, nil}}, "mitz-retry-failed",
		},
		"lookup itself failed": {
			[]answer{{false, unreachable}, {false, unreachable}}, "mitz-retry-unknown",
		},
		"no lookup configured": {nil, "mitz-retry-unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, anna := sharedNotSubscribed(t)
			cfg.mitzSubscribe = func(context.Context, string) error { return errors.New("timeout") }
			cfg.mitzSubscribed = nil
			if tc.answers != nil {
				answers := tc.answers
				cfg.mitzSubscribed = func(context.Context, string) (bool, error) {
					next := answers[0]
					if len(answers) > 1 {
						answers = answers[1:]
					}
					return next.subscribed, next.err
				}
			}
			srv, client := demoServer(t, cfg)
			openPatient(t, client, srv, anna.Key)

			res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/subscribe", nil)
			defer res.Body.Close()

			require.Equal(t, "/demo/ehr/patients/"+anna.Key+"?notice="+tc.want, res.Header.Get("Location"))
		})
	}
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
	cfg.nviRegister = func(_ context.Context, bsn string, cats []string) error {
		calls.registeredBSN, calls.registeredCats = bsn, cats
		calls.registerCount++
		return nil
	}
	// The read after the write sees the write. A fake that always answers "no
	// records" makes the share response render the green publication card above
	// prose saying the patient is local-only, and every success test here would
	// pass while showing the presenter a page that contradicts itself.
	cfg.nviLookup = func(_ context.Context, bsn string) (nviRecords, error) {
		if calls.registeredBSN != bsn {
			return nviRecords{}, nil
		}
		return nviRecords{Count: len(calls.registeredCats), Categories: calls.registeredCats}, nil
	}
	cfg.mitzSubscribe = func(context.Context, string) error { calls.subscribeCount++; return nil }
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return false, nil }
	return cfg, calls, anna
}

func openAndShare(t *testing.T, cfg Config, key string) (int, string) {
	t.Helper()
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, key)
	return postAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/share")
}

func TestShare_RegistersTheDerivedCategoriesAndConfirmsBothSteps(t *testing.T) {
	cfg, calls, anna := shareConfig(t)

	_, body := openAndShare(t, cfg, anna.Key)

	require.Equal(t, anna.BSN, calls.registeredBSN)
	require.Equal(t, anna.PlataanCategories(), calls.registeredCats,
		"the handler must send exactly the categories De Plataan holds")
	require.Contains(t, body, "Localization records published")
	require.Contains(t, body, "Consent subscription started")
	// State, not just title: a card carrying Failed alongside a success title
	// renders red with a cross and would satisfy the titles alone.
	require.Contains(t, body, `<span class="ok-ic">`)
	require.NotContains(t, body, "confirm-card failed")
	require.NotContains(t, body, "exists only in De Plataan's own store",
		"the page must not deny the publication it just reported")
}

// The publish succeeded and the read that follows it failed. The header is built
// from that read, so without the write's own outcome overriding it the response
// carries the green card above prose saying the record never left the building.
func TestShare_SuccessIsNotContradictedByAFailedFollowUpRead(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	cfg.nviLookup = func(context.Context, string) (nviRecords, error) {
		return nviRecords{}, errors.New("connection refused")
	}

	_, body := openAndShare(t, cfg, anna.Key)

	require.Contains(t, body, "Localization records published")
	require.NotContains(t, body, "exists only in De Plataan's own store")
	require.NotContains(t, body, "it is not known whether this")
}

// The mirror of the test above, on the way in: with no card to override it, an
// unreachable NVI must not let the share form assert that the patient is
// local-only. The form is where a presenter decides whether to share at all.
func TestShareForm_UnknownNVIIsNotRenderedAsLocalOnly(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	cfg.nviLookup = func(context.Context, string) (nviRecords, error) {
		return nviRecords{}, errors.New("connection refused")
	}
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "it is not known whether this")
	require.NotContains(t, body, "exists only in De Plataan's own store")
	require.NotContains(t, body, "This patient is already shared")
}

func TestShare_CardNamesTheRegisteredCategories(t *testing.T) {
	cfg, _, anna := shareConfig(t)

	_, body := openAndShare(t, cfg, anna.Key)

	// The card's own row, not just "somewhere on the page": the share form above
	// the cards renders the same categories on every response, each in its own
	// span, so only the comma-joined form can come from the card.
	require.Contains(t, body,
		`<span class="k">Data categories</span><span class="v">`+strings.Join(anna.PlataanCategories(), ", ")+`</span>`,
		"the NVI card must name the categories itself")
	require.NotContains(t, body, "BGZ (patient summary)")
}

// The base --profile sandbox deployment sets no MITZMOCK_URL, so the preflight
// cannot run there and the card must not claim this request created anything.
func TestShare_WithoutALookupTheSuccessCardClaimsNoCreation(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	cfg.mitzSubscribed = nil

	_, body := openAndShare(t, cfg, anna.Key)

	require.Contains(t, body, "Consent subscription registered")
	require.NotContains(t, body, "Consent subscription started")
	require.NotContains(t, body, "confirm-card failed")
}

// A preflight that errors is the same state as not having one: nothing was
// established, so neither "started" nor "already registered" is available.
func TestShare_AFailedPreflightFallsBackToTheNeutralCard(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) {
		return false, errors.New("mock unreachable")
	}

	_, body := openAndShare(t, cfg, anna.Key)

	require.Contains(t, body, "Consent subscription registered")
	require.NotContains(t, body, "Consent subscription started")
	require.NotContains(t, body, "Consent subscription already registered")
}

// The NVI holds records this build cannot name. The share form replaces the
// category list with what it proposes to publish, so without carrying that state
// separately it presents a stored legacy record as the same set it is about to
// write, under copy promising convergence on one per category. Both shapes,
// because the mixed one is the trap: derived from an empty recognized set, three
// records this client wrote alongside one it cannot name reads as "all
// recognized".
func TestShareForm_UnrecognizedCategoriesAreNotDescribedAsTheSameRecords(t *testing.T) {
	for name, records := range map[string]nviRecords{
		"only unrecognized": {Count: 1, Unnamed: 1},
		"mixed with recognized": {
			Count: 4, Categories: []string{"Condition", "MedicationRequest", "Patient"}, Unnamed: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, _, anna := shareConfig(t)
			cfg.nviLookup = func(context.Context, string) (nviRecords, error) { return records, nil }
			srv, client := demoServer(t, cfg)
			openPatient(t, client, srv, anna.Key)

			status, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share")

			require.Equal(t, http.StatusOK, status)
			require.Contains(t, body, "does not recognize")
			require.NotContains(t, body, "republishes the same")
			require.NotContains(t, body, "exists only in De Plataan's own store")
			// And no claim about who wrote them or what happens to them, which
			// the record count cannot establish either way.
			require.NotContains(t, body, "not published by this client")
			require.NotContains(t, body, "does not replace or remove")
		})
	}
}

// The retry route is offered when the subscription's state is unknown, and the
// mock upserts, so a POST succeeding proves nothing about creation. Reporting
// "started" here contradicts the warning on the button that produced the click.
func TestSubscribe_ExistingSubscriptionIsNotReportedAsStarted(t *testing.T) {
	cfg, anna := sharedNotSubscribed(t)
	cfg.mitzSubscribe = func(context.Context, string) error { return nil }
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return true, nil }
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

	res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/subscribe", nil)
	defer res.Body.Close()

	require.Equal(t, "/demo/ehr/patients/"+anna.Key+"?notice=mitz-retry-existing",
		res.Header.Get("Location"))
}

// And with no lookup at all, neither claim is available.
func TestSubscribe_WithoutALookupReportsRegisteredNotStarted(t *testing.T) {
	cfg, anna := sharedNotSubscribed(t)
	cfg.mitzSubscribe = func(context.Context, string) error { return nil }
	cfg.mitzSubscribed = nil
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

	res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/subscribe", nil)
	defer res.Body.Close()

	require.Equal(t, "/demo/ehr/patients/"+anna.Key+"?notice=mitz-retry-registered",
		res.Header.Get("Location"))
}

// The GET the record page's Share button points at. Every other share test
// exercises the POST, so without this the mux registration could go and the
// suite would stay green while the demo's front door 404s. It is also the only
// test of the template with no cards yet.
func TestShare_FormRendersBeforeAnythingIsShared(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Share patient")
	require.NotContains(t, body, "confirm-card", "no outcome cards before the share has run")
}

func TestShare_MitzFailureIsShownWithoutFakingTheNVIResult(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	cfg.mitzSubscribe = func(context.Context, string) error { return errors.New("mitz unreachable") }

	_, body := openAndShare(t, cfg, anna.Key)

	require.Contains(t, body, "Localization records published")
	require.Contains(t, body, "Mitz subscription failed")
	require.NotContains(t, body, "Consent subscription started")
	require.Contains(t, body, "confirm-card failed", "the failed card must render as failed, not merely be titled so")
}

// An error the reconciliation query says did commit is a lost response, not a
// failure, and must not invite a retry that could duplicate against real Mitz.
func TestShare_LostMitzResponseIsReportedAsStartedNotFailed(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	// Stateful, because the handler asks twice and the answers differ: not
	// subscribed on the way in, subscribed once the call has committed. A fake
	// answering "yes" to both would report a subscription that was already
	// there, which is a different claim from the one under test.
	committed := false
	cfg.mitzSubscribe = func(context.Context, string) error { committed = true; return context.DeadlineExceeded }
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return committed, nil }

	_, body := openAndShare(t, cfg, anna.Key)

	require.Contains(t, body, "Consent subscription started")
	require.NotContains(t, body, "Mitz subscription failed")
	require.NotContains(t, body, "Mitz subscription outcome unknown")
}

// The third outcome. A failed call the sandbox cannot reconcile is not a failed
// subscription: MITZMOCK_URL is unset in the base compose profile, so this is the
// shipped behaviour there, and a red cross would tell the presenter that Mitz
// refused when nothing established that.
func TestShare_UnreconcilableMitzFailureIsUnknownNotFailed(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	cfg.mitzSubscribe = func(context.Context, string) error { return context.DeadlineExceeded }
	cfg.mitzSubscribed = nil

	_, body := openAndShare(t, cfg, anna.Key)

	require.Contains(t, body, "Mitz subscription outcome unknown")
	require.NotContains(t, body, "Mitz subscription failed")
	require.NotContains(t, body, "confirm-card failed",
		"an outcome nobody established must not render as a confirmed failure")
	require.Contains(t, body, "confirm-card unknown")
}

// The reconciliation query itself failing is the same state as not having one:
// the call's outcome is unestablished either way.
func TestShare_FailedReconciliationQueryIsUnknownNotFailed(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	cfg.mitzSubscribe = func(context.Context, string) error { return context.DeadlineExceeded }
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) { return false, errors.New("mock unreachable") }

	_, body := openAndShare(t, cfg, anna.Key)

	require.Contains(t, body, "Mitz subscription outcome unknown")
	require.NotContains(t, body, "confirm-card failed")
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
	openPatient(t, client, srv, first)

	res := postForm(t, client, srv, "/demo/ehr/patients/"+second+"/share", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusConflict, res.StatusCode)
	require.Zero(t, calls.registerCount)
}

func TestShare_RejectsASubmissionAfterTheLeaseExpired(t *testing.T) {
	cfg, calls, anna := shareConfig(t)
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

	// The presenter walked away; the lease ran out while the page stayed open.
	cfg.Locks.ReleaseAll()

	res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusConflict, res.StatusCode)
	require.Zero(t, calls.registerCount)
}

// Held by someone else, not merely held. Every other rejection test arranges for
// nobody to hold the patient, where "locked by anyone" and "locked by me" answer
// the same, so the ownership half of the guard is pinned only here.
func TestWriteRoutes_RejectAPatientHeldByAnotherSession(t *testing.T) {
	for name, path := range map[string]string{"share": "/share", "subscribe": "/subscribe"} {
		t.Run(name, func(t *testing.T) {
			cfg, calls, anna := shareConfig(t)
			var subscribes int
			cfg.mitzSubscribe = func(context.Context, string) error { subscribes++; return nil }
			srv, client := demoServer(t, cfg)
			require.True(t, cfg.Locks.Lock(anna.Key, "another-session"),
				"precondition: a different session holds the patient")

			res := postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+path, nil)
			defer res.Body.Close()

			require.Equal(t, http.StatusConflict, res.StatusCode)
			require.Zero(t, calls.registerCount, "no NVI write for a patient this session does not hold")
			require.Zero(t, subscribes, "and no Mitz write either")
		})
	}
}

// The presenter's own patient is not "in use by another demo run". Without the
// ownership half of that condition the row they just opened goes disabled and
// tells them someone else has it.
func TestPatientList_DoesNotMarkThisSessionsOwnPatientBusy(t *testing.T) {
	cfg, _, anna := shareConfig(t)
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

	status, body := getBody(t, client, srv, "/demo/ehr")

	require.Equal(t, http.StatusOK, status)
	require.NotContains(t, body, "Demo in progress",
		"the row this session holds must stay openable")
	require.NotContains(t, body, "disabled")
}

func TestShare_RejectsCrossSiteSubmissions(t *testing.T) {
	cfg, calls, anna := shareConfig(t)
	srv, client := demoServer(t, cfg)
	openPatient(t, client, srv, anna.Key)

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
	// Every shared counter is atomic and nothing calls require off the test
	// goroutine: shareConfig's closures increment plain ints and postForm calls
	// require, both of which race when two handlers run at once.
	//
	// The fakes copy the mock's behaviour, because that is what makes the claim
	// falsifiable: one subscription per provider and patient, a repeat answered
	// with the existing one, and nothing in the response to tell the two apart.
	var mitzMu sync.Mutex
	subscriptionExists := false
	// A barrier, not a sleep: both preflights must get every chance to land in
	// the gap between reading "no subscription" and creating one, so each waits
	// here until the other arrives, or briefly. With the lock released before the
	// Mitz step both arrive and read absent; with it spanning both steps the
	// second cannot start until the first has finished.
	var arrived int32
	cfg.mitzSubscribed = func(context.Context, string) (bool, error) {
		mitzMu.Lock()
		existing := subscriptionExists
		mitzMu.Unlock()

		atomic.AddInt32(&arrived, 1)
		for deadline := time.Now().Add(200 * time.Millisecond); atomic.LoadInt32(&arrived) < 2 && time.Now().Before(deadline); {
			time.Sleep(time.Millisecond)
		}
		return existing, nil
	}
	cfg.mitzSubscribe = func(context.Context, string) error {
		mitzMu.Lock()
		defer mitzMu.Unlock()
		subscriptionExists = true
		return nil
	}
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
	openPatient(t, client, srv, anna.Key)

	bodies := make([]string, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := client.Post(srv.URL+"/demo/ehr/patients/"+anna.Key+"/share",
				"application/x-www-form-urlencoded", nil)
			if err != nil {
				return
			}
			defer res.Body.Close()
			body, readErr := io.ReadAll(res.Body)
			if readErr == nil {
				bodies[i] = string(body)
			}
		}()
	}
	wg.Wait()

	require.Equal(t, int32(2), atomic.LoadInt32(&total), "both submits should have reached the NVI")

	require.Equal(t, int32(1), atomic.LoadInt32(&maxInFlight),
		"the per-patient mutex must keep two share submits from overlapping")

	// What the two responses claimed, which serialization of the NVI call alone
	// does not establish: exactly one request created the subscription, and if
	// the lock ended before the Mitz step both would read "not subscribed" first
	// and both would render the card saying they started it.
	started, existing := 0, 0
	for _, body := range bodies {
		if strings.Contains(body, "Consent subscription started") {
			started++
		}
		if strings.Contains(body, "Consent subscription already registered") {
			existing++
		}
	}
	require.Equal(t, 1, started, "only the request that created the subscription may say it started it")
	require.Equal(t, 1, existing, "the other must report that it found one")
}
