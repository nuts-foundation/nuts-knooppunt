package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/bsnutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/nvi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// acceptanceServer starts the real stack and the sandbox against it, signed in.
func acceptanceServer(t *testing.T) (harness.Details, *httptest.Server, *http.Client) {
	t.Helper()
	h := harness.Start(t)
	require.NoError(t, vectors.SeedNVI(t.Context(), h.KnooppuntInternalBaseURL))

	cfg := Config{Locks: NewRegistry()}
	cfg.nviLookup, cfg.nviRegister = nviFuncs(h.KnooppuntInternalBaseURL, pool.PlataanClientID)
	cfg.mitzSubscribe = mitzSubscribeFunc(h.KnooppuntInternalBaseURL, plataanURA, "Z3")
	mockBase, err := url.Parse(h.MockMitzXACML.GetURL())
	require.NoError(t, err)
	cfg.mitzSubscribed = mitzSubscribedFunc(mockBase, plataanURA)

	t.Setenv("DEZI_INTERNAL_BASE_URL", fakeDezi(t).URL)
	srv := httptest.NewServer(NewMux(cfg))
	t.Cleanup(srv.Close)
	return h, srv, signInViaDezi(t, srv)
}

// rawNVILists reads the upstream NVI store directly, bypassing the Knooppunt.
//
// Reading back through the same component that wrote would let a consistently
// wrong tokenization find itself and pass. The store holds a pseudonym; the
// interceptor converts a search token to that pseudonym and mints a token again
// on read, so searching with a freshly derived token finds what was written.
// "nvi" is the audience the harness configures (test/e2e/harness/entrypoint.go);
// the fake pseudonymizer derives the token from the BSN and that recipient.
func rawNVILists(t *testing.T, h harness.Details, bsn string) []fhir.List {
	t.Helper()
	token, err := bsnutil.CreateTransportToken(bsn, "nvi")
	require.NoError(t, err)

	client := fhirclient.New(h.Vectors.NVI.FHIRBaseURL, http.DefaultClient, nil)
	var bundle fhir.Bundle
	require.NoError(t, client.SearchWithContext(t.Context(), "List", url.Values{
		"subject:identifier": {coding.BSNTransportTokenNamingSystem + "|" + token},
	}, &bundle))

	var lists []fhir.List
	for _, entry := range bundle.Entry {
		var list fhir.List
		require.NoError(t, json.Unmarshal(entry.Resource, &list))
		lists = append(lists, list)
	}
	return lists
}

func postAndRead(t *testing.T, client *http.Client, srv *httptest.Server, path string) (int, string) {
	t.Helper()
	res := postForm(t, client, srv, path, nil)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

// filterByCustodian keeps the Lists carrying one custodian in their localization
// extension. nvi.custodianOf does the same thing but is unexported.
func filterByCustodian(lists []fhir.List, custodianURA string) []fhir.List {
	var mine []fhir.List
	for _, list := range lists {
		for _, ext := range list.Extension {
			if ext.Url != "http://minvws.github.io/generiekefuncties-docs/StructureDefinition/nl-gf-localization-custodian" {
				continue
			}
			if ext.ValueReference != nil && ext.ValueReference.Identifier != nil &&
				ext.ValueReference.Identifier.Value != nil &&
				*ext.ValueReference.Identifier.Value == custodianURA {
				mine = append(mine, list)
				// One match is enough: a List carrying the extension twice would
				// otherwise be counted twice and read as a delete that missed.
				break
			}
		}
	}
	return mine
}

// AC1, AC2 and AC3: sharing through the UI publishes one localization record per
// data category, creates the Mitz subscription, and confirms both on screen.
func TestAcceptance_ShareRegistersBothAndConfirms(t *testing.T) {
	h, srv, client := acceptanceServer(t)
	anna, ok := pool.PatientByKey("anna")
	require.True(t, ok)

	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()
	status, body := postAndRead(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Localization records published")
	require.Contains(t, body, "Consent subscription started")

	lists := rawNVILists(t, h, anna.BSN)
	plataanLists := filterByCustodian(lists, plataanURA)
	require.Equal(t, anna.PlataanCategories(), nvi.CategoriesOf(plataanLists))
	for _, list := range plataanLists {
		require.Equal(t, nvi.DataCategorySystem, *list.Code.Coding[0].System)
		require.Equal(t, "withheld", *list.EmptyReason.Coding[0].Code)
		// Not List.source: the interceptor rewrites source.identifier into a
		// Device reference on storage, so the raw record has no identifier to
		// read. That the client scope survives is proved where it is used, by
		// TestAcceptance_SharingTwiceIsSafe: a delete that missed would leave
		// six Lists instead of three.
		require.NotNil(t, list.Source, "the profile requires a source")
		// Deliberately nothing about the subject here. The interceptor re-mints
		// the identifier on every read with a hardcoded system, so asserting that
		// system only restates what the search having matched already implies,
		// and comparing the re-minted value against the BSN says nothing about
		// what is stored. That the NVI holds a pseudonym is proved where it is
		// observable, in component/nvi's tokenization tests.
	}

	subs := h.MockMitzXACML.GetSubscriptions()
	require.Len(t, subs, 1)
	require.Contains(t, subs[0].Criteria, "providerid="+plataanURA)
	require.Contains(t, subs[0].Criteria, "patientid="+anna.BSN)
}

// AC4: repeating the whole flow through the UI leaves one record per category
// and one subscription.
func TestAcceptance_SharingTwiceIsSafe(t *testing.T) {
	h, srv, client := acceptanceServer(t)
	anna, ok := pool.PatientByKey("anna")
	require.True(t, ok)

	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()
	firstStatus, firstBody := postAndRead(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share")
	status, body := postAndRead(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share")

	// Both shares, for the same reason. A first share that failed and wrote
	// nothing would leave the second one's fresh create at the same three-List
	// count, which reads as convergence just as convincingly.
	require.Equal(t, http.StatusOK, firstStatus)
	require.Contains(t, firstBody, "Localization records published")

	// The second share has to have succeeded, or the count below proves nothing:
	// a failed registration answers 200 with a Failed card and writes nothing,
	// leaving exactly the three Lists the first share created. That is
	// indistinguishable from convergence unless the success is asserted. The
	// title is unique to the success branch.
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Localization records published")

	// What each share actually did with the subscription, which the count below
	// cannot show: one subscription is the same number whether the second share
	// created it or found it. The mock answers a repeat with the existing
	// subscription and still returns 201, so a card reading "started" on the
	// second share would be the demo claiming an action Mitz did not take.
	require.Contains(t, firstBody, "Consent subscription started")
	require.Contains(t, body, "Consent subscription already active")
	require.NotContains(t, body, "Consent subscription started")

	plataanLists := filterByCustodian(rawNVILists(t, h, anna.BSN), plataanURA)
	require.Len(t, plataanLists, len(anna.PlataanCategories()),
		"two shares must leave one List per category, not two")
	require.Len(t, h.MockMitzXACML.GetSubscriptions(), 1)
}

// A List whose data category comes from another code system is still a record
// other providers can find. This crosses the real adapter, which the unit test
// cannot: that one injects an already-converted record count, so deriving the
// count from the recognized categories inside nviFuncs would leave it green while
// a findable patient reads as local-only again.
func TestAcceptance_AnUnrecognizedListStillReadsAsShared(t *testing.T) {
	h, srv, client := acceptanceServer(t)
	anna, ok := pool.PatientByKey("anna")
	require.True(t, ok)

	// What an earlier seed left behind: De Plataan's custodian, a code system
	// this build does not know, and a source id the client-scoped delete does not
	// match. Compose reuses an unchanged hapi-fhir container, so a stack upgraded
	// into this branch still holds records like it.
	legacy := nvi.BuildList(nvi.Registration{
		CustodianURA: plataanURA, BSN: anna.BSN, ClientID: "PLATAAN-EHR-legacy",
	}, "Condition")
	legacy.Code.Coding[0].System = to.Ptr("http://example.test/legacy-categories")
	postRawNVIList(t, h, legacy)

	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key)

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "data category this build recognizes",
		"a findable patient whose categories cannot be named must say so")
	require.NotContains(t, body, "Not findable yet")
	require.NotContains(t, body, "exists only in De Plataan's own store")
}

// A duplicate is not a stranger. Two Lists of the same recognized category, which
// a client-id override or a second sandbox process can leave behind, must not be
// reported as records whose category this build cannot name: the category is
// Condition, in the vocabulary, and only the deduplication makes it look absent.
func TestAcceptance_ADuplicateCategoryIsNotReportedAsUnnamed(t *testing.T) {
	h, srv, client := acceptanceServer(t)
	anna, ok := pool.PatientByKey("anna")
	require.True(t, ok)

	// The same category this share is about to publish, under a client id the
	// scoped delete does not match, so both survive.
	duplicate := nvi.BuildList(nvi.Registration{
		CustodianURA: plataanURA, BSN: anna.BSN, ClientID: "PLATAAN-EHR-legacy",
	}, nvi.CategoryCondition)
	postRawNVIList(t, h, duplicate)

	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()
	postAndRead(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share")
	status, body := getBody(t, client, srv, "/demo/ehr/patients/"+anna.Key)

	require.Equal(t, http.StatusOK, status)
	require.NotContains(t, body, "data category this build cannot name",
		"a duplicate of a recognized category is not a record whose category cannot be named")
	require.Contains(t, body, nvi.CategoryCondition)
}

// postRawNVIList writes a List through the Knooppunt without going via
// nvi.Register, so a test can store one the vocabulary does not recognize.
func postRawNVIList(t *testing.T, h harness.Details, list fhir.List) {
	t.Helper()
	body, err := json.Marshal(list)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		h.KnooppuntInternalBaseURL.JoinPath("nvi", "List").String(), bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/fhir+json")
	req.Header.Set("X-Tenant-ID", "http://fhir.nl/fhir/NamingSystem/ura|"+plataanURA)

	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Contains(t, []int{http.StatusOK, http.StatusCreated}, res.StatusCode,
		"precondition: the legacy List was stored")
}

// Sharing must not disturb the source registration E4 depends on.
func TestAcceptance_ShareLeavesZonnebloemIntact(t *testing.T) {
	h, srv, client := acceptanceServer(t)
	anna, ok := pool.PatientByKey("anna")
	require.True(t, ok)

	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()
	status, body := postAndRead(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/share")

	// Without this the test passes when the share never ran: a 409 or a 500
	// leaves Zonnebloem exactly as seeded, which is what it asserts below.
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Localization records published")

	zonnebloem := filterByCustodian(rawNVILists(t, h, anna.BSN), h.SunflowerURA)
	require.Equal(t, anna.ZonnebloemCategories(), nvi.CategoriesOf(zonnebloem))
}

// AC5: a second session cannot claim a patient a run already holds.
func TestAcceptance_ASecondSessionCannotOpenALockedPatient(t *testing.T) {
	_, srv, client := acceptanceServer(t)
	anna, ok := pool.PatientByKey("anna")
	require.True(t, ok)
	other := signInViaDezi(t, srv)

	postForm(t, client, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil).Body.Close()
	res := postForm(t, other, srv, "/demo/ehr/patients/"+anna.Key+"/open", nil)
	defer res.Body.Close()

	require.Equal(t, "/demo/ehr?notice=patient-busy", res.Header.Get("Location"))
}
