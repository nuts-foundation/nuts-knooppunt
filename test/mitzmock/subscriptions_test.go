package mitzmock

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func subscription(providerID, patientID string) fhir.Subscription {
	return fhir.Subscription{
		Status:   fhir.SubscriptionStatusRequested,
		Reason:   "OTV",
		Criteria: "Consent?_query=otv&patientid=" + patientID + "&providerid=" + providerID + "&providertype=Z3",
		Channel:  fhir.SubscriptionChannel{Type: fhir.SubscriptionChannelTypeRestHook},
	}
}

// Sharing the same patient twice must leave one subscription. This mock defines
// the provider/patient pair as the subscription's identity so a demo is
// deterministic; it is not a claim about how the national Mitz behaves.
func TestSubscriptionStore_UpsertIsIdempotentPerProviderAndPatient(t *testing.T) {
	store := newSubscriptionStore()

	first, created := store.upsert(subscription("00000010", "999900006"))
	require.True(t, created)
	require.NotNil(t, first.Id)

	second, created := store.upsert(subscription("00000010", "999900006"))
	require.False(t, created)
	require.Equal(t, *first.Id, *second.Id)
	require.Len(t, store.all(), 1)
}

func TestSubscriptionStore_DifferentPatientsAreDifferentSubscriptions(t *testing.T) {
	store := newSubscriptionStore()
	store.upsert(subscription("00000010", "999900006"))
	store.upsert(subscription("00000010", "999900018"))

	require.Len(t, store.all(), 2)
}

// Both identifiers are required to form the key. Keying on a partial pair would
// merge unrelated subscriptions, and the standalone mock does not run the
// component's required-parameter validation.
func TestSubscriptionStore_PartialCriteriaIsNotKeyed(t *testing.T) {
	for _, criteria := range []string{
		"Consent?_query=otv&providerid=00000010",
		"Consent?_query=otv&patientid=999900006",
		"Consent?_query=otv",
	} {
		store := newSubscriptionStore()
		sub := fhir.Subscription{Status: fhir.SubscriptionStatusRequested, Reason: "OTV", Criteria: criteria}

		store.upsert(sub)
		store.upsert(sub)

		require.Lenf(t, store.all(), 2, "criteria %q cannot be keyed and must not be deduplicated", criteria)
	}
}

func TestSubscriptionStore_FindByProviderAndPatient(t *testing.T) {
	store := newSubscriptionStore()
	store.upsert(subscription("00000010", "999900006"))

	_, found := store.find("00000010", "999900006")
	require.True(t, found)

	_, found = store.find("00000010", "999900018")
	require.False(t, found)
}

func TestSubscriptionStore_Clear(t *testing.T) {
	store := newSubscriptionStore()
	store.upsert(subscription("00000010", "999900006"))

	store.clear()

	require.Empty(t, store.all())
}

func TestStandaloneService_ServesSubscriptions(t *testing.T) {
	service, err := New("127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = service.Stop() })

	body, err := json.Marshal(subscription("00000010", "999900006"))
	require.NoError(t, err)
	endpoint := service.GetURL() + "/abonnementen/fhir/Subscription"

	post, err := http.Post(endpoint, "application/fhir+json", bytes.NewReader(body))
	require.NoError(t, err)
	defer post.Body.Close()
	require.Equal(t, http.StatusCreated, post.StatusCode)
	require.Contains(t, post.Header.Get("Location"), "Subscription/")

	// A second POST for the same pair must not add a second subscription.
	repeat, err := http.Post(endpoint, "application/fhir+json", bytes.NewReader(body))
	require.NoError(t, err)
	require.NoError(t, repeat.Body.Close())

	get, err := http.Get(endpoint)
	require.NoError(t, err)
	defer get.Body.Close()
	require.Equal(t, http.StatusOK, get.StatusCode)
	var bundle fhir.Bundle
	require.NoError(t, json.NewDecoder(get.Body).Decode(&bundle))
	require.Len(t, bundle.Entry, 1)

	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	require.NoError(t, err)
	del, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer del.Body.Close()
	require.Equal(t, http.StatusNoContent, del.StatusCode)
}

// A scoped DELETE removes one pair and leaves the rest. The per-patient recycle
// depends on it: clearing the store instead would cancel a concurrent demo's
// consent subscription while reporting that it restored one patient.
func TestClosedQuestionService_ScopedDeleteRemovesOnlyThatPair(t *testing.T) {
	service := NewClosedQuestionService(t)

	for _, bsn := range []string{"999900006", "999911120"} {
		postSubscription(t, service, "00000010", bsn)
	}
	require.Len(t, service.GetSubscriptions(), 2)

	deleteSubscriptions(t, service, url.Values{"providerid": {"00000010"}, "patientid": {"999900006"}})

	remaining := service.GetSubscriptions()
	require.Len(t, remaining, 1)
	require.Contains(t, remaining[0].Criteria, "patientid=999911120")
}

// Half a pair cannot address a subscription this store keys on both, and the
// destructive reading of that request is to delete everything.
func TestClosedQuestionService_HalfAScopeIsRejectedRatherThanClearingAll(t *testing.T) {
	service := NewClosedQuestionService(t)
	postSubscription(t, service, "00000010", "999900006")

	req, err := http.NewRequest(http.MethodDelete,
		service.GetURL()+"/abonnementen/fhir/Subscription?providerid=00000010", nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	require.Len(t, service.GetSubscriptions(), 1, "an ambiguous scope must not clear the store")
}

func postSubscription(t *testing.T, service *ClosedQuestionService, providerURA, bsn string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"resourceType": "Subscription",
		"status":       "requested",
		"criteria":     "Consent?_query=otv&patientid=" + bsn + "&providerid=" + providerURA,
		"channel":      map[string]any{"type": "rest-hook"},
	})
	require.NoError(t, err)
	res, err := http.Post(service.GetURL()+"/abonnementen/fhir/Subscription",
		"application/fhir+json", bytes.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)
}

func deleteSubscriptions(t *testing.T, service *ClosedQuestionService, scope url.Values) {
	t.Helper()
	endpoint := service.GetURL() + "/abonnementen/fhir/Subscription"
	if len(scope) > 0 {
		endpoint += "?" + scope.Encode()
	}
	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	require.NoError(t, err)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}
