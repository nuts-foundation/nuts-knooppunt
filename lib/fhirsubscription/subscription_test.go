package fhirsubscription

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
)

// specSubscriptionExample is the in-band creation example from TTA Notifications v0.6
// (docs/tta-notifications-v06/02-tta-notifications-v06-spec.md, "Creating a Subscription").
const specSubscriptionExample = `{
  "resourceType": "Subscription",
  "status": "requested",
  "reason": "test",
  "criteria": "SubscriptionTopic/task-status-change",
  "_criteria": {
    "extension": [
      {
        "url": "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-topic-canonical",
        "valueCanonical": "https://example.org/fhir/SubscriptionTopic/task-status-change"
      },
      {
        "url": "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-filter-criteria",
        "valueString": "Task.owner=Organization/receiver-org"
      }
    ]
  },
  "channel": {
    "type": "rest-hook",
    "endpoint": "https://receiver.example/fhir/notifications",
    "payload": "application/fhir+json",
    "_payload": {
      "extension": [
        {
          "url": "http://hl7.org/fhir/uv/subscriptions-backport/StructureDefinition/backport-payload-content",
          "valueCode": "id-only"
        }
      ]
    }
  }
}`

func TestUnmarshalSubscription_SpecExample(t *testing.T) {
	sub, err := UnmarshalSubscription([]byte(specSubscriptionExample))
	require.NoError(t, err)

	assert.Equal(t, StatusRequested, sub.Status)
	assert.Equal(t, "SubscriptionTopic/task-status-change", sub.Criteria)
	assert.Equal(t, "https://example.org/fhir/SubscriptionTopic/task-status-change", sub.TopicCanonical())
	assert.Equal(t, []string{"Task.owner=Organization/receiver-org"}, sub.FilterCriteria())
	assert.Equal(t, PayloadModeIDOnly, sub.PayloadContent())
	assert.Nil(t, sub.HeartbeatPeriod())
	require.NotNil(t, sub.Channel.Endpoint)
	assert.Equal(t, "https://receiver.example/fhir/notifications", *sub.Channel.Endpoint)
	assert.Equal(t, "rest-hook", sub.Channel.Type)
}

func TestSubscription_RoundTrip(t *testing.T) {
	sub := Build(BuildOptions{
		TopicCanonical:  "http://fhir.nl/SubscriptionTopic/nl-task-notified-pull",
		FilterCriteria:  []string{"Task.owner=http://fhir.nl/fhir/NamingSystem/ura|00000020"},
		Endpoint:        "https://receiver.example/fhir/notifications",
		Mode:            PayloadModeIDOnly,
		HeartbeatPeriod: to.Ptr(60),
	})

	data, err := json.Marshal(sub)
	require.NoError(t, err)

	// The wire shape must carry the backport primitive extensions, not Go-invented keys.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "Subscription", raw["resourceType"])
	assert.Contains(t, raw, "_criteria")
	channel := raw["channel"].(map[string]any)
	assert.Contains(t, channel, "_payload")
	assert.Contains(t, channel, "extension") // heartbeat period

	parsed, err := UnmarshalSubscription(data)
	require.NoError(t, err)
	assert.Equal(t, sub.TopicCanonical(), parsed.TopicCanonical())
	assert.Equal(t, sub.FilterCriteria(), parsed.FilterCriteria())
	assert.Equal(t, PayloadModeIDOnly, parsed.PayloadContent())
	require.NotNil(t, parsed.HeartbeatPeriod())
	assert.Equal(t, 60, *parsed.HeartbeatPeriod())
}

func TestUnmarshalSubscription_WrongResourceType(t *testing.T) {
	_, err := UnmarshalSubscription([]byte(`{"resourceType":"Patient"}`))
	assert.ErrorContains(t, err, "expected resourceType Subscription")
}
