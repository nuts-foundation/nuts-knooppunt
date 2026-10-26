package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mitzServer serves handler as the Knooppunt's Mitz endpoint or as the mock, and
// returns its base URL.
func mitzServer(t *testing.T, handler http.HandlerFunc) *url.URL {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	require.NoError(t, err)
	return base
}

func TestMitzSubscribe_PostsAConformantSubscription(t *testing.T) {
	var gotPath, gotContentType string
	var body map[string]any
	base := mitzServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusCreated)
	})

	require.NoError(t, mitzSubscribeFunc(base, "00000010", "Z3")(t.Context(), "999900006"))

	require.Equal(t, "/mitz/Subscription", gotPath)
	require.Equal(t, "application/fhir+json", gotContentType)
	require.Equal(t, "Subscription", body["resourceType"])
	require.Equal(t, "requested", body["status"])
	require.Equal(t, "OTV", body["reason"])
	require.Equal(t,
		"Consent?_query=otv&patientid=999900006&providerid=00000010&providertype=Z3",
		body["criteria"])
}

func TestMitzSubscribe_NonCreatedIsAnError(t *testing.T) {
	base := mitzServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	require.Error(t, mitzSubscribeFunc(base, "00000010", "Z3")(t.Context(), "999900006"))
}

func TestMitzSubscribed_ReadsTheMock(t *testing.T) {
	// assert, not require: this runs on the server's goroutine, where FailNow
	// calls runtime.Goexit and abandons the response mid-write, leaving the
	// client blocked until its own timeout instead of failing here.
	base := mitzServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/abonnementen/fhir/Subscription", r.URL.Path)
		assert.Equal(t, "00000010", r.URL.Query().Get("providerid"))
		assert.Equal(t, "999900006", r.URL.Query().Get("patientid"))
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","total":1,"entry":[{"resource":{"resourceType":"Subscription"}}]}`))
	})

	subscribed, err := mitzSubscribedFunc(base, "00000010")(t.Context(), "999900006")

	require.NoError(t, err)
	require.True(t, subscribed)
}

func TestMitzSubscribed_EmptyBundleMeansNotSubscribed(t *testing.T) {
	base := mitzServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"resourceType":"Bundle","type":"searchset","total":0}`))
	})

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
		nviLookup: func(context.Context, string) (nviRecords, error) {
			return nviRecords{Count: 1, Categories: []string{"Condition"}}, nil
		},
		mitzSubscribed: func(context.Context, string) (bool, error) { return false, context.DeadlineExceeded },
	}

	status := cfg.patientShareStatus(t.Context(), "999900006")

	require.True(t, status.Shared)
	require.True(t, status.SubscriptionUnknown)
	require.False(t, status.Subscribed)
}

const consentChangeID = "4bf92f35-77b3-4da6-a3ce-929d0e0e4736"

var consentChangedAt = time.Date(2026, 10, 12, 14, 4, 55, 100_000_000, time.UTC)

func TestMitzSetConsent_PutsTheDecisionWithItsChangeIDAndReturnsWhenItChanged(t *testing.T) {
	base := mitzServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/mock/consent/999900006", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		assert.JSONEq(t, `{"decision":"Deny","changeId":"`+consentChangeID+`"}`, string(body))
		_, _ = w.Write([]byte(`{"bsn":"999900006","decision":"Deny","changeId":"` + consentChangeID + `","changedAt":"2026-10-12T14:04:55.1Z","notifications":[]}`))
	})
	// The act is an event of the run: the call goes through the capture transport.
	var events []stepEvent
	ctx := withEventCapture(t.Context(), func(event stepEvent) { events = append(events, event) }, true)

	changedAt, err := mitzSetConsentFunc(base)(ctx, "999900006", "Deny", consentChangeID)

	require.NoError(t, err)
	require.True(t, changedAt.Equal(consentChangedAt), "got %s", changedAt)
	require.Len(t, events, 1)
	require.Equal(t, "consent", events[0].GF)
	require.Equal(t, "consent-change", events[0].Purpose)
	require.Equal(t, "ok", events[0].Outcome)
}

func TestMitzSetConsent_ANon200AndAnUnparseableAnswerAreErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		answer func(http.ResponseWriter)
		reason string
	}{
		"refused":     {func(w http.ResponseWriter) { http.Error(w, "decision must be Permit or Deny", http.StatusBadRequest) }, "consent change returned 400"},
		"unparseable": {func(w http.ResponseWriter) { _, _ = w.Write([]byte(`not json`)) }, "parse consent change answer"},
	} {
		t.Run(name, func(t *testing.T) {
			base := mitzServer(t, func(w http.ResponseWriter, _ *http.Request) { tc.answer(w) })
			_, err := mitzSetConsentFunc(base)(t.Context(), "999900006", "Deny", consentChangeID)
			require.ErrorContains(t, err, tc.reason)
		})
	}
}

func TestMitzConsentState_ReadsWhatTheMockAnswersNow(t *testing.T) {
	base := mitzServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/mock/consent/999900006", r.URL.Path)
		_, _ = w.Write([]byte(`{"bsn":"999900006","decision":"Deny","overridden":true,"changedAt":"2026-10-12T14:04:55.1Z"}`))
	})
	// A status read, not an act of the demo: it produces no event.
	var events []stepEvent
	ctx := withEventCapture(t.Context(), func(event stepEvent) { events = append(events, event) }, true)

	state, err := mitzConsentStateFunc(base)(ctx, "999900006")

	require.NoError(t, err)
	require.Equal(t, "Deny", state.Decision)
	require.True(t, state.Overridden)
	require.True(t, state.ChangedAt.Equal(consentChangedAt))
	require.Empty(t, events)
}

func TestMitzConsentState_TheConfiguredDefaultHasNoChangeTime(t *testing.T) {
	base := mitzServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"bsn":"999900006","decision":"Permit","overridden":false}`))
	})

	state, err := mitzConsentStateFunc(base)(t.Context(), "999900006")

	require.NoError(t, err)
	require.Equal(t, mitzConsentStatus{Decision: "Permit"}, state)
}

func TestMitzConsentState_RefusesAnAnswerItCannotUse(t *testing.T) {
	for name, tc := range map[string]struct {
		answer func(http.ResponseWriter)
		reason string
	}{
		"unavailable":      {func(w http.ResponseWriter) { http.Error(w, "down", http.StatusServiceUnavailable) }, "consent state returned 503"},
		"unparseable":      {func(w http.ResponseWriter) { _, _ = w.Write([]byte(`not json`)) }, "parse consent state"},
		"another decision": {func(w http.ResponseWriter) { _, _ = w.Write([]byte(`{"decision":"Indeterminate"}`)) }, `consent state carried decision "Indeterminate"`},
	} {
		t.Run(name, func(t *testing.T) {
			base := mitzServer(t, func(w http.ResponseWriter, _ *http.Request) { tc.answer(w) })
			_, err := mitzConsentStateFunc(base)(t.Context(), "999900006")
			require.ErrorContains(t, err, tc.reason)
		})
	}
}

func TestMitzConsentClients_NilBaseIsUnconfigured(t *testing.T) {
	require.Nil(t, mitzSetConsentFunc(nil))
	require.Nil(t, mitzConsentStateFunc(nil))
}
