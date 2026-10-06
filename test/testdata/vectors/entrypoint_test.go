package vectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

// mitzMock serves handler as the mock Mitz and returns its base URL.
func mitzMock(t *testing.T, handler http.HandlerFunc) *url.URL {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return base
}

// No mock configured is not a clean slate. The sandbox subscribes through the
// Knooppunt, which reaches Mitz whether or not MITZMOCK_URL was ever set, so a
// subscription can exist that this cannot remove, and nil here made the reset
// notice claim a restored consent state nothing had touched.
func TestClearMitzSubscriptions_NilURLIsAPartialReset(t *testing.T) {
	err := clearMitzSubscriptions(context.Background(), nil, url.Values{})
	if !errors.Is(err, ErrPartialReset) {
		t.Fatalf("got %v, want ErrPartialReset", err)
	}
}

func TestClearMitzSubscriptions_CallsDelete(t *testing.T) {
	var method, path, query string
	base := mitzMock(t, func(w http.ResponseWriter, r *http.Request) {
		method, path, query = r.Method, r.URL.Path, r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	})

	if err := clearMitzSubscriptions(context.Background(), base, url.Values{}); err != nil {
		t.Fatalf("clearMitzSubscriptions: %v", err)
	}
	if method != http.MethodDelete || path != "/abonnementen/fhir/Subscription" {
		t.Fatalf("called %s %s", method, path)
	}
	if query != "" {
		t.Fatalf("an empty scope must clear everything, got query %q", query)
	}
}

// The scope has to reach the mock as query parameters. A recycle that sent none
// would clear every subscription, cancelling a concurrent demo's consent
// subscription while reporting that it restored one patient.
func TestClearMitzSubscriptions_ScopeIsSentAsQueryParameters(t *testing.T) {
	var query string
	base := mitzMock(t, func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	})

	scope := url.Values{"providerid": {"00000010"}, "patientid": {"999900006"}}
	if err := clearMitzSubscriptions(context.Background(), base, scope); err != nil {
		t.Fatalf("clearMitzSubscriptions: %v", err)
	}
	if query != scope.Encode() {
		t.Fatalf("query = %q, want %q", query, scope.Encode())
	}
}

// A configured but unreachable mock must not fail the whole reset, and must not
// report a clean slate either.
func TestClearMitzSubscriptions_UnreachableIsAPartialReset(t *testing.T) {
	base, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}

	if err := clearMitzSubscriptions(context.Background(), base, url.Values{}); !errors.Is(err, ErrPartialReset) {
		t.Fatalf("got %v, want ErrPartialReset", err)
	}
}

func TestClearMitzSubscriptions_UnexpectedStatusIsAPartialReset(t *testing.T) {
	base := mitzMock(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if err := clearMitzSubscriptions(context.Background(), base, url.Values{}); !errors.Is(err, ErrPartialReset) {
		t.Fatalf("got %v, want ErrPartialReset", err)
	}
}

// searchBundle is a searchset holding one resource per id, of the given type.
func searchBundle(t *testing.T, resourceType string, ids ...string) []byte {
	t.Helper()
	entries := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		entries = append(entries, map[string]any{"resource": map[string]any{"resourceType": resourceType, "id": id}})
	}
	body, err := json.Marshal(map[string]any{"resourceType": "Bundle", "type": "searchset", "entry": entries})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// taggedSearch is one search the fake store received.
type taggedSearch struct {
	params       url.Values
	cacheControl string
}

// fakeTaggedStore answers AllergyIntolerance searches with pages in order (the
// last one repeats) and records searches and deletes.
func fakeTaggedStore(t *testing.T, pages ...[]byte) (fhirclient.Client, *[]taggedSearch, *[]string) {
	t.Helper()
	var searches []taggedSearch
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/AllergyIntolerance/_search":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse search: %v", err)
			}
			searches = append(searches, taggedSearch{params: r.PostForm, cacheControl: r.Header.Get("Cache-Control")})
			page := pages[0]
			if len(pages) > 1 {
				pages = pages[1:]
			}
			w.Header().Set("Content-Type", "application/fhir+json")
			_, _ = w.Write(page)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/AllergyIntolerance/"):
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, "/AllergyIntolerance/"))
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return fhirclient.New(base, server.Client(), nil), &searches, &deleted
}

// Recycle deletes what was added for the patient, by tag and patient together:
// the tag alone would take another patient's additions, the patient alone the
// seeded fixtures. A search that comes back empty is what ends it.
func TestDeleteUserCreated_DeletesTheTaggedRecordsOfOnePatient(t *testing.T) {
	client, searches, deleted := fakeTaggedStore(t,
		searchBundle(t, "AllergyIntolerance", "a1", "a2"),
		searchBundle(t, "AllergyIntolerance"))

	err := deleteUserCreated(context.Background(), client, "Patient/pool-anna-zonnebloem-patient")

	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(*deleted, ",") != "a1,a2" {
		t.Fatalf("deleted %v, want [a1 a2]", *deleted)
	}
	if len(*searches) != 2 {
		t.Fatalf("searched %d times, want 2: the second confirms nothing tagged is left", len(*searches))
	}
	for _, q := range *searches {
		if got := q.params.Get("patient"); got != "Patient/pool-anna-zonnebloem-patient" {
			t.Errorf("patient = %q", got)
		}
		if got := q.params.Get("_tag"); got != pool.UserCreatedTagSystem+"|"+pool.UserCreatedTagCode {
			t.Errorf("_tag = %q", got)
		}
	}
}

// HAPI answers an identical search from its cache for a minute by default, and
// every recycle of a patient sends the same one. Without this, recycling a
// patient twice within the minute finds what the first recycle saw, reports
// "restored" and leaves a record added in between.
func TestDeleteUserCreated_AsksForFreshResults(t *testing.T) {
	client, searches, _ := fakeTaggedStore(t,
		searchBundle(t, "AllergyIntolerance", "a1"),
		searchBundle(t, "AllergyIntolerance"))

	if err := deleteUserCreated(context.Background(), client, "Patient/p"); err != nil {
		t.Fatal(err)
	}
	for i, q := range *searches {
		if q.cacheControl != "no-cache" {
			t.Errorf("search %d Cache-Control = %q, want no-cache", i+1, q.cacheControl)
		}
	}
}

// A global reset clears each mutable tenant by searching every type with the
// same parameters, so two resets within HAPI's one-minute search cache would see
// the store as the first did and leave records added in between.
func TestSearchResourceIDs_AsksForFreshResults(t *testing.T) {
	page := searchBundle(t, "AllergyIntolerance")
	var cacheControl string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cacheControl = r.Header.Get("Cache-Control")
		w.Header().Set("Content-Type", "application/fhir+json")
		_, _ = w.Write(page)
	}))
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := searchResourceIDs(context.Background(), fhirclient.New(base, server.Client(), nil), "AllergyIntolerance"); err != nil {
		t.Fatal(err)
	}
	if cacheControl != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", cacheControl)
	}
}

// A server may add an OperationOutcome to a search result. It is not a record,
// and deleting AllergyIntolerance/<its id> would be deleting something else.
func TestDeleteUserCreated_SkipsAnOperationOutcomeInTheResult(t *testing.T) {
	withOutcome := []byte(`{"resourceType":"Bundle","type":"searchset","entry":[` +
		`{"resource":{"resourceType":"OperationOutcome","id":"warning"}},` +
		`{"resource":{"resourceType":"AllergyIntolerance","id":"a1"}}]}`)
	client, _, deleted := fakeTaggedStore(t, withOutcome, searchBundle(t, "AllergyIntolerance"))

	if err := deleteUserCreated(context.Background(), client, "Patient/p"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(*deleted, ",") != "a1" {
		t.Fatalf("deleted %v, want only [a1]", *deleted)
	}
}

// The limit is on rounds that still find tagged records. When the last allowed
// round deletes the last of them, the search after it finds nothing and recycle
// goes on to restore the patient.
func TestDeleteUserCreated_TheLastAllowedRoundCanTakeTheLastRecords(t *testing.T) {
	var pages [][]byte
	for i := range maxUserCreatedRounds {
		pages = append(pages, searchBundle(t, "AllergyIntolerance", fmt.Sprintf("a%d", i+1)))
	}
	pages = append(pages, searchBundle(t, "AllergyIntolerance"))
	client, searches, deleted := fakeTaggedStore(t, pages...)

	err := deleteUserCreated(context.Background(), client, "Patient/p")

	if err != nil {
		t.Fatalf("err = %v, want nil: every tagged record was deleted", err)
	}
	if len(*deleted) != maxUserCreatedRounds {
		t.Fatalf("deleted %d records, want %d", len(*deleted), maxUserCreatedRounds)
	}
	if len(*searches) != maxUserCreatedRounds+1 {
		t.Fatalf("searched %d times, want %d: the last confirms nothing is left", len(*searches), maxUserCreatedRounds+1)
	}
}

// A store that answers with the same records however often they are deleted is
// not deleting them. Looping on would hang the recycle button.
func TestDeleteUserCreated_GivesUpOnAStoreThatDoesNotDelete(t *testing.T) {
	client, _, _ := fakeTaggedStore(t, searchBundle(t, "AllergyIntolerance", "a1"))

	err := deleteUserCreated(context.Background(), client, "Patient/p")

	if err == nil || !strings.Contains(err.Error(), "still there") {
		t.Fatalf("err = %v, want one saying the records are still there", err)
	}
}
