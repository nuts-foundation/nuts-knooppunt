package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
)

const zonnebloemURA = "00000020"

// retrievalConfig wires the three legs of the chain with fakes: the NVI answer,
// the directory answer, and the retrieval itself.
func retrievalConfig(t *testing.T) Config {
	t.Helper()
	cfg, _, _ := fakeConfig()
	// Set here rather than left to NewMux, which takes Config by value: without
	// this the test and the running mux would hold different stores and a test
	// could not see what the handlers stored.
	cfg.Retrievals = newRetrievalStore()
	cfg.nviLookup = categoriesByBSN(nil, nil)
	cfg.nviLocalize = func(context.Context, string) ([]localizedRecord, error) {
		return []localizedRecord{{
			CustodianURA: zonnebloemURA,
			Categories: []string{nvi.CategoryAllergyIntolerance, nvi.CategoryCondition,
				nvi.CategoryMedicationRequest, nvi.CategoryPatient},
		}}, nil
	}
	cfg.mcsdResolve = func(_ context.Context, ura string) (sourceAddress, error) {
		return sourceAddress{URA: ura, Name: "Zorgcentrum De Zonnebloem",
			Address: "http://pep-zonnebloem:8080/fhir"}, nil
	}
	cfg.retrieveFromSource = func(_ context.Context, _ authSession, source sourceAddress, _ string, _ []string) (sourceRetrieval, error) {
		return sourceRetrieval{
			Source: source, Outcome: chainGranted, PatientID: "zb-anna",
			Queries: []queryOutcome{{Label: "Patient", Status: http.StatusOK}},
			Items: []recordItem{
				{Section: "Allergies", Title: "Penicilline", Detail: "2019", Source: source.Name},
				{Section: "Medication", Title: "Metoprolol", Detail: "Metoprolol 50 mg 1dd", Source: source.Name},
			},
		}, nil
	}
	return cfg
}

func annaKey(t *testing.T) string {
	t.Helper()
	anna, ok := pool.PatientByKey("anna")
	require.True(t, ok)
	return anna.Key
}

// The two generic functions answer different questions and the screen has to
// show which said what: the NVI names the holder and the data categories, GF
// Addressing turns that into an endpoint. Collapsing them would hide that the
// index never holds an address.
func TestRetrieveForm_SeparatesLocalizationFromAddressing(t *testing.T) {
	cfg := retrievalConfig(t)
	var retrieved bool
	inner := cfg.retrieveFromSource
	cfg.retrieveFromSource = func(ctx context.Context, s authSession, source sourceAddress, bsn string, cats []string) (sourceRetrieval, error) {
		retrieved = true
		return inner(ctx, s, source, bsn, cats)
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Zorgcentrum De Zonnebloem")
	require.Contains(t, body, zonnebloemURA)
	require.Contains(t, body, nvi.CategoryMedicationRequest)
	require.Contains(t, body, "http://pep-zonnebloem:8080/fhir")
	require.False(t, retrieved, "nothing may be retrieved before the practitioner confirms")
}

// De Plataan's own localization records describe the record already on screen.
// Offering them as a source to retrieve from would present the hospital's own
// data as an external find.
func TestRetrieveForm_ExcludesOurOwnRecords(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.nviLocalize = func(context.Context, string) ([]localizedRecord, error) {
		return []localizedRecord{
			{CustodianURA: plataanURA, Categories: []string{nvi.CategoryCondition}},
			{CustodianURA: zonnebloemURA, Categories: []string{nvi.CategoryCondition}},
		}, nil
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, zonnebloemURA)
	require.NotContains(t, body, "URA "+plataanURA,
		"our own registration is not a source to retrieve from")
}

// An organization the index names but the directory cannot address is a real
// state of the chain, and the screen has to say which of the two steps fell
// short rather than presenting the source as simply absent.
func TestRetrieveForm_ReportsASourceItCannotAddress(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.mcsdResolve = func(context.Context, string) (sourceAddress, error) {
		return sourceAddress{}, errors.New("publishes no usable endpoint")
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, zonnebloemURA, "the NVI did find the holder")
	require.Contains(t, body, "could not be addressed")
	require.NotContains(t, body, "Retrieve from")
}

// The confirmation names a source. Accepting a URA the index never reported
// would let the screen drive a retrieval the localization step never justified.
func TestRetrieve_RefusesASourceTheIndexDidNotReport(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	res := postForm(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		url.Values{"ura": {"00009999"}})
	defer res.Body.Close()

	require.Equal(t, http.StatusConflict, res.StatusCode)
}

func TestRetrieve_RendersTheGrantedDecision(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	status, body := postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		url.Values{"ura": {zonnebloemURA}})

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Access granted")
	require.Contains(t, body, `<table class="kv-table">`)
	require.Contains(t, body, `<th scope="row">Patient</th>`)
	require.Contains(t, body, "Zorgcentrum De Zonnebloem")
	// The disclaimer is an acceptance criterion, not decoration: the breakdown
	// below it is narration, and the screen has to say so.
	require.Contains(t, body, "demonstration purposes")
	// This page reports the decision; the data itself lives on the record, and a
	// granted decision has to offer the way there.
	require.Contains(t, body, `href="/demo/ehr/patients/`+key+`"`)
}

// A refusal must leave the screen with no source data on it at all.
func TestRetrieve_RendersADenialWithoutData(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.retrieveFromSource = func(_ context.Context, _ authSession, source sourceAddress, _ string, _ []string) (sourceRetrieval, error) {
		return sourceRetrieval{
			Source: source, Outcome: chainRefused,
			Queries: []queryOutcome{{Label: "Patient", Status: http.StatusForbidden}},
		}, nil
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	status, body := postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		url.Values{"ura": {zonnebloemURA}})

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Access denied")
	require.Contains(t, body, `class="page-header auth-hero denied"`, "a refusal must use the denied visual state")
	require.NotContains(t, body, "Penicilline")
	require.NotContains(t, body, "Metoprolol")
}

// The point of the enriched record: every retrieved element names the source it
// came from, next to the hospital's own data.
func TestRecord_ShowsRetrievedDataAttributedToItsSource(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		url.Values{"ura": {zonnebloemURA}})

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Metoprolol")
	require.Contains(t, body, "Zorgcentrum De Zonnebloem")
	// De Plataan's own resources stay on the record and stay attributed too,
	// otherwise "every element carries its source" is only true of half of it.
	require.Contains(t, body, "Apixaban")
	require.Contains(t, body, "Ziekenhuis De Plataan")
}

// A denied retrieval must not leave the record claiming a second source.
func TestRecord_ShowsNoRetrievedDataAfterADenial(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.retrieveFromSource = func(_ context.Context, _ authSession, source sourceAddress, _ string, _ []string) (sourceRetrieval, error) {
		return sourceRetrieval{Source: source, Outcome: chainRefused,
			Queries: []queryOutcome{{Label: "Patient", Status: http.StatusForbidden}}}, nil
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		url.Values{"ura": {zonnebloemURA}})

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.NotContains(t, body, "Zorgcentrum De Zonnebloem")
}

// Another practitioner's run must not see what this one retrieved.
func TestRecord_RetrievedDataIsScopedToTheSessionThatFetchedIt(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		url.Values{"ura": {zonnebloemURA}})

	other := signInViaDezi(t, srv)
	_, body := getBody(t, other, srv, "/demo/ehr/patients/"+key)

	require.NotContains(t, body, "Metoprolol")
}

// A source that failed to answer did not refuse. Rendering a 502 or a timeout as
// "the source refused the request" asserts a decision nobody took, and sends the
// presenter looking at consent and credentials instead of at a source that is
// down. This is the same rule the share cards follow.
func TestRetrieve_AFailureIsNotRenderedAsADenial(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.retrieveFromSource = func(_ context.Context, _ authSession, source sourceAddress, _ string, _ []string) (sourceRetrieval, error) {
		return sourceRetrieval{
			Source: source, Outcome: chainFailed,
			Queries: []queryOutcome{{Label: "Patient", Status: http.StatusBadGateway}},
		}, nil
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	status, body := postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		url.Values{"ura": {zonnebloemURA}})

	require.Equal(t, http.StatusOK, status)
	require.NotContains(t, body, "Access denied", "nothing was refused")
	require.NotContains(t, body, "Access granted", "and nothing was served")
	require.Contains(t, body, "could not be established")
	require.NotContains(t, body, "Penicilline")
	// No way on to the record: there is nothing there, and offering the link
	// would suggest the retrieval produced something.
	require.NotContains(t, body, `href="/demo/ehr/patients/`+key+`"`)
}

// And the record must not claim a second source after a failure either.
func TestRecord_ShowsNoRetrievedDataAfterAFailure(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.retrieveFromSource = func(_ context.Context, _ authSession, source sourceAddress, _ string, _ []string) (sourceRetrieval, error) {
		return sourceRetrieval{Source: source, Outcome: chainFailed,
			Queries: []queryOutcome{{Label: "Patient", Status: http.StatusBadGateway}}}, nil
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		url.Values{"ura": {zonnebloemURA}})

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.NotContains(t, body, "Zorgcentrum De Zonnebloem")
}

// The patient list bounds every NVI call so one slow round trip costs a chip
// rather than the page. Localization runs on the same index from the retrieval
// screens and had no deadline at all, so a source that accepts a connection and
// never answers held the screen open indefinitely instead of reaching the
// failure state the screen is built to render.
func TestLocalizeSources_IsBounded(t *testing.T) {
	cfg := retrievalConfig(t)
	previous := nviLocalizeTimeout
	nviLocalizeTimeout = 50 * time.Millisecond
	t.Cleanup(func() { nviLocalizeTimeout = previous })
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	cfg.nviLocalize = func(ctx context.Context, _ string) ([]localizedRecord, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return nil, nil
		}
	}

	start := time.Now()
	_, err := cfg.localizeSources(t.Context(), "999900006")

	require.Error(t, err, "an index that never answers must fail, not hang")
	require.Less(t, time.Since(start), 5*time.Second,
		"and it must fail on its own deadline rather than the caller's")
}

// Signing out discards what the session retrieved. A retrieval still in flight
// at that moment used to write its result back afterwards, into an owner that no
// longer exists, where no later sweep could reach it: clinical data from another
// organization, kept past the session that was authorized to see it, with
// nothing left to clear it.
func TestRetrieve_ARetrievalInFlightAtLogoutIsNotKept(t *testing.T) {
	cfg := retrievalConfig(t)
	reachedSource := make(chan struct{})
	release := make(chan struct{})
	inner := cfg.retrieveFromSource
	cfg.retrieveFromSource = func(ctx context.Context, s authSession, source sourceAddress, bsn string, cats []string) (sourceRetrieval, error) {
		close(reachedSource)
		<-release
		return inner(ctx, s, source, bsn, cats)
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	done := make(chan struct{})
	go func() {
		defer close(done)
		res := postForm(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve", url.Values{"ura": {zonnebloemURA}})
		_ = res.Body.Close()
	}()

	<-reachedSource
	res := postForm(t, client, srv, "/demo/logout", nil)
	require.NoError(t, res.Body.Close())
	close(release)
	<-done

	cfg.Retrievals.mu.Lock()
	defer cfg.Retrievals.mu.Unlock()
	require.Empty(t, cfg.Retrievals.byOwner,
		"a retrieval that finished after its session ended must not be stored")
}

// Authorization succeeding is not the same as the retrieval succeeding. A screen
// that says access was granted, narrates four passed checks and links to the
// data, while nothing could actually be fetched, overstates the run.
func TestRetrieve_SaysWhenTheRetrievalWasIncomplete(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.retrieveFromSource = func(_ context.Context, _ authSession, source sourceAddress, _ string, _ []string) (sourceRetrieval, error) {
		return sourceRetrieval{
			Source: source, Outcome: chainGranted, PatientID: "zb-anna",
			Queries: []queryOutcome{
				{Label: "Patient", Status: http.StatusOK},
				{Label: "Conditions", Status: http.StatusServiceUnavailable},
			},
		}, nil
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	status, body := postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		url.Values{"ura": {zonnebloemURA}})

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Access granted", "the source did authorize the request")
	require.Contains(t, body, "everything it authorized came back")
	// Nothing came back, so there is nothing to go and look at.
	require.NotContains(t, body, `href="/demo/ehr/patients/`+key+`"`)
}
