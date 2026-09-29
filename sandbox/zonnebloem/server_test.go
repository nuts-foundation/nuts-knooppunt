package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// fakeStore is the store the handlers see in these tests. asked records every
// client id the handlers looked up.
type fakeStore struct {
	clients    []client
	clientsErr error
	records    map[string]record
	fail       error
	addErr     error
	added      []fhir.AllergyIntolerance
	asked      []string

	allergies     map[string]fhir.AllergyIntolerance // what Allergy reads, by id
	allergyErr    error
	readAllergies []string // every allergy id the handlers read
	deleteErr     error
	deleted       []string // every allergy id the handlers deleted
}

func (f *fakeStore) Clients(context.Context) ([]client, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	if f.clientsErr != nil {
		return nil, f.clientsErr
	}
	return f.clients, nil
}

func (f *fakeStore) Client(_ context.Context, id string) (client, error) {
	f.asked = append(f.asked, id)
	if f.fail != nil {
		return client{}, f.fail
	}
	for _, c := range f.clients {
		if c.ID == id {
			return c, nil
		}
	}
	return client{}, errNotFound
}

func (f *fakeStore) Record(_ context.Context, id string) (record, error) {
	if f.fail != nil {
		return record{}, f.fail
	}
	return f.records[id], nil
}

func (f *fakeStore) AddAllergy(_ context.Context, a fhir.AllergyIntolerance) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, a)
	return nil
}

func (f *fakeStore) Allergy(_ context.Context, id string) (fhir.AllergyIntolerance, error) {
	f.readAllergies = append(f.readAllergies, id)
	if f.allergyErr != nil {
		return fhir.AllergyIntolerance{}, f.allergyErr
	}
	allergy, ok := f.allergies[id]
	if !ok {
		return fhir.AllergyIntolerance{}, errNotFound
	}
	return allergy, nil
}

func (f *fakeStore) DeleteAllergy(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func annaStore() *fakeStore {
	return &fakeStore{
		clients: []client{{ID: "pool-anna-zonnebloem-patient", Name: "Anna Jansen", Initials: "AJ", BirthDate: "1944-03-12", BSN: "999900006"}},
		records: map[string]record{"pool-anna-zonnebloem-patient": {Allergies: []entry{{ID: "seeded-1", Title: "Penicilline", Meta: "Recorded 2019", Status: "unconfirmed", StatusLabel: "Suspected"}}}},
	}
}

// bramID is a demo-pool client annaStore does not hold: the pool check lets it
// through, so only the store can report it unknown.
const bramID = "pool-pool-02-zonnebloem-patient"

func serve(t *testing.T, st store) *httptest.Server {
	t.Helper()
	fixedNow := func() time.Time { return time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC) }
	server := httptest.NewServer(newMux(st, fixedNow, func() string { return "DEMO-FIXED-MARKER-07" }))
	t.Cleanup(server.Close)
	return server
}

// noRedirect keeps a 303 as the response instead of following it.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	res, err := noRedirect.Get(url)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, string(body)
}

func post(t *testing.T, target string, form url.Values, secFetchSite string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if secFetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", secFetchSite)
	}
	res, err := noRedirect.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func validPost() url.Values {
	return url.Values{"substance": {"762952008"}, "status": {"confirmed"}, "note": {"Na pinda. DEMO-BLUE-OTTER-12"}}
}

func TestHealthz(t *testing.T) {
	status, body := get(t, serve(t, annaStore()).URL+"/healthz")

	require.Equal(t, http.StatusOK, status)
	require.JSONEq(t, `{"ok":true}`, body)
}

func TestStatic_ServesTheStylesheet(t *testing.T) {
	res, err := http.Get(serve(t, annaStore()).URL + "/static/css/zonnebloem.css")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Contains(t, res.Header.Get("Content-Type"), "text/css")
}

func TestClients_ListsTheStoresClients(t *testing.T) {
	status, body := get(t, serve(t, annaStore()).URL+"/")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Anna Jansen")
	require.Contains(t, body, `href="/clients/pool-anna-zonnebloem-patient"`)
	require.Contains(t, body, "Test environment with synthetic data")
	require.Contains(t, body, "Zorgcentrum De Zonnebloem")
}

func TestClients_AFailingStoreIs502(t *testing.T) {
	st := annaStore()
	st.fail = errors.New("connection refused")

	status, _ := get(t, serve(t, st).URL+"/")

	require.Equal(t, http.StatusBadGateway, status)
}

func TestClient_ShowsTheRecordAndSuggestsAMarker(t *testing.T) {
	st := annaStore()
	st.records["pool-anna-zonnebloem-patient"] = record{Allergies: []entry{
		{ID: "added-1", Title: "Pinda", Meta: "Recorded 28 September 2026", Note: "DEMO-X", Status: "confirmed", StatusLabel: "Confirmed", AddedInDemo: true},
		{ID: "seeded-1", Title: "Penicilline", Meta: "Recorded 2019", Status: "unconfirmed", StatusLabel: "Suspected"},
	}}

	status, body := get(t, serve(t, st).URL+"/clients/pool-anna-zonnebloem-patient")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Penicilline")
	require.Contains(t, body, `<span class="tag">Added in this demo</span>`)
	require.Contains(t, body, "Established after reaction. DEMO-FIXED-MARKER-07</textarea>")
	require.Contains(t, body, `action="/clients/pool-anna-zonnebloem-patient/allergies"`)
}

func TestClient_UnknownClientIs404(t *testing.T) {
	st := annaStore()

	status, _ := get(t, serve(t, st).URL+"/clients/"+bramID)

	require.Equal(t, http.StatusNotFound, status)
	require.Equal(t, []string{bramID}, st.asked)
}

// An id that is not a FHIR id never reaches the store's URL, where ".." would
// walk out of the Patient path.
func TestClient_AnIDThatIsNotAFHIRIDIs404WithoutAskingTheStore(t *testing.T) {
	st := annaStore()
	srv := serve(t, st)

	// The mux hands "%2e%2e" over as ".." and "a%2Fb" as "a/b"; only a literal
	// ".." is redirected before any handler runs.
	for _, id := range []string{"%2e%2e", "a%2Fb", "anna%20jansen", strings.Repeat("a", 65)} {
		status, _ := get(t, srv.URL+"/clients/"+id)
		require.Equalf(t, http.StatusNotFound, status, "id %q", id)
	}
	require.Empty(t, st.asked)
}

func TestClient_EscapesWhatTheStoreHolds(t *testing.T) {
	st := annaStore()
	st.records["pool-anna-zonnebloem-patient"] = record{Allergies: []entry{{Title: "Pinda", Note: "<script>alert(1)</script>"}}}

	_, body := get(t, serve(t, st).URL+"/clients/pool-anna-zonnebloem-patient")

	require.Contains(t, body, "&lt;script&gt;alert(1)&lt;/script&gt;")
	require.NotContains(t, body, "<script>alert(1)")
}

func TestAddAllergy_SavesAndRedirectsToTheRecord(t *testing.T) {
	st := annaStore()

	res := post(t, serve(t, st).URL+"/clients/pool-anna-zonnebloem-patient/allergies", validPost(), "same-origin")

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/clients/pool-anna-zonnebloem-patient?saved=marker", res.Header.Get("Location"))
	require.Len(t, st.added, 1)
	require.Equal(t, "Patient/pool-anna-zonnebloem-patient", *st.added[0].Patient.Reference)
	require.Equal(t, "2026-09-28", *st.added[0].RecordedDate)
	require.True(t, userCreated(st.added[0].Meta))
}

func TestAddAllergy_ConfirmationSaysWhetherPlataanWillHighlightIt(t *testing.T) {
	st := annaStore()
	srv := serve(t, st)
	form := validPost()
	form.Set("note", "Na pinda.")

	res := post(t, srv.URL+"/clients/pool-anna-zonnebloem-patient/allergies", form, "")
	require.Equal(t, "/clients/pool-anna-zonnebloem-patient?saved=plain", res.Header.Get("Location"))

	_, plain := get(t, srv.URL+"/clients/pool-anna-zonnebloem-patient?saved=plain")
	_, marker := get(t, srv.URL+"/clients/pool-anna-zonnebloem-patient?saved=marker")
	require.Contains(t, plain, "The note has no DEMO- word, so Plataan shows the entry without highlighting it.")
	require.Contains(t, marker, "retrieve Anna Jansen there to see it arrive through the chain")
}

func TestAddAllergy_AnInvalidFormRendersAgainWithTheReason(t *testing.T) {
	st := annaStore()
	form := validPost()
	form.Set("substance", "294505008")

	res := post(t, serve(t, st).URL+"/clients/pool-anna-zonnebloem-patient/allergies", form, "")
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	require.Contains(t, string(body), "Choose a substance from the list.")
	require.Contains(t, string(body), "Na pinda. DEMO-BLUE-OTTER-12</textarea>", "the entered note survives")
	require.Empty(t, st.added)
}

func TestAddAllergy_RefusesAPostFromAnotherSite(t *testing.T) {
	for _, site := range []string{"cross-site", "same-site"} {
		st := annaStore()

		res := post(t, serve(t, st).URL+"/clients/pool-anna-zonnebloem-patient/allergies", validPost(), site)

		require.Equalf(t, http.StatusForbidden, res.StatusCode, "Sec-Fetch-Site: %s", site)
		require.Emptyf(t, st.added, "Sec-Fetch-Site: %s", site)
	}
}

func TestAddAllergy_UnknownClientIs404(t *testing.T) {
	st := annaStore()

	res := post(t, serve(t, st).URL+"/clients/"+bramID+"/allergies", validPost(), "")

	require.Equal(t, http.StatusNotFound, res.StatusCode)
	require.Equal(t, []string{bramID}, st.asked)
	require.Empty(t, st.added)
}

func TestAddAllergy_AStoreThatRefusesIs502(t *testing.T) {
	st := annaStore()
	st.addErr = errors.New("create allergy: FHIR request failed (status=422)")

	res := post(t, serve(t, st).URL+"/clients/pool-anna-zonnebloem-patient/allergies", validPost(), "")

	require.Equal(t, http.StatusBadGateway, res.StatusCode)
	require.Empty(t, res.Header.Get("Location"))
}

func TestClients_OnlyListsDemoPool(t *testing.T) {
	st := annaStore()
	st.clients = append(st.clients, clientFrom(sunflower.Patients()[0]))

	status, body := get(t, serve(t, st).URL+"/")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Anna Jansen")
	require.NotContains(t, body, "Jan Jansen")
}

func TestClient_OutsideDemoPoolIsUnavailable(t *testing.T) {
	legacy := clientFrom(sunflower.Patients()[0])
	for name, route := range map[string]struct{ method, path string }{
		"record":         {http.MethodGet, ""},
		"add allergy":    {http.MethodPost, "/allergies"},
		"remove allergy": {http.MethodPost, "/allergies/added-1/remove"},
	} {
		t.Run(name, func(t *testing.T) {
			st := annaStore()
			st.clients = append(st.clients, legacy)
			request := httptest.NewRequest(route.method, "/clients/"+legacy.ID+route.path, strings.NewReader(validPost().Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()

			newMux(st, time.Now, func() string { return "DEMO-TEST" }).ServeHTTP(response, request)

			require.Equal(t, http.StatusNotFound, response.Code)
			require.Empty(t, st.added)
			require.Empty(t, st.deleted)
			require.Empty(t, st.asked, "a non-pool client must be rejected before accessing its record")
		})
	}
}

func TestAddAllergy_ChecksOriginWithoutFetchMetadata(t *testing.T) {
	for name, tc := range map[string]struct {
		origin string
		status int
	}{
		"same origin":             {"http://zonnebloem.example:3001", http.StatusSeeOther},
		"TLS terminated upstream": {"https://zonnebloem.example:3001", http.StatusSeeOther},
		"other site":              {"http://another-site.example", http.StatusForbidden},
		"other port":              {"http://zonnebloem.example:3002", http.StatusForbidden},
		"opaque origin":           {"null", http.StatusForbidden},
		"malformed origin":        {"://invalid", http.StatusForbidden},
		"non-browser request":     {"", http.StatusSeeOther},
	} {
		t.Run(name, func(t *testing.T) {
			st := annaStore()
			request := httptest.NewRequest(http.MethodPost,
				"http://zonnebloem.example:3001/clients/pool-anna-zonnebloem-patient/allergies",
				strings.NewReader(validPost().Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			if tc.origin != "" {
				request.Header.Set("Origin", tc.origin)
			}
			response := httptest.NewRecorder()

			newMux(st, time.Now, func() string { return "DEMO-TEST" }).ServeHTTP(response, request)

			require.Equal(t, tc.status, response.Code)
			if tc.status == http.StatusForbidden {
				require.Empty(t, st.added)
				require.Empty(t, st.asked, "a rejected form must not access the store")
			} else {
				require.Len(t, st.added, 1)
			}
		})
	}
}

// removableStore is annaStore holding three allergies to remove: one added for
// Anna in the demo, one of Anna's seeded ones, and one added for Bram.
func removableStore() *fakeStore {
	st := annaStore()
	anna := "Patient/pool-anna-zonnebloem-patient"
	st.allergies = map[string]fhir.AllergyIntolerance{
		"added-1":  {Id: to.Ptr("added-1"), Meta: userCreatedMeta(), Patient: fhir.Reference{Reference: to.Ptr(anna)}},
		"seeded-1": {Id: to.Ptr("seeded-1"), Patient: fhir.Reference{Reference: to.Ptr(anna)}},
		"bram-1":   {Id: to.Ptr("bram-1"), Meta: userCreatedMeta(), Patient: fhir.Reference{Reference: to.Ptr("Patient/" + bramID)}},
	}
	return st
}

func removeURL(srv *httptest.Server, allergyID string) string {
	return srv.URL + "/clients/pool-anna-zonnebloem-patient/allergies/" + allergyID + "/remove"
}

func TestRemoveAllergy_RemovesAnAllergyAddedInTheDemo(t *testing.T) {
	st := removableStore()

	res := post(t, removeURL(serve(t, st), "added-1"), nil, "same-origin")

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/clients/pool-anna-zonnebloem-patient?removed=1", res.Header.Get("Location"))
	require.Equal(t, []string{"added-1"}, st.deleted)
}

// Only what the demo added for this client can go. A seeded allergy stays, so the
// NVI registration for the category keeps pointing at data that exists; one
// added for Bram cannot be removed through Anna's address; an unknown id is
// not there to remove. Each is read first, so the refusal is the check's.
func TestRemoveAllergy_RefusesAnythingButThisClientsDemoAddition(t *testing.T) {
	for _, id := range []string{"seeded-1", "bram-1", "unknown-1"} {
		st := removableStore()

		res := post(t, removeURL(serve(t, st), id), nil, "same-origin")

		require.Equalf(t, http.StatusNotFound, res.StatusCode, "allergy %s", id)
		require.Equalf(t, []string{id}, st.readAllergies, "allergy %s", id)
		require.Emptyf(t, st.deleted, "allergy %s", id)
	}
}

func TestRemoveAllergy_AnIDThatIsNotAFHIRIDIs404WithoutAskingTheStore(t *testing.T) {
	st := removableStore()
	srv := serve(t, st)

	for _, id := range []string{"%2e%2e", "a%2Fb", strings.Repeat("a", 65)} {
		res := post(t, removeURL(srv, id), nil, "same-origin")
		require.Equalf(t, http.StatusNotFound, res.StatusCode, "id %q", id)
	}

	require.Len(t, st.asked, 3, "each request reached the handler, which found the client")
	require.Empty(t, st.readAllergies)
	require.Empty(t, st.deleted)
}

func TestRemoveAllergy_RefusesAPostFromAnotherSite(t *testing.T) {
	st := removableStore()

	res := post(t, removeURL(serve(t, st), "added-1"), nil, "cross-site")

	require.Equal(t, http.StatusForbidden, res.StatusCode)
	require.Empty(t, st.asked)
	require.Empty(t, st.deleted)
}

func TestRemoveAllergy_AFailingStoreIs502(t *testing.T) {
	t.Run("reading the allergy", func(t *testing.T) {
		st := removableStore()
		st.allergyErr = errors.New("read allergy: connection refused")

		res := post(t, removeURL(serve(t, st), "added-1"), nil, "same-origin")

		require.Equal(t, http.StatusBadGateway, res.StatusCode)
		require.Empty(t, st.deleted)
	})
	t.Run("deleting it", func(t *testing.T) {
		st := removableStore()
		st.deleteErr = errors.New("delete allergy: FHIR request failed (status=409)")

		res := post(t, removeURL(serve(t, st), "added-1"), nil, "same-origin")

		require.Equal(t, http.StatusBadGateway, res.StatusCode)
		require.Empty(t, res.Header.Get("Location"))
	})
}

func TestClient_OffersRemovalOnlyForWhatTheDemoAdded(t *testing.T) {
	st := annaStore()
	st.records["pool-anna-zonnebloem-patient"] = record{Allergies: []entry{
		{ID: "added-1", Title: "Pinda", Status: "confirmed", StatusLabel: "Confirmed", AddedInDemo: true},
		{ID: "seeded-1", Title: "Penicilline", Status: "unconfirmed", StatusLabel: "Suspected"},
	}}

	_, body := get(t, serve(t, st).URL+"/clients/pool-anna-zonnebloem-patient")

	require.Contains(t, body, `action="/clients/pool-anna-zonnebloem-patient/allergies/added-1/remove"`)
	require.NotContains(t, body, "/allergies/seeded-1/remove")
}

func TestClient_ConfirmsARemoval(t *testing.T) {
	srv := serve(t, annaStore())

	_, removed := get(t, srv.URL+"/clients/pool-anna-zonnebloem-patient?removed=1")
	_, plain := get(t, srv.URL+"/clients/pool-anna-zonnebloem-patient")

	require.Contains(t, removed, "Removed from Anna Jansen's record.")
	require.NotContains(t, plain, "Removed from")
}

// serve's clock stands on 28 September 2026, when Anna Jansen is 82.
func TestClients_ShowsEachClientsAgeAndBirthDate(t *testing.T) {
	_, body := get(t, serve(t, annaStore()).URL+"/")

	require.Contains(t, body, "82 years, born 12 March 1944")
}

func TestClient_TheBannerShowsAgeBirthDateAndAllergies(t *testing.T) {
	st := annaStore()
	st.records["pool-anna-zonnebloem-patient"] = record{Allergies: []entry{
		{ID: "added-1", Title: "Pinda", Status: "confirmed", StatusLabel: "Confirmed", AddedInDemo: true},
		{ID: "seeded-1", Title: "Penicilline", Status: "unconfirmed", StatusLabel: "Suspected"},
	}}

	_, body := get(t, serve(t, st).URL+"/clients/pool-anna-zonnebloem-patient")

	require.Contains(t, body, "82 years")
	require.Contains(t, body, "born 12 March 1944")
	require.Contains(t, body, "Pinda, Penicilline (suspected)")
}

// Next to the record, the client list offers the other demo clients and marks
// the one open; a client outside the pool is not in it here either.
func TestClient_ListsTheDemoClientsAndMarksThisOne(t *testing.T) {
	st := annaStore()
	st.clients = append(st.clients,
		client{ID: bramID, Name: "Bram de Vries", Initials: "BD"},
		clientFrom(sunflower.Patients()[0]))

	_, body := get(t, serve(t, st).URL+"/clients/pool-anna-zonnebloem-patient")

	require.Contains(t, body, `href="/clients/`+bramID+`"`)
	require.Contains(t, body, `href="/clients/pool-anna-zonnebloem-patient" aria-current="page"`)
	require.NotContains(t, body, `href="/clients/`+bramID+`" aria-current`)
	require.NotContains(t, body, "Jan Jansen")
}

func TestClient_AFailingClientListIs502(t *testing.T) {
	st := annaStore()
	st.clientsErr = errors.New("search Patient: connection refused")

	status, _ := get(t, serve(t, st).URL+"/clients/pool-anna-zonnebloem-patient")

	require.Equal(t, http.StatusBadGateway, status)
}
