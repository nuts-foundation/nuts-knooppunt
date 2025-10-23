package mitz

import (
	"net/http"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Test_MITZSubscription(t *testing.T) {
	// Start minimal MITZ harness
	mitzDetail := harness.StartMITZ(t)
	mockMITZ := mitzDetail.MockMITZ

	t.Run("create subscription via MITZ component", func(t *testing.T) {
		// Create FHIR client to communicate with Knooppunt's MITZ endpoint
		mitzClient := fhirclient.New(mitzDetail.KnooppuntInternalBaseURL.JoinPath("mitz"), http.DefaultClient, nil)

		// Create a valid MITZ subscription
		subscription := fhir.Subscription{
			Status:   fhir.SubscriptionStatusRequested,
			Reason:   "OTV",
			Criteria: "Consent?_query=otv&patientid=900186021&providerid=123456&providertype=provider",
			Channel: fhir.SubscriptionChannel{
				Type:    fhir.SubscriptionChannelTypeRestHook,
				Payload: to.Ptr("application/fhir+json"),
				// Note: Endpoint is intentionally not provided to test fallback to configuration
			},
		}

		// Send subscription to Knooppunt
		var created fhir.Subscription
		err := mitzClient.Create(subscription, &created)
		require.NoError(t, err, "failed to create subscription")

		t.Run("verify subscription was forwarded to mock MITZ", func(t *testing.T) {
			// Check that the subscription was received by the mock MITZ server
			subscriptions := mockMITZ.GetSubscriptions()
			require.Len(t, subscriptions, 1, "mock MITZ should have received exactly 1 subscription")

			capturedSub := subscriptions[0]

			// Verify basic subscription properties
			assert.Equal(t, fhir.SubscriptionStatusRequested, capturedSub.Status)
			assert.Equal(t, "OTV", capturedSub.Reason)
			assert.Contains(t, capturedSub.Criteria, "Consent?_query=otv")
			assert.Contains(t, capturedSub.Criteria, "patientid=900186021")
			assert.Contains(t, capturedSub.Criteria, "providerid=123456")
			assert.Contains(t, capturedSub.Criteria, "providertype=provider")

			// Verify channel properties
			assert.Equal(t, fhir.SubscriptionChannelTypeRestHook, capturedSub.Channel.Type)
			require.NotNil(t, capturedSub.Channel.Payload)
			assert.Equal(t, "application/fhir+json", *capturedSub.Channel.Payload)

			// Verify endpoint was set from configuration
			require.NotNil(t, capturedSub.Channel.Endpoint)
			assert.Equal(t, "http://localhost:8080/consent/notify", *capturedSub.Channel.Endpoint)

			t.Run("verify extensions from configuration", func(t *testing.T) {
				// Check that gateway system extension was added
				var foundGatewayExt bool
				var foundSourceExt bool

				for _, ext := range capturedSub.Extension {
					if ext.Url == "http://fhir.nl/StructureDefinition/GatewaySystem" {
						foundGatewayExt = true
						require.NotNil(t, ext.ValueOid)
						assert.Equal(t, "test-gateway", *ext.ValueOid)
					}
					if ext.Url == "http://fhir.nl/StructureDefinition/SourceSystem" {
						foundSourceExt = true
						require.NotNil(t, ext.ValueOid)
						assert.Equal(t, "test-source", *ext.ValueOid)
					}
				}

				assert.True(t, foundGatewayExt, "GatewaySystem extension should be present")
				assert.True(t, foundSourceExt, "SourceSystem extension should be present")
			})
		})
	})

	t.Run("create subscription with provided endpoint overrides config", func(t *testing.T) {
		mitzClient := fhirclient.New(mitzDetail.KnooppuntInternalBaseURL.JoinPath("mitz"), http.DefaultClient, nil)

		// Create subscription with explicit endpoint (should override config)
		subscription := fhir.Subscription{
			Status:   fhir.SubscriptionStatusRequested,
			Reason:   "OTV",
			Criteria: "Consent?_query=otv&patientid=900186021&providerid=654321&providertype=provider",
			Channel: fhir.SubscriptionChannel{
				Type:     fhir.SubscriptionChannelTypeRestHook,
				Payload:  to.Ptr("application/fhir+json"),
				Endpoint: to.Ptr("https://custom-endpoint.example.com/notify"),
			},
		}

		var created fhir.Subscription
		err := mitzClient.Create(subscription, &created)
		require.NoError(t, err, "failed to create subscription with custom endpoint")

		t.Run("verify custom endpoint is preserved", func(t *testing.T) {
			subscriptions := mockMITZ.GetSubscriptions()
			require.Len(t, subscriptions, 2, "mock MITZ should have received 2 subscriptions total")

			capturedSub := subscriptions[1]

			// Verify the provided endpoint was preserved
			require.NotNil(t, capturedSub.Channel.Endpoint)
			assert.Equal(t, "https://custom-endpoint.example.com/notify", *capturedSub.Channel.Endpoint)
		})
	})
}
