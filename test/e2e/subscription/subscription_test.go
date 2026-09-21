package subscription

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/cmd"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirsubscription"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
)

func ingest(t *testing.T, server *node, ev api.SubscriptionEventIngest) api.SubscriptionEventIngestReport {
	t.Helper()
	status, report := postJSON[api.SubscriptionEventIngestReport](t, server.InternalURL+"/subscription/events", ev)
	require.Equal(t, http.StatusOK, status)
	return report
}

func taskEvent(id string, suppress bool) api.SubscriptionEventIngest {
	interaction := api.Create
	return api.SubscriptionEventIngest{
		ResourceType:     "Task",
		ResourceId:       id,
		Interaction:      &interaction,
		OwnerUra:         to.Ptr(zonnebloemURA),
		UseCaseCode:      to.Ptr("308292007"),
		SuppressDelivery: to.Ptr(suppress),
	}
}

func listing(t *testing.T, n *node) api.SubscriptionListing {
	t.Helper()
	status, result := getAs[api.SubscriptionListing](t, n.InternalURL+"/subscription/subscriptions")
	require.Equal(t, http.StatusOK, status)
	return result
}

func inboxOf(t *testing.T, n *node) []api.SubscriptionInboxEvent {
	t.Helper()
	status, result := getAs[[]api.SubscriptionInboxEvent](t, n.InternalURL+"/subscription/inbox?all=true")
	require.Equal(t, http.StatusOK, status)
	return result
}

func logOf(t *testing.T, n *node) []api.SubscriptionTransportLogEntry {
	t.Helper()
	status, result := getAs[[]api.SubscriptionTransportLogEntry](t, n.InternalURL+"/subscription/log")
	require.Equal(t, http.StatusOK, status)
	return result
}

func logContains(entries []api.SubscriptionTransportLogEntry, entryType, outcomeSubstring string) bool {
	for _, entry := range entries {
		if entry.Type == entryType && strings.Contains(entry.Outcome, outcomeSubstring) {
			return true
		}
	}
	return false
}

// waitActiveOnBoth waits until the server lists an active subscription and the client's
// record is active too (handshake completed).
func waitActiveOnBoth(t *testing.T, server, client *node) (api.ServerSubscriptionInfo, api.ClientSubscriptionInfo) {
	t.Helper()
	var serverSub api.ServerSubscriptionInfo
	var clientSub api.ClientSubscriptionInfo
	require.Eventually(t, func() bool {
		serverListing := listing(t, server)
		clientListing := listing(t, client)
		if len(serverListing.Server) == 0 || len(clientListing.Client) == 0 {
			return false
		}
		serverSub = serverListing.Server[0]
		clientSub = clientListing.Client[0]
		return serverSub.Status == api.ServerSubscriptionInfoStatusActive &&
			clientSub.Status == api.ClientSubscriptionInfoStatusActive
	}, 10*time.Second, 100*time.Millisecond, "subscription did not become active on both sides")
	return serverSub, clientSub
}

func inboxEvents(entries []api.SubscriptionInboxEvent) []api.SubscriptionInboxEvent {
	var events []api.SubscriptionInboxEvent
	for _, entry := range entries {
		if entry.Type == api.Event {
			events = append(events, entry)
		}
	}
	return events
}

func inboxGaps(entries []api.SubscriptionInboxEvent) []api.SubscriptionInboxEvent {
	var gaps []api.SubscriptionInboxEvent
	for _, entry := range entries {
		if entry.Type == api.Gap {
			gaps = append(gaps, entry)
		}
	}
	return gaps
}

// assertUniformInboxEvents is the research-question-6 check: whatever the permutation,
// the vendor-facing inbox events have one identical shape.
func assertUniformInboxEvents(t *testing.T, entries []api.SubscriptionInboxEvent, wantFocus bool) {
	t.Helper()
	for _, entry := range inboxEvents(entries) {
		assert.NotZero(t, entry.Id, "inbox event without sequence id")
		assert.NotEmpty(t, entry.SubscriptionReference)
		require.NotNil(t, entry.EventNumber)
		require.NotNil(t, entry.Topic)
		assert.Equal(t, taskTopic, *entry.Topic)
		if wantFocus {
			require.NotNil(t, entry.Focus, "id-only event %d without focus", *entry.EventNumber)
		} else {
			assert.Nil(t, entry.Focus, "empty-mode event %d leaked a focus", *entry.EventNumber)
		}
	}
}

// eventNumbers extracts the event numbers of type=event entries in inbox order.
func eventNumbers(entries []api.SubscriptionInboxEvent) []int64 {
	var numbers []int64
	for _, entry := range inboxEvents(entries) {
		numbers = append(numbers, *entry.EventNumber)
	}
	return numbers
}

// Permutation 1 — in-band creation, heartbeat on/on, $events available, id-only:
// full happy path, gap recovery via $events, aliased follow-up pull, idempotency,
// inbox ack.
func TestPermutation1_InBandHappyPath(t *testing.T) {
	// Fake FHIR store at the sender so the follow-up pull has something to serve.
	taskJSON := `{"resourceType": "Task", "id": "task-1", "status": "requested", "intent": "order"}`
	fakeStore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/Task/task-1" {
			w.Header().Set("Content-Type", "application/fhir+json")
			_, _ = w.Write([]byte(taskJSON))
			return
		}
		http.NotFound(w, r)
	}))
	defer fakeStore.Close()

	server, client := startPair(t, func(serverCfg, clientCfg *cmd.Config) {
		serverCfg.Subscription.Server.FHIRBaseURL = fakeStore.URL
	})

	// Vendor registers the authorization basis when the care relationship is created.
	status, _ := postJSON[api.SubscriptionAuthorizationRequest](t, server.InternalURL+"/subscription/authorizations", api.SubscriptionAuthorizationRequest{
		ReceiverUra: zonnebloemURA,
		Topic:       taskTopic,
	})
	require.Equal(t, http.StatusCreated, status)

	// Client subscribes in-band at the server: one internal call.
	status, intentResult := postJSON[api.ClientSubscriptionInfo](t, client.InternalURL+"/subscription/intents", api.SubscriptionIntentRequest{
		PartnerUra:             plataanURA,
		Topic:                  taskTopic,
		HeartbeatPeriodSeconds: to.Ptr(1),
	})
	require.Equal(t, http.StatusCreated, status)
	assert.Equal(t, "id-only", intentResult.PayloadMode, "id-only is the default mode")

	serverSub, clientSub := waitActiveOnBoth(t, server, client)
	assert.Equal(t, api.ServerSubscriptionInfoCreatedViaInBand, serverSub.CreatedVia)
	assert.Equal(t, api.ClientSubscriptionInfoOriginInBand, clientSub.Origin)
	require.NotNil(t, serverSub.HeartbeatPeriodSeconds, "server did not record the agreed heartbeat")

	// Event 1: delivered normally.
	report := ingest(t, server, taskEvent("task-1", false))
	require.Len(t, report.Matches, 1)
	assert.Equal(t, int64(1), report.Matches[0].EventNumber)

	require.Eventually(t, func() bool {
		return len(inboxEvents(inboxOf(t, client))) == 1
	}, 5*time.Second, 100*time.Millisecond, "event 1 did not reach the inbox")

	entries := inboxOf(t, client)
	firstEvent := inboxEvents(entries)[0]
	require.NotNil(t, firstEvent.Focus)
	// Identifier requirements (research question 8): the focus is aliased — the
	// internal id must not leak.
	assert.NotContains(t, *firstEvent.Focus, "task-1")

	// Follow-up pull through the de-aliasing read: the knooppunt in the data path.
	aliasID := strings.TrimPrefix(*firstEvent.Focus, "Task/")
	pullStatus, pulled := getAs[map[string]any](t, server.PublicURL+"/fhir/Task/"+aliasID)
	require.Equal(t, http.StatusOK, pullStatus)
	assert.Equal(t, "Task", pulled["resourceType"])
	assert.Equal(t, aliasID, pulled["id"], "pull response must carry the alias, not the internal id")

	// Event 2 is lost (suppressed), event 3 delivered: the client must detect the gap
	// and recover event 2 via $events.
	ingest(t, server, taskEvent("task-1", true))
	ingest(t, server, taskEvent("task-1", false))

	require.Eventually(t, func() bool {
		return len(inboxEvents(inboxOf(t, client))) == 3
	}, 5*time.Second, 100*time.Millisecond, "gap recovery via $events did not complete")

	entries = inboxOf(t, client)
	assert.Equal(t, []int64{1, 2, 3}, eventNumbers(entries), "events must arrive in order, gap filled")
	assert.Empty(t, inboxGaps(entries), "recoverable gap must not surface as a gap entry")
	assert.True(t, logContains(logOf(t, client), "events-query", "200"), "client log must show the $events recovery call")
	assertUniformInboxEvents(t, entries, true)

	// Idempotency: replay event 3 verbatim at the notification endpoint.
	replay, err := fhirsubscription.Notification{
		Type:                  fhirsubscription.NotificationTypeEvent,
		SubscriptionReference: clientSub.Reference,
		Status:                fhirsubscription.StatusActive,
		Events:                []fhirsubscription.NotificationEvent{{EventNumber: 3, Focus: firstEvent.Focus}},
	}.BuildBundle(fhirsubscription.PayloadModeIDOnly)
	require.NoError(t, err)
	replayJSON, err := json.Marshal(replay)
	require.NoError(t, err)
	resp, err := http.Post(client.PublicURL+"/fhir/notifications", "application/fhir+json", bytes.NewReader(replayJSON))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "duplicate must be acknowledged")
	assert.Len(t, inboxEvents(inboxOf(t, client)), 3, "duplicate must not create a new inbox entry")
	assert.True(t, logContains(logOf(t, client), "event", "duplicate"))

	// Heartbeats: agreed period 1s, so within a few seconds both sides log them.
	require.Eventually(t, func() bool {
		return logContains(logOf(t, server), "heartbeat", "200") &&
			logContains(logOf(t, client), "heartbeat", "200")
	}, 10*time.Second, 200*time.Millisecond, "heartbeats were not exchanged")
	assert.False(t, logContains(logOf(t, client), "heartbeat", "missed"), "no heartbeat may be missed on the happy path")

	// Inbox acknowledgement: at-least-once semantics on the internal hop.
	ackResp, err := http.Post(fmt.Sprintf("%s/subscription/inbox/%d/ack", client.InternalURL, firstEvent.Id), "application/json", nil)
	require.NoError(t, err)
	ackResp.Body.Close()
	require.Equal(t, http.StatusNoContent, ackResp.StatusCode)
	statusCode, unacked := getAs[[]api.SubscriptionInboxEvent](t, client.InternalURL+"/subscription/inbox")
	require.Equal(t, http.StatusOK, statusCode)
	for _, entry := range unacked {
		assert.NotEqual(t, firstEvent.Id, entry.Id, "acknowledged entry must leave the default inbox view")
	}
}

// Permutation 2 — out-of-band creation, no heartbeats, no $events, id-only: the client
// bootstraps from an unknown subscription; gap recovery degrades to $status plus a gap
// entry telling the vendor to resync.
func TestPermutation2_OutOfBandDegradedRecovery(t *testing.T) {
	server, client := startPair(t, func(serverCfg, clientCfg *cmd.Config) {
		serverCfg.Subscription.Server.EventsOperationEnabled = false
	})

	// The sender's system decides the partner should be informed: out-of-band setup.
	status, created := postJSON[api.ServerSubscriptionInfo](t, server.InternalURL+"/subscription/subscriptions", api.OutOfBandSubscriptionRequest{
		PartnerUra: zonnebloemURA,
		Topic:      taskTopic,
	})
	require.Equal(t, http.StatusCreated, status)
	assert.Equal(t, api.ServerSubscriptionInfoCreatedViaOutOfBand, created.CreatedVia)
	assert.Nil(t, created.HeartbeatPeriodSeconds)

	// The handshake hits a subscription the client never created; it must fetch the
	// Subscription resource, check it against the agreement, and accept.
	serverSub, clientSub := waitActiveOnBoth(t, server, client)
	assert.Equal(t, api.ClientSubscriptionInfoOriginOutOfBand, clientSub.Origin)
	assert.Equal(t, serverSub.Id, *clientSub.Id)
	assert.True(t, logContains(logOf(t, client), "subscription-fetch", "200"), "client must have fetched the unknown Subscription")

	// Event 1 delivered; event 2 lost; event 3 delivered. No $events on this server:
	// recovery degrades to $status and a gap entry.
	ingest(t, server, taskEvent("task-a", false))
	require.Eventually(t, func() bool {
		return len(inboxEvents(inboxOf(t, client))) == 1
	}, 5*time.Second, 100*time.Millisecond)
	ingest(t, server, taskEvent("task-a", true))
	ingest(t, server, taskEvent("task-a", false))

	require.Eventually(t, func() bool {
		entries := inboxOf(t, client)
		return len(inboxEvents(entries)) == 2 && len(inboxGaps(entries)) == 1
	}, 5*time.Second, 100*time.Millisecond, "degraded recovery did not produce event 3 plus a gap entry")

	entries := inboxOf(t, client)
	assert.Equal(t, []int64{1, 3}, eventNumbers(entries))
	gap := inboxGaps(entries)[0]
	require.NotNil(t, gap.GapFrom)
	require.NotNil(t, gap.GapTo)
	assert.Equal(t, int64(2), *gap.GapFrom)
	assert.Equal(t, int64(2), *gap.GapTo)

	clientLog := logOf(t, client)
	assert.True(t, logContains(clientLog, "events-query", "404"), "$events must have been tried and 404ed")
	assert.True(t, logContains(clientLog, "status-query", "200"), "recovery must degrade to $status")
	assertUniformInboxEvents(t, entries, true)
}

// Permutation 3 — in-band, server sends heartbeats the client ignores, empty payload
// mode: heartbeats are harmless noise, and empty-mode events carry no focus.
func TestPermutation3_HeartbeatIgnoredEmptyMode(t *testing.T) {
	server, client := startPair(t, func(serverCfg, clientCfg *cmd.Config) {
		clientCfg.Subscription.Client.HeartbeatMissCheck = false
	})

	payloadMode := api.SubscriptionIntentRequestPayloadMode("empty")
	status, _ := postJSON[api.ClientSubscriptionInfo](t, client.InternalURL+"/subscription/intents", api.SubscriptionIntentRequest{
		PartnerUra:             plataanURA,
		Topic:                  taskTopic,
		PayloadMode:            &payloadMode,
		HeartbeatPeriodSeconds: to.Ptr(1),
	})
	require.Equal(t, http.StatusCreated, status)
	waitActiveOnBoth(t, server, client)

	ingest(t, server, taskEvent("task-b", false))
	require.Eventually(t, func() bool {
		return len(inboxEvents(inboxOf(t, client))) == 1
	}, 5*time.Second, 100*time.Millisecond)

	entries := inboxOf(t, client)
	assertUniformInboxEvents(t, entries, false) // empty mode: no focus anywhere

	// Server keeps sending heartbeats; the client accepts (200) but never reacts.
	require.Eventually(t, func() bool {
		return logContains(logOf(t, server), "heartbeat", "200")
	}, 10*time.Second, 200*time.Millisecond)
	clientLog := logOf(t, client)
	assert.True(t, logContains(clientLog, "heartbeat", "200"), "ignored heartbeats are still acknowledged and logged")
	assert.False(t, logContains(clientLog, "heartbeat", "missed"))
	assert.False(t, logContains(clientLog, "status-query", "200"), "an ignoring client never queries $status")
}

// Permutation 4 — client requests a heartbeat the server does not support: the spec
// says heartbeats are "agreed at creation" but not what happens when the agreement is
// not honoured. The server strips it; the client's timer fires and $status shows all is
// well — busywork, not failure. (Finding.)
func TestPermutation4_HeartbeatNotHonoured(t *testing.T) {
	server, client := startPair(t, func(serverCfg, clientCfg *cmd.Config) {
		serverCfg.Subscription.Server.HeartbeatSupported = false
	})

	status, _ := postJSON[api.ClientSubscriptionInfo](t, client.InternalURL+"/subscription/intents", api.SubscriptionIntentRequest{
		PartnerUra:             plataanURA,
		Topic:                  taskTopic,
		HeartbeatPeriodSeconds: to.Ptr(1),
	})
	require.Equal(t, http.StatusCreated, status)
	serverSub, _ := waitActiveOnBoth(t, server, client)

	// The created resource shows no heartbeat — an inspecting client could notice.
	assert.Nil(t, serverSub.HeartbeatPeriodSeconds, "server must not record a heartbeat it will not honour")

	// The client still believes one was agreed: its miss-timer fires and it queries
	// $status, which reports active. Repeatedly, forever.
	require.Eventually(t, func() bool {
		clientLog := logOf(t, client)
		return logContains(clientLog, "heartbeat", "missed") && logContains(clientLog, "status-query", "200")
	}, 10*time.Second, 200*time.Millisecond, "client miss-detection did not fire")

	// The subscription itself stays healthy: events still arrive.
	ingest(t, server, taskEvent("task-c", false))
	require.Eventually(t, func() bool {
		return len(inboxEvents(inboxOf(t, client))) == 1
	}, 5*time.Second, 100*time.Millisecond)
	assertUniformInboxEvents(t, inboxOf(t, client), true)
}

// Permutation 5 — in-band, no heartbeats, server without $events, client asks anyway:
// the 404 OperationOutcome is handled gracefully and recovery degrades.
func TestPermutation5_EventsUnsupported(t *testing.T) {
	server, client := startPair(t, func(serverCfg, clientCfg *cmd.Config) {
		serverCfg.Subscription.Server.EventsOperationEnabled = false
	})

	status, _ := postJSON[api.ClientSubscriptionInfo](t, client.InternalURL+"/subscription/intents", api.SubscriptionIntentRequest{
		PartnerUra: plataanURA,
		Topic:      taskTopic,
	})
	require.Equal(t, http.StatusCreated, status)
	waitActiveOnBoth(t, server, client)

	ingest(t, server, taskEvent("task-d", true))  // event 1 lost
	ingest(t, server, taskEvent("task-d", false)) // event 2 delivered

	require.Eventually(t, func() bool {
		entries := inboxOf(t, client)
		return len(inboxEvents(entries)) == 1 && len(inboxGaps(entries)) == 1
	}, 5*time.Second, 100*time.Millisecond)

	entries := inboxOf(t, client)
	assert.Equal(t, []int64{2}, eventNumbers(entries))
	clientLog := logOf(t, client)
	assert.True(t, logContains(clientLog, "events-query", "404"))
	assert.True(t, logContains(clientLog, "status-query", "200"))
	assertUniformInboxEvents(t, entries, true)
}

// Permutation 6 — out-of-band creation with a payload mode outside the receiver's
// agreement: the receiver inspects the Subscription and refuses the handshake; the
// sender ends in error.
func TestPermutation6_ModeOutsideAgreementRejected(t *testing.T) {
	server, client := startPair(t, func(serverCfg, clientCfg *cmd.Config) {
		// The receiver's agreement covers id-only only; the sender still allows empty.
		clientCfg.Subscription.Client.AllowedModes = []string{"id-only"}
	})

	payloadMode := api.OutOfBandSubscriptionRequestPayloadModeEmpty
	status, created := postJSON[api.ServerSubscriptionInfo](t, server.InternalURL+"/subscription/subscriptions", api.OutOfBandSubscriptionRequest{
		PartnerUra:  zonnebloemURA,
		Topic:       taskTopic,
		PayloadMode: &payloadMode,
	})
	require.Equal(t, http.StatusCreated, status)

	// The handshake is refused (422), which is definitive: the sender must end in
	// error, and the receiver must not track the subscription.
	require.Eventually(t, func() bool {
		serverListing := listing(t, server)
		return len(serverListing.Server) == 1 && serverListing.Server[0].Status == api.ServerSubscriptionInfoStatusError
	}, 10*time.Second, 100*time.Millisecond, "sender did not end in error after refused handshake")

	assert.Empty(t, listing(t, client).Client, "receiver must not track a refused subscription")
	assert.True(t, logContains(logOf(t, client), "handshake", "422"), "receiver log must show the refusal")
	assert.Empty(t, inboxOf(t, client))
	_ = created
}
