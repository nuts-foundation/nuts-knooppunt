package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMitzSubscribe_PostsAConformantSubscription(t *testing.T) {
	var gotPath, gotTenant, gotContentType string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotTenant = r.URL.Path, r.Header.Get("X-Tenant-ID")
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	base, err := url.Parse(server.URL)
	require.NoError(t, err)

	require.NoError(t, mitzSubscribeFunc(base, "00000010", "Z3")(t.Context(), "999900006"))

	require.Equal(t, "/mitz/Subscription", gotPath)
	require.Equal(t, "http://fhir.nl/fhir/NamingSystem/ura|00000010", gotTenant)
	require.Equal(t, "application/fhir+json", gotContentType)
	require.Equal(t, "Subscription", body["resourceType"])
	require.Equal(t, "requested", body["status"])
	require.Equal(t, "OTV", body["reason"])
	require.Equal(t,
		"Consent?_query=otv&patientid=999900006&providerid=00000010&providertype=Z3",
		body["criteria"])
}

func TestMitzSubscribe_NonCreatedIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)

	require.Error(t, mitzSubscribeFunc(base, "00000010", "Z3")(t.Context(), "999900006"))
}

func TestMitzSubscribed_ReadsTheMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/abonnementen/fhir/Subscription", r.URL.Path)
		require.Equal(t, "00000010", r.URL.Query().Get("providerid"))
		require.Equal(t, "999900006", r.URL.Query().Get("patientid"))
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","total":1,"entry":[{"resource":{"resourceType":"Subscription"}}]}`))
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)

	subscribed, err := mitzSubscribedFunc(base, "00000010")(t.Context(), "999900006")

	require.NoError(t, err)
	require.True(t, subscribed)
}

func TestMitzSubscribed_EmptyBundleMeansNotSubscribed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","total":0}`))
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)

	subscribed, err := mitzSubscribedFunc(base, "00000010")(t.Context(), "999900006")

	require.NoError(t, err)
	require.False(t, subscribed)
}

// Reconciliation is a demo affordance against the mock. With no mock configured
// the answer is unknown, which must not be reported as "not subscribed".
func TestMitzSubscribed_NilBaseIsUnconfigured(t *testing.T) {
	require.Nil(t, mitzSubscribedFunc(nil, "00000010"))
}

func TestPatientShareStatus_SubscriptionUnknownWhenTheQueryFails(t *testing.T) {
	cfg := Config{
		nviCategories:  func(context.Context, string) ([]string, error) { return []string{"Condition"}, nil },
		mitzSubscribed: func(context.Context, string) (bool, error) { return false, context.DeadlineExceeded },
	}

	status := cfg.patientShareStatus(t.Context(), "999900006")

	require.True(t, status.Shared)
	require.True(t, status.SubscriptionUnknown)
	require.False(t, status.Subscribed)
}
