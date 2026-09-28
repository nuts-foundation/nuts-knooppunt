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

	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// fakeStore is the store the handlers see in these tests. asked records every
// client id the handlers looked up.
type fakeStore struct {
	clients []client
	records map[string]record
	fail    error
	addErr  error
	added   []fhir.AllergyIntolerance
	asked   []string
}

func (f *fakeStore) Clients(context.Context) ([]client, error) {
	if f.fail != nil {
		return nil, f.fail
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

func annaStore() *fakeStore {
	return &fakeStore{
		clients: []client{{ID: "anna", Name: "Jansen, Anna", Initials: "AJ", BirthDate: "1944-03-12", BSN: "999900006"}},
		records: map[string]record{"anna": {Allergies: []entry{{Title: "Penicilline", Detail: "Unconfirmed · 2019"}}}},
	}
}

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
	require.Contains(t, body, "Jansen, Anna")
	require.Contains(t, body, `href="/clients/anna"`)
	require.Contains(t, body, "Test environment · synthetic data")
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
	st.records["anna"] = record{Allergies: []entry{
		{Title: "Pinda", Detail: "Confirmed · 2026-09-28 · DEMO-X", AddedInDemo: true},
		{Title: "Penicilline", Detail: "Unconfirmed · 2019"},
	}}

	status, body := get(t, serve(t, st).URL+"/clients/anna")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Penicilline")
	require.Contains(t, body, `<span class="added-chip">added in this demo</span>`)
	require.Contains(t, body, "Established after reaction. DEMO-FIXED-MARKER-07</textarea>")
	require.Contains(t, body, `action="/clients/anna/allergies"`)
}

func TestClient_UnknownClientIs404(t *testing.T) {
	status, _ := get(t, serve(t, annaStore()).URL+"/clients/nobody")

	require.Equal(t, http.StatusNotFound, status)
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
	st.records["anna"] = record{Allergies: []entry{{Title: "Pinda", Detail: "<script>alert(1)</script>"}}}

	_, body := get(t, serve(t, st).URL+"/clients/anna")

	require.Contains(t, body, "&lt;script&gt;alert(1)&lt;/script&gt;")
	require.NotContains(t, body, "<script>alert(1)")
}

func TestAddAllergy_SavesAndRedirectsToTheRecord(t *testing.T) {
	st := annaStore()

	res := post(t, serve(t, st).URL+"/clients/anna/allergies", validPost(), "same-origin")

	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/clients/anna?saved=marker", res.Header.Get("Location"))
	require.Len(t, st.added, 1)
	require.Equal(t, "Patient/anna", *st.added[0].Patient.Reference)
	require.Equal(t, "2026-09-28", *st.added[0].RecordedDate)
	require.True(t, userCreated(st.added[0].Meta))
}

func TestAddAllergy_ConfirmationSaysWhetherPlataanWillHighlightIt(t *testing.T) {
	st := annaStore()
	srv := serve(t, st)
	form := validPost()
	form.Set("note", "Na pinda.")

	res := post(t, srv.URL+"/clients/anna/allergies", form, "")
	require.Equal(t, "/clients/anna?saved=plain", res.Header.Get("Location"))

	_, plain := get(t, srv.URL+"/clients/anna?saved=plain")
	_, marker := get(t, srv.URL+"/clients/anna?saved=marker")
	require.Contains(t, plain, "carries no <code>DEMO-</code> word")
	require.Contains(t, marker, "retrieve the patient there to see it arrive through the chain")
}

func TestAddAllergy_AnInvalidFormRendersAgainWithTheReason(t *testing.T) {
	st := annaStore()
	form := validPost()
	form.Set("substance", "294505008")

	res := post(t, serve(t, st).URL+"/clients/anna/allergies", form, "")
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

		res := post(t, serve(t, st).URL+"/clients/anna/allergies", validPost(), site)

		require.Equalf(t, http.StatusForbidden, res.StatusCode, "Sec-Fetch-Site: %s", site)
		require.Emptyf(t, st.added, "Sec-Fetch-Site: %s", site)
	}
}

func TestAddAllergy_UnknownClientIs404(t *testing.T) {
	st := annaStore()

	res := post(t, serve(t, st).URL+"/clients/nobody/allergies", validPost(), "")

	require.Equal(t, http.StatusNotFound, res.StatusCode)
	require.Empty(t, st.added)
}

func TestAddAllergy_AStoreThatRefusesIs502(t *testing.T) {
	st := annaStore()
	st.addErr = errors.New("create allergy: FHIR request failed (status=422)")

	res := post(t, serve(t, st).URL+"/clients/anna/allergies", validPost(), "")

	require.Equal(t, http.StatusBadGateway, res.StatusCode)
	require.Empty(t, res.Header.Get("Location"))
}
