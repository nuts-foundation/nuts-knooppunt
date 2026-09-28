package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/stretchr/testify/require"
)

var selectionInput = regexp.MustCompile(`name="selection" value="([^"]+)"`)

func sourceConfirmation(t *testing.T, client *http.Client, srv *httptest.Server, key, ura string) url.Values {
	t.Helper()
	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve")
	require.Equal(t, http.StatusOK, status)
	match := selectionInput.FindStringSubmatch(body)
	require.Len(t, match, 2, "the confirmation must refer to the server's discovery result")
	return url.Values{"ura": {ura}, "selection": {match[1]}}
}

func TestRetrieve_ConfirmationReusesDiscovery(t *testing.T) {
	cfg := retrievalConfig(t)
	statusReads := 0
	statusLookup := cfg.nviLookup
	cfg.nviLookup = func(ctx context.Context, bsn string) (nviRecords, error) {
		statusReads++
		return statusLookup(ctx, bsn)
	}
	localizations, resolutions := 0, 0
	localize, resolve := cfg.nviLocalize, cfg.mcsdResolve
	cfg.nviLocalize = func(ctx context.Context, bsn string) ([]localizedRecord, error) {
		localizations++
		return localize(ctx, bsn)
	}
	cfg.mcsdResolve = func(ctx context.Context, ura string) (sourceAddress, error) {
		resolutions++
		source, err := resolve(ctx, ura)
		source.AuthorizationServer = "http://source.example/oauth2"
		return source, err
	}
	var used sourceAddress
	var categories []string
	retrieve := cfg.retrieveFromSource
	cfg.retrieveFromSource = func(ctx context.Context, session authSession, source sourceAddress, bsn string, cats []string) (sourceRetrieval, error) {
		used, categories = source, cats
		return retrieve(ctx, session, source, bsn, cats)
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve")
	form := url.Values{"ura": {zonnebloemURA}, "address": {"http://untrusted.example/fhir"}, "categories": {"Observation"}}
	if match := selectionInput.FindStringSubmatch(body); len(match) == 2 {
		form.Set("selection", match[1])
	}
	status, _ := postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve", form)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, 1, localizations, "confirming an established source must not repeat NVI/PRS discovery")
	require.Equal(t, 1, resolutions, "the already resolved source address must be reused")
	require.Zero(t, statusReads, "retrieval screens do not display sharing status and must not query it")
	require.Equal(t, "http://pep-zonnebloem:8080/fhir", used.Address)
	require.Equal(t, "http://source.example/oauth2", used.AuthorizationServer)
	require.ElementsMatch(t, []string{nvi.CategoryPatient, nvi.CategoryCondition, nvi.CategoryAllergyIntolerance, nvi.CategoryMedicationRequest}, categories)
}

func TestRetrieve_SelectionCannotOutliveItsScope(t *testing.T) {
	for _, change := range []string{"expiry", "replaced", "released", "switched", "another patient", "another session", "forged selection", "forged source"} {
		t.Run(change, func(t *testing.T) {
			cfg := retrievalConfig(t)
			cfg.Runs = newRunStore(cfg.Locks)
			var clock atomic.Int64
			clock.Store(time.Now().UnixNano())
			cfg.Runs.now = func() time.Time { return time.Unix(0, clock.Load()) }
			var lookups, retrievals atomic.Int32
			localize, retrieve := cfg.nviLocalize, cfg.retrieveFromSource
			cfg.nviLocalize = func(ctx context.Context, bsn string) ([]localizedRecord, error) {
				lookups.Add(1)
				return localize(ctx, bsn)
			}
			cfg.retrieveFromSource = func(ctx context.Context, session authSession, source sourceAddress, bsn string, cats []string) (sourceRetrieval, error) {
				retrievals.Add(1)
				return retrieve(ctx, session, source, bsn, cats)
			}
			srv, client := demoServer(t, cfg)
			key := annaKey(t)
			openPatient(t, client, srv, key)
			form := sourceConfirmation(t, client, srv, key, zonnebloemURA)
			switch change {
			case "expiry":
				clock.Add(int64(5 * time.Minute))
			case "replaced":
				_ = sourceConfirmation(t, client, srv, key, zonnebloemURA)
			case "released":
				postForm(t, client, srv, "/demo/patients/"+key+"/release", nil).Body.Close()
				openPatient(t, client, srv, key)
			case "switched":
				openPatient(t, client, srv, "pool-02")
				openPatient(t, client, srv, key)
			case "another patient":
				// Manually locked, so anna's run and its selection stay, and
				// pool-02 has a discovery of its own for the selection to miss.
				postForm(t, client, srv, "/demo/patients/pool-02/lock", nil).Body.Close()
				key = "pool-02"
				_ = sourceConfirmation(t, client, srv, key, zonnebloemURA)
			case "another session":
				client = signInViaDezi(t, srv)
				key = "pool-02"
				openPatient(t, client, srv, key)
				_ = sourceConfirmation(t, client, srv, key, zonnebloemURA)
			case "forged selection":
				form.Set("selection", "untrusted-selection")
			case "forged source":
				form.Set("ura", "00009999")
			}
			before := lookups.Load()
			status, body := postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve", form)
			require.Equal(t, http.StatusConflict, status)
			require.Contains(t, body, "Refresh source list")
			require.NotContains(t, body, "The referral index holds no records")
			require.Equal(t, before, lookups.Load(), "invalid confirmation must not silently perform discovery")
			require.Zero(t, retrievals.Load(), "an invalid selection cannot request a token or source data")
		})
	}
}

func TestRetrieve_DiscoveryCannotPopulateAReplacementRun(t *testing.T) {
	cfg := retrievalConfig(t)
	entered, release := make(chan struct{}), make(chan struct{})
	localize := cfg.nviLocalize
	cfg.nviLocalize = func(ctx context.Context, bsn string) ([]localizedRecord, error) {
		close(entered)
		<-release
		return localize(ctx, bsn)
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	response := make(chan *http.Response, 1)
	go func() {
		res, err := client.Get(srv.URL + "/demo/ehr/patients/" + key + "/retrieve")
		if err != nil {
			t.Error(err)
		}
		response <- res
	}()
	<-entered
	postForm(t, client, srv, "/demo/patients/"+key+"/release", nil).Body.Close()
	openPatient(t, client, srv, key)
	close(release)
	res := <-response
	require.NotNil(t, res)
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)
	require.Contains(t, res.Header.Get("Content-Type"), "text/html")
}

func TestSourceSelectionCopiesCategories(t *testing.T) {
	store := newRunStore(NewRegistry())
	id, err := store.start("owner", "anna", time.Now().Add(time.Hour), true)
	require.NoError(t, err)
	sources := []localizedSource{{URA: zonnebloemURA, Address: "http://source.example/fhir", Categories: []string{"Condition"}}}
	selection, err := store.rememberSources(id, "owner", sources, 0)
	require.NoError(t, err)
	sources[0].Categories[0] = "MedicationRequest"
	source, ok := store.selectedSource(id, "owner", selection, zonnebloemURA, 0)
	require.True(t, ok)
	require.Equal(t, []string{"Condition"}, source.Categories)
	source.Categories[0] = "AllergyIntolerance"
	again, ok := store.selectedSource(id, "owner", selection, zonnebloemURA, 0)
	require.True(t, ok)
	require.Equal(t, []string{"Condition"}, again.Categories)
}

func TestRetrieveForm_RequiresALiveSessionToSaveSources(t *testing.T) {
	for _, missing := range []bool{true, false} {
		cfg := retrievalConfig(t)
		cfg.Runs = newRunStore(cfg.Locks)
		session := &authSession{ID: "ended-session", ExpiresAt: time.Now().Add(time.Hour)}
		id, err := cfg.Runs.start(lockOwner(session), "anna", session.ExpiresAt, true)
		require.NoError(t, err)
		if !missing {
			cfg.sessions = newSessionStore()
		}
		req := httptest.NewRequest(http.MethodGet, "/demo/ehr/patients/anna/retrieve", nil)
		req.SetPathValue("key", "anna")
		res := httptest.NewRecorder()
		cfg.handleRetrieveForm(res, req, session)
		require.Equal(t, http.StatusSeeOther, res.Code)
		require.Equal(t, "/demo/login", res.Header().Get("Location"))
		cfg.Runs.mu.Lock()
		saved := cfg.Runs.runs[id].sources
		cfg.Runs.mu.Unlock()
		require.Nil(t, saved, "an absent session must not retain a source selection")
	}
}

func TestRetrieve_MissingSelectionDoesNotRediscover(t *testing.T) {
	cfg := retrievalConfig(t)
	lookups, retrievals := 0, 0
	cfg.nviLocalize = func(context.Context, string) ([]localizedRecord, error) {
		lookups++
		return nil, nil
	}
	cfg.retrieveFromSource = func(context.Context, authSession, sourceAddress, string, []string) (sourceRetrieval, error) {
		retrievals++
		return sourceRetrieval{}, nil
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	status, body := postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve", url.Values{"ura": {zonnebloemURA}})
	require.Equal(t, http.StatusConflict, status)
	require.Zero(t, lookups, "a missing selection must prompt an explicit refresh")
	require.Zero(t, retrievals)
	require.Contains(t, body, "Refresh source list")
}

// A recycle proceeds only while nobody holds the patient, but a session can
// open it and discover sources while the restore runs. That discovery read the
// index halfway through the restore and must not outlive it.
func TestRetrieve_RecycleInvalidatesADiscoveryItOverlapped(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.Runs = newRunStore(cfg.Locks)
	retrievals := 0
	retrieve := cfg.retrieveFromSource
	cfg.retrieveFromSource = func(ctx context.Context, s authSession, source sourceAddress, bsn string, cats []string) (sourceRetrieval, error) {
		retrievals++
		return retrieve(ctx, s, source, bsn, cats)
	}
	var srv *httptest.Server
	var presenter *http.Client
	var selection string
	overlapped := make(chan error, 1)
	cfg.recyclePatient = func(context.Context, string) error {
		overlapped <- func() error {
			res, err := presenter.Post(srv.URL+"/demo/ehr/patients/anna/open", "application/x-www-form-urlencoded", nil)
			if err != nil {
				return err
			}
			_ = res.Body.Close()
			res, err = presenter.Get(srv.URL + "/demo/ehr/patients/anna/retrieve")
			if err != nil {
				return err
			}
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			if err != nil {
				return err
			}
			match := selectionInput.FindStringSubmatch(string(body))
			if len(match) != 2 {
				return fmt.Errorf("discovery during the restore offered no selection (status %d)", res.StatusCode)
			}
			selection = match[1]
			return nil
		}()
		return nil
	}
	srv, operator := demoServer(t, cfg)
	presenter = signInViaDezi(t, srv)
	res := postForm(t, operator, srv, "/demo/patients/anna/recycle", nil)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.NoError(t, <-overlapped)

	status, body := postFormAndRead(t, presenter, srv, "/demo/ehr/patients/anna/retrieve",
		url.Values{"ura": {zonnebloemURA}, "selection": {selection}})
	require.Equal(t, http.StatusConflict, status)
	require.Contains(t, body, "Refresh source list")
	require.Zero(t, retrievals, "a discovery the recycle overtook cannot request a token or source data")
}

// A session that does not hold the patient, for instance after its lease ran
// out, is sent back to the list to open it rather than shown a bare error.
func TestRetrieveForm_WithoutAnOpenPatientPointsBackToTheList(t *testing.T) {
	cfg := retrievalConfig(t)
	lookups := 0
	localize := cfg.nviLocalize
	cfg.nviLocalize = func(ctx context.Context, bsn string) ([]localizedRecord, error) {
		lookups++
		return localize(ctx, bsn)
	}
	srv, client := demoServer(t, cfg)
	res, err := client.Get(srv.URL + "/demo/ehr/patients/anna/retrieve")
	require.NoError(t, err)
	require.NoError(t, res.Body.Close())
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo/ehr?notice=patient-not-open", res.Header.Get("Location"))
	require.Zero(t, lookups, "nothing is discovered for a patient this session does not hold")
	_, list := getBody(t, client, srv, "/demo/ehr?notice=patient-not-open")
	require.Contains(t, list, "Open the patient first")
}
