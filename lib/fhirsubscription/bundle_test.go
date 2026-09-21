package fhirsubscription

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Wire examples from TTA Notifications v0.6
// (docs/tta-notifications-v06/02-tta-notifications-v06-spec.md).

const specHandshakeExample = `{
  "resourceType": "Bundle",
  "type": "history",
  "entry": [
    {
      "resource": {
        "resourceType": "Parameters",
        "parameter": [
          { "name": "subscription",
            "valueReference": { "reference": "Subscription/7f3e9a2c-5d18-4b6f-9c3a-8e2d4f6b1a59" } },
          { "name": "status", "valueCode": "requested" },
          { "name": "type", "valueCode": "handshake-notification" }
        ]
      }
    }
  ]
}`

const specEventExample = `{
  "resourceType": "Bundle",
  "type": "history",
  "entry": [
    {
      "fullUrl": "urn:uuid:c3a5d8f1-9b2e-4d67-8a4c-5e1f7b9d2a36",
      "resource": {
        "resourceType": "Parameters",
        "parameter": [
          { "name": "subscription",
            "valueReference": { "reference": "Subscription/7f3e9a2c-5d18-4b6f-9c3a-8e2d4f6b1a59" } },
          { "name": "status", "valueCode": "active" },
          { "name": "type", "valueCode": "event-notification" },
          { "name": "notification-event",
            "part": [
              { "name": "event-number", "valueString": "42" },
              { "name": "timestamp", "valueInstant": "2026-07-16T09:15:00Z" },
              { "name": "focus",
                "valueReference": { "reference": "Task/5f2f9a4e-8c1d-4b6e-9d3a-7c0e2f4b8a1d" } }
            ] }
        ]
      }
    },
    {
      "fullUrl": "https://sender.example/fhir/Task/5f2f9a4e-8c1d-4b6e-9d3a-7c0e2f4b8a1d",
      "request": { "method": "GET", "url": "Task/5f2f9a4e-8c1d-4b6e-9d3a-7c0e2f4b8a1d" },
      "response": { "status": "200" }
    }
  ]
}`

const specHeartbeatExample = `{
  "resourceType": "Bundle",
  "type": "history",
  "entry": [
    {
      "resource": {
        "resourceType": "Parameters",
        "parameter": [
          { "name": "subscription",
            "valueReference": { "reference": "Subscription/7f3e9a2c-5d18-4b6f-9c3a-8e2d4f6b1a59" } },
          { "name": "status", "valueCode": "active" },
          { "name": "type", "valueCode": "heartbeat-notification" }
        ]
      }
    }
  ]
}`

func parseBundleJSON(t *testing.T, data string) fhir.Bundle {
	t.Helper()
	var bundle fhir.Bundle
	require.NoError(t, json.Unmarshal([]byte(data), &bundle))
	return bundle
}

func TestParseBundle_SpecHandshake(t *testing.T) {
	notification, err := ParseBundle(parseBundleJSON(t, specHandshakeExample))
	require.NoError(t, err)
	assert.Equal(t, NotificationTypeHandshake, notification.Type)
	assert.Equal(t, StatusRequested, notification.Status)
	assert.Equal(t, "Subscription/7f3e9a2c-5d18-4b6f-9c3a-8e2d4f6b1a59", notification.SubscriptionReference)
	assert.Empty(t, notification.Events)
}

func TestParseBundle_SpecEvent(t *testing.T) {
	notification, err := ParseBundle(parseBundleJSON(t, specEventExample))
	require.NoError(t, err)
	assert.Equal(t, NotificationTypeEvent, notification.Type)
	assert.Equal(t, StatusActive, notification.Status)
	require.Len(t, notification.Events, 1)
	event := notification.Events[0]
	assert.Equal(t, int64(42), event.EventNumber)
	require.NotNil(t, event.Timestamp)
	assert.Equal(t, time.Date(2026, 7, 16, 9, 15, 0, 0, time.UTC), event.Timestamp.UTC())
	require.NotNil(t, event.Focus)
	assert.Equal(t, "Task/5f2f9a4e-8c1d-4b6e-9d3a-7c0e2f4b8a1d", *event.Focus)
}

func TestParseBundle_SpecHeartbeat(t *testing.T) {
	notification, err := ParseBundle(parseBundleJSON(t, specHeartbeatExample))
	require.NoError(t, err)
	assert.Equal(t, NotificationTypeHeartbeat, notification.Type)
	assert.Empty(t, notification.Events)
}

func TestBuildBundle_EventRoundTrip(t *testing.T) {
	ts := time.Date(2026, 7, 16, 9, 15, 0, 0, time.UTC)
	original := Notification{
		Type:                  NotificationTypeEvent,
		SubscriptionReference: "https://sender.example/fhir/Subscription/abc",
		Status:                StatusActive,
		Events: []NotificationEvent{{
			EventNumber:  7,
			Timestamp:    &ts,
			Focus:        to.Ptr("Task/j5xw6z3vnfxgk4ttmvwgc3dj"),
			FocusFullURL: to.Ptr("https://sender.example/fhir/Task/j5xw6z3vnfxgk4ttmvwgc3dj"),
		}},
	}

	bundle, err := original.BuildBundle(PayloadModeIDOnly)
	require.NoError(t, err)
	assert.Equal(t, fhir.BundleTypeHistory, bundle.Type)
	require.NotNil(t, bundle.Meta)
	assert.Contains(t, bundle.Meta.Profile, ProfileNotificationR4)
	// id-only: SubscriptionStatus entry plus a focus GET entry
	require.Len(t, bundle.Entry, 2)
	require.NotNil(t, bundle.Entry[1].Request)
	assert.Equal(t, "Task/j5xw6z3vnfxgk4ttmvwgc3dj", bundle.Entry[1].Request.Url)
	assert.Equal(t, "https://sender.example/fhir/Task/j5xw6z3vnfxgk4ttmvwgc3dj", *bundle.Entry[1].FullUrl)

	parsed, err := ParseBundle(bundle)
	require.NoError(t, err)
	assert.Equal(t, original.Type, parsed.Type)
	assert.Equal(t, original.SubscriptionReference, parsed.SubscriptionReference)
	require.Len(t, parsed.Events, 1)
	assert.Equal(t, int64(7), parsed.Events[0].EventNumber)
	assert.Equal(t, "Task/j5xw6z3vnfxgk4ttmvwgc3dj", *parsed.Events[0].Focus)
}

func TestBuildBundle_EmptyModeCarriesNoFocus(t *testing.T) {
	focus := "Task/should-not-appear"
	bundle, err := Notification{
		Type:                  NotificationTypeEvent,
		SubscriptionReference: "Subscription/abc",
		Status:                StatusActive,
		Events:                []NotificationEvent{{EventNumber: 1, Focus: &focus}},
	}.BuildBundle(PayloadModeEmpty)
	require.NoError(t, err)

	// empty mode: single Parameters entry, no focus anywhere in the payload
	require.Len(t, bundle.Entry, 1)
	raw, err := json.Marshal(bundle)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "should-not-appear")

	parsed, err := ParseBundle(bundle)
	require.NoError(t, err)
	require.Len(t, parsed.Events, 1)
	assert.Nil(t, parsed.Events[0].Focus)
	assert.Equal(t, int64(1), parsed.Events[0].EventNumber)
}

func TestStatusParameters_RoundTrip(t *testing.T) {
	t.Run("bare Parameters (TTA v0.6 wording)", func(t *testing.T) {
		params := BuildStatusParameters("https://sender.example/fhir/Subscription/abc", StatusActive, 12)
		data, err := json.Marshal(params)
		require.NoError(t, err)

		parsed, err := ParseStatusParameters(data)
		require.NoError(t, err)
		assert.Equal(t, NotificationTypeQueryStatus, parsed.Type)
		assert.Equal(t, StatusActive, parsed.Status)
		require.Len(t, parsed.Events, 1)
		assert.Equal(t, int64(12), parsed.Events[0].EventNumber)
	})

	t.Run("searchset Bundle (backport IG $status reply)", func(t *testing.T) {
		bundle, err := BuildStatusBundle("https://sender.example/fhir/Subscription/abc", StatusActive, 12)
		require.NoError(t, err)
		assert.Equal(t, fhir.BundleTypeSearchset, bundle.Type)
		data, err := json.Marshal(bundle)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"resourceType":"Bundle"`)

		parsed, err := ParseStatusParameters(data)
		require.NoError(t, err)
		assert.Equal(t, NotificationTypeQueryStatus, parsed.Type)
		require.Len(t, parsed.Events, 1)
		assert.Equal(t, int64(12), parsed.Events[0].EventNumber)
	})

	t.Run("unexpected resource type", func(t *testing.T) {
		_, err := ParseStatusParameters([]byte(`{"resourceType":"OperationOutcome"}`))
		assert.ErrorContains(t, err, "unexpected $status reply")
	})
}

func TestParseBundle_Errors(t *testing.T) {
	t.Run("wrong bundle type", func(t *testing.T) {
		_, err := ParseBundle(fhir.Bundle{Type: fhir.BundleTypeSearchset})
		assert.ErrorContains(t, err, "must be of type history")
	})
	t.Run("no parameters entry", func(t *testing.T) {
		_, err := ParseBundle(fhir.Bundle{Type: fhir.BundleTypeHistory})
		assert.ErrorContains(t, err, "no Parameters")
	})
}
