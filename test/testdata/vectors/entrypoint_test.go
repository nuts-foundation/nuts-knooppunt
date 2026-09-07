package vectors

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestClearMitzSubscriptions_NilURLIsANoOp(t *testing.T) {
	if err := clearMitzSubscriptions(context.Background(), nil); err != nil {
		t.Fatalf("no mock configured means nothing to clear, got %v", err)
	}
}

func TestClearMitzSubscriptions_CallsDelete(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	if err := clearMitzSubscriptions(context.Background(), base); err != nil {
		t.Fatalf("clearMitzSubscriptions: %v", err)
	}
	if method != http.MethodDelete || path != "/abonnementen/fhir/Subscription" {
		t.Fatalf("called %s %s", method, path)
	}
}

// A configured but unreachable mock must not fail the whole reset, and must not
// report a clean slate either.
func TestClearMitzSubscriptions_UnreachableIsAPartialReset(t *testing.T) {
	base, err := url.Parse("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}

	if err := clearMitzSubscriptions(context.Background(), base); !errors.Is(err, ErrPartialReset) {
		t.Fatalf("got %v, want ErrPartialReset", err)
	}
}

func TestClearMitzSubscriptions_UnexpectedStatusIsAPartialReset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)

	if err := clearMitzSubscriptions(context.Background(), base); !errors.Is(err, ErrPartialReset) {
		t.Fatalf("got %v, want ErrPartialReset", err)
	}
}
