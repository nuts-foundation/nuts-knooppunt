package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
