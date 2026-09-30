package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

// A holder can publish an authorization server and nothing to read data from.
// The directory answered, so nothing failed; what is missing is the half this
// screen needs, and saying "could not be addressed" is the difference between a
// holder that cannot be reached and one the index never named.
func TestRetrieveForm_ReportsASourceThatPublishesNoDataEndpoint(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.mcsdResolve = func(_ context.Context, ura string) (sourceAddress, error) {
		return sourceAddress{URA: ura, Name: "Zorgcentrum De Zonnebloem",
			AuthorizationServer: "http://published.example/nuts/oauth2/" + ura}, nil
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, zonnebloemURA, "the NVI did find the holder")
	require.Contains(t, body, "publishes no endpoint to read data from")
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
		sourceConfirmation(t, client, srv, key, "00009999"))
	defer res.Body.Close()

	require.Equal(t, http.StatusConflict, res.StatusCode)
}

func TestRetrieve_RendersTheGrantedDecision(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	status, body := postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

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
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

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
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Metoprolol")
	require.Contains(t, body, "Zorgcentrum De Zonnebloem")
	// De Plataan's own resources stay on the record and stay attributed too,
	// otherwise "every element carries its source" is only true of half of it.
	require.Contains(t, body, "Apixaban")
	require.Contains(t, body, "Ziekenhuis De Plataan")
}

// A denied retrieval must not leave the record claiming a second source, and
// that has to hold when there is something to replace. Starting from an empty
// store proves only that a denial adds nothing; consent can be withdrawn between
// two runs of the same demo, and the data from before it must not survive the
// refusal that follows.
func TestRecord_ShowsNoRetrievedDataAfterADenial(t *testing.T) {
	srv, client, key, refuse := retrievalThatCanTurn(t, chainRefused, http.StatusForbidden)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))
	_, granted := getBody(t, client, srv, "/demo/ehr/patients/"+key)
	require.Contains(t, granted, "Metoprolol", "the first run has to have put data on the record")

	refuse()
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))
	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.NotContains(t, body, "Zorgcentrum De Zonnebloem")
	require.NotContains(t, body, "Metoprolol", "what the refusal replaced must be gone with it")
}

// retrievalThatCanTurn starts a sandbox whose source answers normally until the
// returned function is called, after which it answers with outcome and status.
// The patient is open and the caller holds its lease.
func retrievalThatCanTurn(t *testing.T, outcome chainOutcome, status int) (
	*httptest.Server, *http.Client, string, func()) {
	t.Helper()
	cfg := retrievalConfig(t)
	serves := cfg.retrieveFromSource
	turned := false
	cfg.retrieveFromSource = func(ctx context.Context, session authSession, source sourceAddress,
		bsn string, categories []string) (sourceRetrieval, error) {
		if turned {
			return sourceRetrieval{Source: source, Outcome: outcome,
				Queries: []queryOutcome{{Label: "Patient", Status: status}}}, nil
		}
		return serves(ctx, session, source, bsn, categories)
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	return srv, client, key, func() { turned = true }
}

// Another practitioner's run must not see what this one retrieved.
func TestRecord_RetrievedDataIsScopedToTheSessionThatFetchedIt(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

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
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

	require.Equal(t, http.StatusOK, status)
	require.NotContains(t, body, "Access denied", "nothing was refused")
	require.NotContains(t, body, "Access granted", "and nothing was served")
	require.Contains(t, body, "could not be established")
	require.NotContains(t, body, "Penicilline")
	// No way on to the record: there is nothing there, and offering the link
	// would suggest the retrieval produced something.
	require.NotContains(t, body, `href="/demo/ehr/patients/`+key+`"`)
}

// And the record must not claim a second source after a failure either, with the
// same replacement question: a source that answered a minute ago and is down now
// leaves stale clinical data on screen if the failure does not clear it.
func TestRecord_ShowsNoRetrievedDataAfterAFailure(t *testing.T) {
	srv, client, key, fail := retrievalThatCanTurn(t, chainFailed, http.StatusBadGateway)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))
	_, granted := getBody(t, client, srv, "/demo/ehr/patients/"+key)
	require.Contains(t, granted, "Metoprolol", "the first run has to have put data on the record")

	fail()
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))
	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.NotContains(t, body, "Zorgcentrum De Zonnebloem")
	require.NotContains(t, body, "Metoprolol", "a source that is down does not keep answering from memory")
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
		res := postForm(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve", sourceConfirmation(t, client, srv, key, zonnebloemURA))
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
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Access granted", "the source did authorize the request")
	// The negation is the claim, so it has to be inside the asserted string: an
	// assertion on the tail alone still passed with the "not" taken out.
	require.Contains(t, body, "not everything it authorized came back")
	// Nothing came back, so there is nothing to go and look at.
	require.NotContains(t, body, `href="/demo/ehr/patients/`+key+`"`)
}

// The warning has to survive the click. Retained pages reach the record, so
// without carrying the incompleteness with them "View the retrieved data" opens
// an ordinary-looking record holding only part of what the source has, with
// nothing on screen saying so.
func TestRecord_KeepsTheIncompleteWarningWithTheData(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.retrieveFromSource = func(_ context.Context, _ authSession, source sourceAddress, _ string, _ []string) (sourceRetrieval, error) {
		return sourceRetrieval{
			Source: source, Outcome: chainGranted, PatientID: "zb-anna",
			Queries: []queryOutcome{
				{Label: "Patient", Status: http.StatusOK},
				{Label: "Conditions", Status: http.StatusOK, Truncated: "this result is incomplete"},
			},
			Items: []recordItem{{Section: "Conditions", Title: "Diabetes mellitus type 2", Source: source.Name}},
		}, nil
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Diabetes mellitus type 2", "the part that did arrive is shown")
	require.Contains(t, body, "not everything", "and the record says it is not the whole picture")
}

// A retrieval that came back whole must not carry a warning.
func TestRecord_SaysNothingAboutCompletenessWhenItIsComplete(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.Contains(t, body, "Metoprolol")
	require.NotContains(t, body, "not everything")
}

// Recycle is how a presenter starts the scenario over: it puts the seeded
// resources back at both providers. A retrieval this session already made is a
// copy of what the source held before that, so leaving it in place means the
// record screen keeps serving data the recycle just replaced, and the run that
// follows is not the clean one it claims to be.
func TestRecycle_DiscardsWhatWasRetrievedForThatPatient(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))
	_, before := getBody(t, client, srv, "/demo/ehr/patients/"+key)
	require.Contains(t, before, "Metoprolol", "the retrieval has to be on the record for this test to mean anything")

	// Recycle refuses a locked patient, which is the state a demo is in while the
	// record is open, so the lease is released first exactly as the presenter would.
	postForm(t, client, srv, "/demo/patients/"+key+"/release", nil).Body.Close()
	res := postForm(t, client, srv, "/demo/patients/"+key+"/recycle", nil)
	res.Body.Close()
	require.Equal(t, "/demo?notice=recycle-done", res.Header.Get("Location"))

	openPatient(t, client, srv, key)
	_, after := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.NotContains(t, after, "Metoprolol",
		"the retrieved copy was replaced by the recycle and must not survive it")
	require.NotContains(t, after, "Zorgcentrum De Zonnebloem",
		"and the source must no longer be listed as one this record draws on")
}

// The same question for the global reset, but it cannot be asked of the screen:
// a reset ends every session, and the store is keyed by session, so the next
// sign-in sees nothing either way. What has to hold is that the data is gone
// rather than merely unreachable, so this reads the store. Another
// organization's clinical data sitting behind an owner that no longer exists is
// exactly what the reset is there to prevent.
func TestReset_LeavesNoRetrievedDataBehind(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))
	require.NotZero(t, retrievalsHeld(cfg.Retrievals),
		"the retrieval has to be in the store for this test to mean anything")

	// Override, because the record this session has open holds the lease and a
	// plain reset refuses to run past an active one.
	postForm(t, client, srv, "/demo/reset", url.Values{"override": {"true"}}).Body.Close()

	require.Zero(t, retrievalsHeld(cfg.Retrievals),
		"a reset restores the whole pool, so no retrieved copy may outlive it")
}

// retrievalsHeld counts what the store is holding, across owners.
func retrievalsHeld(store *retrievalStore) int {
	store.mu.Lock()
	defer store.mu.Unlock()
	held := 0
	for _, byPatient := range store.byOwner {
		held += len(byPatient)
	}
	return held
}

// The screen explains the privacy property of the localization step, and it has
// to describe the one this stack actually has. The BSN does travel: the
// application sends it to the Knooppunt, which converts it to a pseudonym before
// the index sees it, and it goes to the source as well once a retrieval is
// confirmed. Claiming it never leaves overstates a real protection into one
// nobody implemented.
func TestRetrieveForm_DescribesThePseudonymisationItActuallyPerforms(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve")

	require.NotContains(t, body, "never travels",
		"the BSN does travel; what it does not reach is the index")
	// The whole clause, not a fragment of it: asserting on a few words let the
	// sentence be negated or narrowed around them while the test stayed green.
	// Whitespace is normalized because the template wraps this paragraph.
	require.Contains(t, whitespaceNormalized(body),
		"The Knooppunt converts the BSN into a pseudonym, and the national referral index is "+
			"searched on that pseudonym rather than on the BSN itself.",
		"the screen has to describe the pseudonymisation this build performs")
}

// whitespaceNormalized collapses runs of whitespace to single spaces, so an
// assertion can name a whole sentence that the template wraps across lines.
func whitespaceNormalized(body string) string {
	return strings.Join(strings.Fields(body), " ")
}

// Recycle is not transactional: vectors.RecyclePatient PUTs the fixtures back in
// sequence and verifies afterwards, so an ordinary error can follow resources
// that were already replaced. Clearing only on a clean or partial result left
// the pre-recycle answer on the record screen while the data behind it had
// changed, which is the state recycle exists to leave behind.
func TestRecycle_DiscardsRetrievalsEvenWhenItReportsFailure(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.recyclePatient = func(context.Context, string) error {
		return errors.New("the NVI verification after the restore failed")
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))
	require.NotZero(t, retrievalsHeld(cfg.Retrievals),
		"the retrieval has to be in the store for this test to mean anything")

	postForm(t, client, srv, "/demo/patients/"+key+"/release", nil).Body.Close()
	res := postForm(t, client, srv, "/demo/patients/"+key+"/recycle", nil)
	res.Body.Close()

	require.Equal(t, http.StatusInternalServerError, res.StatusCode)
	require.Zero(t, retrievalsHeld(cfg.Retrievals),
		"the restore had already run, so the retrieved copy may not survive the error")
}

// The lease admits a retrieval; it is not held across the network call, and
// releasing it or letting it expire does not cancel one already running. So a
// recycle can clear the store while a retrieval of the pre-recycle data is still
// on its way back, and the publication puts exactly what recycle removed back in
// place.
func TestRecycle_ARetrievalInFlightIsNotStoredAfterwards(t *testing.T) {
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
		res := postForm(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve", sourceConfirmation(t, client, srv, key, zonnebloemURA))
		_ = res.Body.Close()
	}()

	<-reachedSource
	// What a presenter does while the spinner runs: move on, which drops the
	// lease, and then reset the patient.
	postForm(t, client, srv, "/demo/patients/"+key+"/release", nil).Body.Close()
	postForm(t, client, srv, "/demo/patients/"+key+"/recycle", nil).Body.Close()
	close(release)
	<-done

	require.Zero(t, retrievalsHeld(cfg.Retrievals),
		"a retrieval of the pre-recycle data must not land in a store the recycle just cleared")
}

// A retrieval that was granted and produced nothing renderable is still a
// retrieval, and the reason it produced nothing is the only thing worth showing.
// Carrying the outcome only when items came with it meant the emptiest results,
// the ones a reader is most likely to misread as "this source had nothing", were
// exactly the ones that reached the record screen with nothing to say.
func TestRecord_WarnsAboutAnAnswerThatYieldedNoItems(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.retrieveFromSource = func(_ context.Context, _ authSession, source sourceAddress, _ string, _ []string) (sourceRetrieval, error) {
		return sourceRetrieval{
			Source: source, Outcome: chainGranted,
			Queries: []queryOutcome{{Label: "Condition", Status: http.StatusOK,
				Truncated: "1 resource(s) in this answer could not be rendered, so this result is incomplete"}},
		}, nil
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.Contains(t, body, "but not everything it authorized",
		"the record has to carry the warning even when the retrieval put nothing on it")
	require.NotContains(t, body, "Zorgcentrum De Zonnebloem</b>",
		"and it may not list a source it drew no data from")
}

// A holder the directory can address but publishes no authorization server for
// cannot be asked for a token. Building one out of the URA is what this replaced:
// it guesses an address, and a guess that happens to resolve asks a server nobody
// said was the right one.
func TestRetrieve_RefusesASourceWithNoPublishedAuthorizationServer(t *testing.T) {
	cfg := retrievalConfig(t)
	cfg.mcsdResolve = func(_ context.Context, ura string) (sourceAddress, error) {
		return sourceAddress{URA: ura, Name: "Zorgcentrum De Zonnebloem",
			Address: "http://pep-zonnebloem:8080/fhir"}, nil
	}
	cfg.retrieveFromSource = nil
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	status, body := postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

	require.Equal(t, http.StatusBadGateway, status)
	require.Contains(t, body, "authorization server",
		"the reason has to name what the directory did not publish")
}

// The address the directory published has to survive the trip from the
// addressing step to the token request. Every other test on this path replaces
// retrieveFromSource with a fake, so the one closure that reads the resolved
// address was covered nowhere: it dropped the field between localization and
// retrieval and the whole chain failed with "the directory publishes no
// authorization server" against a directory that published one.
func TestRetrieve_AsksTheAuthorizationServerTheDirectoryPublished(t *testing.T) {
	node, recorded := recordingNode(t, http.StatusOK, `{"access_token":"the-token"}`)
	t.Setenv("KNOOPPUNT_INTERNAL_URL", node.URL)

	cfg := retrievalConfig(t)
	cfg.mcsdResolve = func(_ context.Context, ura string) (sourceAddress, error) {
		return sourceAddress{URA: ura, Name: "Zorgcentrum De Zonnebloem",
			Address:             "http://pep-zonnebloem:8080/fhir",
			AuthorizationServer: "http://published.example/nuts/oauth2/" + ura}, nil
	}
	// nil so NewMux assembles the production closure: that is the code under test.
	cfg.retrieveFromSource = nil
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

	require.Equal(t, "http://published.example/nuts/oauth2/"+zonnebloemURA,
		recorded.body["authorization_server"],
		"the token has to be requested from the address the directory gave")
}

// The sign-in demo asks a token of an authorization server too, and reads that
// address from the directory under this installation's own URA. The wallet the
// request is made from is a separate choice, the subject in the path of the
// internal call, so the two differ here without either being wrong.
func TestAuthorize_AsksTheAuthorizationServerTheDirectoryPublished(t *testing.T) {
	var tokenRequest map[string]any
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/request-service-access-token") {
			_ = json.NewDecoder(r.Body).Decode(&tokenRequest)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"the-token"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"active":true,"organization_ura":"00000010"}`))
	}))
	t.Cleanup(node.Close)
	t.Setenv("KNOOPPUNT_INTERNAL_URL", node.URL)

	cfg := retrievalConfig(t)
	cfg.mcsdResolve = func(_ context.Context, ura string) (sourceAddress, error) {
		return sourceAddress{URA: ura, Name: "Ziekenhuis De Plataan",
			Address:             "http://pep-plataan:8080/fhir",
			AuthorizationServer: "http://published.example/nuts/oauth2/" + ura}, nil
	}
	srv, client := demoServer(t, cfg)

	postForm(t, client, srv, "/demo/authorize", nil).Body.Close()

	require.Equal(t, "http://published.example/nuts/oauth2/"+plataanURA,
		tokenRequest["authorization_server"],
		"the sign-in demo has to ask the address published for this installation's own URA")
}

// Showing what is in the access token needs an authorization server and nothing
// else: this screen reads no data, so a holder's FHIR endpoint being out of
// service has no bearing on it. The real directory resolver is wired in rather
// than a hand-built sourceAddress, because that independence is a property of
// the selector and an injected address cannot exercise it.
func TestAuthorize_NeedsNoDataEndpointOfItsOwn(t *testing.T) {
	var tokenRequest map[string]any
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/request-service-access-token") {
			_ = json.NewDecoder(r.Body).Decode(&tokenRequest)
			_, _ = w.Write([]byte(`{"access_token":"the-token"}`))
			return
		}
		_, _ = w.Write([]byte(`{"active":true,"organization_ura":"00000010"}`))
	}))
	t.Cleanup(node.Close)
	t.Setenv("KNOOPPUNT_INTERNAL_URL", node.URL)

	// A directory entry holding an authorization server and no data endpoint.
	directory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","entry":[
		  {"search":{"mode":"match"},"resource":{"resourceType":"Organization","id":"plataan",
		    "name":"Ziekenhuis De Plataan",
		    "identifier":[{"system":"http://fhir.nl/fhir/NamingSystem/ura","value":"00000010"}],
		    "endpoint":[{"reference":"Endpoint/pl-oauth","type":"Endpoint"}]}},
		  {"search":{"mode":"include"},"resource":{"resourceType":"Endpoint","id":"pl-oauth",
		    "status":"active","address":"http://published.example/nuts/oauth2/00000010",
		    "connectionType":{"system":"http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-authorization-server-cs","code":"oauth-nuts"}}}
		]}`))
	}))
	t.Cleanup(directory.Close)
	base, err := url.Parse(directory.URL + "/fhir")
	require.NoError(t, err)

	cfg := retrievalConfig(t)
	cfg.mcsdResolve = mcsdResolveFunc(base)
	srv, client := demoServer(t, cfg)

	postForm(t, client, srv, "/demo/authorize", nil).Body.Close()

	require.Equal(t, "http://published.example/nuts/oauth2/00000010",
		tokenRequest["authorization_server"],
		"the token request goes out on the published address, with no data endpoint in sight")
}

// The share status and the retrieved data sit on one screen, so the status has
// to be a statement about findability and not about where this patient's data
// lives. "This record exists only in De Plataan's own store" read as a claim
// about the list underneath it, which by then held three items from another
// care provider.
func TestRecord_DoesNotDenyTheDataItIsShowing(t *testing.T) {
	cfg := retrievalConfig(t)
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)

	require.Contains(t, body, "Metoprolol", "the retrieval has to be on screen for this to mean anything")
	require.Contains(t, body, "Not findable yet",
		"and the patient is still unshared, so the status itself is right")
	require.NotContains(t, body, "exists only in De Plataan's own store",
		"which is not a thing this screen can say while showing another provider's data")
}

// A retrieval that never reaches the source is still an attempt that replaced
// the last one. When the chain answers with a failed outcome the record is
// cleared; when it answers with a Go error, because the token could not be
// obtained or the directory published no authorization server, the handler used
// to return 502 and leave the previous answer in place. The record then showed
// another provider's clinical data as the current result of an attempt that
// produced nothing.
func TestRecord_ShowsNoRetrievedDataAfterATokenFailure(t *testing.T) {
	cfg := retrievalConfig(t)
	inner := cfg.retrieveFromSource
	failing := false
	cfg.retrieveFromSource = func(ctx context.Context, s authSession, source sourceAddress, bsn string, cats []string) (sourceRetrieval, error) {
		if failing {
			return sourceRetrieval{}, errors.New("request service access token: 503 Service Unavailable")
		}
		return inner(ctx, s, source, bsn, cats)
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))
	_, granted := getBody(t, client, srv, "/demo/ehr/patients/"+key)
	require.Contains(t, granted, "Metoprolol", "the first run has to have put data on the record")

	failing = true
	status, _ := postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))
	require.Equal(t, http.StatusBadGateway, status)

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)
	require.NotContains(t, body, "Metoprolol",
		"what the failed attempt replaced must be gone with it")
	require.NotContains(t, body, "Zorgcentrum De Zonnebloem",
		"and the record may not name a source it drew nothing from")
}

// A failed attempt discards what it replaced, but only if what it finds is still
// its own to discard. An attempt that started before a recycle and fails after
// one has nothing to say about the answer retrieved since: that one describes
// the restored data and was published under a later generation.
func TestRetrieve_AFailureFromBeforeARecycleLeavesTheFreshAnswerAlone(t *testing.T) {
	cfg := retrievalConfig(t)
	inner := cfg.retrieveFromSource
	reachedSource := make(chan struct{})
	release := make(chan struct{})
	var calls int
	cfg.retrieveFromSource = func(ctx context.Context, s authSession, source sourceAddress, bsn string, cats []string) (sourceRetrieval, error) {
		calls++
		if calls == 1 {
			close(reachedSource)
			<-release
			return sourceRetrieval{}, errors.New("request service access token: 503 Service Unavailable")
		}
		return inner(ctx, s, source, bsn, cats)
	}
	srv, client := demoServer(t, cfg)
	key := annaKey(t)
	openPatient(t, client, srv, key)

	done := make(chan struct{})
	go func() {
		defer close(done)
		res := postForm(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve", sourceConfirmation(t, client, srv, key, zonnebloemURA))
		_ = res.Body.Close()
	}()
	<-reachedSource

	// The patient is reset and retrieved again while the first attempt hangs.
	postForm(t, client, srv, "/demo/patients/"+key+"/release", nil).Body.Close()
	postForm(t, client, srv, "/demo/patients/"+key+"/recycle", nil).Body.Close()
	openPatient(t, client, srv, key)
	_, _ = postFormAndRead(t, client, srv, "/demo/ehr/patients/"+key+"/retrieve",
		sourceConfirmation(t, client, srv, key, zonnebloemURA))
	require.NotZero(t, retrievalsHeld(cfg.Retrievals), "the second retrieval has to be stored")

	close(release)
	<-done

	_, body := getBody(t, client, srv, "/demo/ehr/patients/"+key)
	require.Contains(t, body, "Metoprolol",
		"the answer retrieved after the recycle is not the failed attempt's to discard")
}
