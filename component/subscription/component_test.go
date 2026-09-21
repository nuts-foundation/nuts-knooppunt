package subscription

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirsubscription"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
)

func newTestComponent(t *testing.T, config Config) *Component {
	t.Helper()
	if config.Server.RetryMaxAttempts == 0 {
		config.Server.RetryMaxAttempts = 2
	}
	if config.Server.RetryBaseDelayMillis == 0 {
		config.Server.RetryBaseDelayMillis = 5
	}
	if len(config.Server.AgreedTopics) == 0 {
		config.Server.AgreedTopics = DefaultConfig().Server.AgreedTopics
	}
	if len(config.Server.AllowedModes) == 0 {
		config.Server.AllowedModes = DefaultConfig().Server.AllowedModes
	}
	if len(config.Client.AgreedTopics) == 0 {
		config.Client.AgreedTopics = DefaultConfig().Client.AgreedTopics
	}
	if len(config.Client.AllowedModes) == 0 {
		config.Client.AllowedModes = DefaultConfig().Client.AllowedModes
	}
	if config.PublicBaseURL == "" {
		config.PublicBaseURL = "http://sender.example"
	}
	component, err := New(config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = component.Stop(context.Background()) })
	return component
}

func TestAliaser(t *testing.T) {
	aliaser := newAliaser("test-key")
	alias := aliaser.alias("Task/4711")

	// Stability: same input, same alias.
	assert.Equal(t, alias, aliaser.alias("Task/4711"))
	// Opacity: the internal id doesn't appear.
	assert.NotContains(t, alias, "4711")
	// Spec example shape: 24 lowercase base32 characters.
	assert.Len(t, alias, 24)
	// A different key yields a different alias (unpredictability without the key).
	assert.NotEqual(t, alias, newAliaser("other-key").alias("Task/4711"))
}

func TestFilterURA(t *testing.T) {
	assert.Equal(t, "00000020", filterURA("Task.owner=http://fhir.nl/fhir/NamingSystem/ura|00000020"))
	assert.Equal(t, "00000020", filterURA("Task.owner=00000020"))
	assert.Equal(t, "", filterURA("Task.owner=Organization/receiver-org"))
	assert.Equal(t, "", filterURA("Task.code=fulfill"))
}

func TestMatchesTopic(t *testing.T) {
	component := newTestComponent(t, Config{Server: ServerConfig{Enabled: true}})
	sub := &serverSubscription{
		Topic:   TaskTopicCanonical,
		Filters: []string{"Task.owner=http://fhir.nl/fhir/NamingSystem/ura|00000020"},
	}

	assert.True(t, component.matchesTopic(sub, event{ResourceType: "Task", Interaction: "create", OwnerURA: "00000020"}))
	assert.True(t, component.matchesTopic(sub, event{ResourceType: "Task", Interaction: "update", OwnerURA: "00000020"}))
	// Owner-less events match permissively (poller may not know the owner).
	assert.True(t, component.matchesTopic(sub, event{ResourceType: "Task", Interaction: "update"}))
	assert.False(t, component.matchesTopic(sub, event{ResourceType: "Task", Interaction: "update", OwnerURA: "99999999"}))
	assert.False(t, component.matchesTopic(sub, event{ResourceType: "Observation", Interaction: "create", OwnerURA: "00000020"}))
	assert.False(t, component.matchesTopic(sub, event{ResourceType: "Task", Interaction: "delete", OwnerURA: "00000020"}))

	otherTopic := &serverSubscription{Topic: "http://example.org/other"}
	assert.False(t, component.matchesTopic(otherTopic, event{ResourceType: "Task", Interaction: "create"}))
}

func TestCreateInBandSubscription_Validation(t *testing.T) {
	component := newTestComponent(t, Config{Server: ServerConfig{Enabled: true}})

	valid := func() fhirsubscription.Subscription {
		return fhirsubscription.Build(fhirsubscription.BuildOptions{
			TopicCanonical: TaskTopicCanonical,
			Endpoint:       "http://receiver.example/fhir/notifications",
			Mode:           fhirsubscription.PayloadModeIDOnly,
		})
	}

	t.Run("unknown topic is 422", func(t *testing.T) {
		sub := fhirsubscription.Build(fhirsubscription.BuildOptions{
			TopicCanonical: "http://example.org/not-agreed",
			Endpoint:       "http://receiver.example/fhir/notifications",
			Mode:           fhirsubscription.PayloadModeIDOnly,
		})
		_, reqErr := component.CreateInBandSubscription(context.Background(), sub)
		require.NotNil(t, reqErr)
		assert.Equal(t, http.StatusUnprocessableEntity, reqErr.Status)
	})

	t.Run("full-resource mode is outside the agreement", func(t *testing.T) {
		sub := valid()
		sub.Channel.PayloadElement.Extension[0].ValueCode = to.Ptr("full-resource")
		_, reqErr := component.CreateInBandSubscription(context.Background(), sub)
		require.NotNil(t, reqErr)
		assert.Equal(t, http.StatusUnprocessableEntity, reqErr.Status)
	})

	t.Run("missing endpoint is 400", func(t *testing.T) {
		sub := valid()
		sub.Channel.Endpoint = nil
		_, reqErr := component.CreateInBandSubscription(context.Background(), sub)
		require.NotNil(t, reqErr)
		assert.Equal(t, http.StatusBadRequest, reqErr.Status)
	})

	t.Run("wrong channel type is 400", func(t *testing.T) {
		sub := valid()
		sub.Channel.Type = "websocket"
		_, reqErr := component.CreateInBandSubscription(context.Background(), sub)
		require.NotNil(t, reqErr)
		assert.Equal(t, http.StatusBadRequest, reqErr.Status)
	})

	t.Run("wrong status is 400", func(t *testing.T) {
		sub := valid()
		sub.Status = fhirsubscription.StatusActive
		_, reqErr := component.CreateInBandSubscription(context.Background(), sub)
		require.NotNil(t, reqErr)
		assert.Equal(t, http.StatusBadRequest, reqErr.Status)
	})
}

// TestHandshakeLifecycle exercises requested -> active on a 200 handshake and
// requested -> error on persistent failure.
func TestHandshakeLifecycle(t *testing.T) {
	t.Run("successful handshake activates", func(t *testing.T) {
		received := make(chan []byte, 1)
		receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(body)
			received <- body
			w.WriteHeader(http.StatusOK)
		}))
		defer receiver.Close()

		component := newTestComponent(t, Config{Server: ServerConfig{Enabled: true}})
		sub := fhirsubscription.Build(fhirsubscription.BuildOptions{
			TopicCanonical: TaskTopicCanonical,
			Endpoint:       receiver.URL,
			Mode:           fhirsubscription.PayloadModeIDOnly,
		})
		created, reqErr := component.CreateInBandSubscription(context.Background(), sub)
		require.Nil(t, reqErr)
		require.NotNil(t, created.ID)

		select {
		case body := <-received:
			var raw map[string]any
			require.NoError(t, json.Unmarshal(body, &raw))
			assert.Contains(t, string(body), "handshake-notification")
		case <-time.After(2 * time.Second):
			t.Fatal("no handshake delivered")
		}
		require.Eventually(t, func() bool {
			stored, _ := component.store.getServerSub(*created.ID)
			return stored.Status == fhirsubscription.StatusActive
		}, 2*time.Second, 10*time.Millisecond)
	})

	t.Run("persistent failure sets error", func(t *testing.T) {
		component := newTestComponent(t, Config{Server: ServerConfig{Enabled: true}})
		sub := fhirsubscription.Build(fhirsubscription.BuildOptions{
			TopicCanonical: TaskTopicCanonical,
			Endpoint:       "http://127.0.0.1:1", // nothing listens here
			Mode:           fhirsubscription.PayloadModeIDOnly,
		})
		created, reqErr := component.CreateInBandSubscription(context.Background(), sub)
		require.Nil(t, reqErr)
		require.Eventually(t, func() bool {
			stored, _ := component.store.getServerSub(*created.ID)
			return stored.Status == fhirsubscription.StatusError
		}, 2*time.Second, 10*time.Millisecond)
	})
}

// TestEventNumbering verifies monotonic, per-subscription event numbers under
// concurrent ingestion (the spec's concurrency-safety rule).
func TestEventNumbering(t *testing.T) {
	component := newTestComponent(t, Config{Server: ServerConfig{Enabled: true}})
	sub := serverSubscription{
		ID:     "sub-1",
		Topic:  TaskTopicCanonical,
		Status: fhirsubscription.StatusActive,
		Mode:   fhirsubscription.PayloadModeIDOnly,
	}
	component.store.mu.Lock()
	stored := sub
	component.store.serverSubs[sub.ID] = &stored
	component.store.mu.Unlock()

	done := make(chan int64, 100)
	for i := 0; i < 100; i++ {
		go func() {
			record, ok := component.store.allocateEvent("sub-1", func(number int64) eventRecord {
				return eventRecord{EventNumber: number, Timestamp: time.Now()}
			})
			require.True(t, ok)
			done <- record.EventNumber
		}()
	}
	seen := map[int64]bool{}
	for i := 0; i < 100; i++ {
		number := <-done
		assert.False(t, seen[number], "event number %d allocated twice", number)
		seen[number] = true
	}
	final, _ := component.store.getServerSub("sub-1")
	assert.Equal(t, int64(100), final.EventCounter)
}

// TestPollTasks feeds a fake FHIR Task/_history and verifies detection + event pipeline
// entry.
func TestPollTasks(t *testing.T) {
	historyJSON := `{
	  "resourceType": "Bundle", "type": "history",
	  "entry": [
	    {"resource": {"resourceType": "Task", "id": "task-1", "status": "requested", "intent": "order",
	      "meta": {"versionId": "1"},
	      "code": {"coding": [
	        {"system": "http://hl7.org/fhir/CodeSystem/task-code", "code": "fulfill"},
	        {"system": "http://snomed.info/sct", "code": "308292007"}]},
	      "owner": {"identifier": {"system": "http://fhir.nl/fhir/NamingSystem/ura", "value": "00000020"}}}}
	  ]
	}`
	fhirServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/Task/_history", r.URL.Path)
		w.Header().Set("Content-Type", "application/fhir+json")
		_, _ = w.Write([]byte(historyJSON))
	}))
	defer fhirServer.Close()

	component := newTestComponent(t, Config{Server: ServerConfig{Enabled: true, FHIRBaseURL: fhirServer.URL}})
	// An active subscription for the partner the Task addresses; suppression-free
	// delivery would need a live endpoint, so subscribe a test receiver.
	notified := make(chan struct{}, 1)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notified <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	component.store.mu.Lock()
	component.store.serverSubs["sub-1"] = &serverSubscription{
		ID:       "sub-1",
		Topic:    TaskTopicCanonical,
		Filters:  []string{"Task.owner=http://fhir.nl/fhir/NamingSystem/ura|00000020"},
		Mode:     fhirsubscription.PayloadModeIDOnly,
		Endpoint: receiver.URL,
		Status:   fhirsubscription.StatusActive,
	}
	component.store.mu.Unlock()

	component.pollTasks(context.Background())

	select {
	case <-notified:
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not deliver a notification")
	}
	sub, _ := component.store.getServerSub("sub-1")
	assert.Equal(t, int64(1), sub.EventCounter)

	// The focus must be aliased: the internal id may not leak.
	records, _ := component.store.eventRange("sub-1", 1, 1)
	require.Len(t, records, 1)
	require.NotNil(t, records[0].Focus)
	assert.NotContains(t, *records[0].Focus, "task-1")
}
