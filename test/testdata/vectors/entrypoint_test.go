package vectors

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// No mock configured is not a clean slate. The sandbox subscribes through the
// Knooppunt, which reaches Mitz whether or not MITZMOCK_URL was ever set, so a
// subscription can exist that this has no way to remove. Returning nil here made
// the reset notice claim the seeded fixtures were restored over a consent state
// nothing had touched.
func TestClearMitzSubscriptions_NilURLIsAPartialReset(t *testing.T) {
	err := clearMitzSubscriptions(context.Background(), nil, url.Values{})
	if !errors.Is(err, ErrPartialReset) {
		t.Fatalf("got %v, want ErrPartialReset", err)
	}
}

func TestClearMitzSubscriptions_CallsDelete(t *testing.T) {
	var method, path, query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, query = r.Method, r.URL.Path, r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)

	if err := clearMitzSubscriptions(context.Background(), base, url.Values{}); !errors.Is(err, ErrPartialReset) {
		t.Fatalf("got %v, want ErrPartialReset", err)
	}
}
