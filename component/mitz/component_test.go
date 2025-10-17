package mitz

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestNew(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		config := Config{
			MitzBase:      "http://example.com",
			GatewaySystem: "urn:oid:2.16.840.1.113883.2.4.6.6.1",
			SourceSystem:  "urn:oid:2.16.840.1.113883.2.4.6.6.90000017",
			ProviderType:  "Z3",
		}

		component, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, component)
		assert.Equal(t, "urn:oid:2.16.840.1.113883.2.4.6.6.1", component.gatewaySystem)
		assert.Equal(t, "urn:oid:2.16.840.1.113883.2.4.6.6.90000017", component.sourceSystem)
		assert.Equal(t, "Z3", component.providerType)
	})

	t.Run("missing mitzbase", func(t *testing.T) {
		config := Config{}

		component, err := New(config)
		require.Error(t, err)
		assert.Nil(t, component)
		assert.Contains(t, err.Error(), "mitzbase must be configured")
	})

	t.Run("invalid mitzbase", func(t *testing.T) {
		config := Config{
			MitzBase: "://invalid-url",
		}

		component, err := New(config)
		require.Error(t, err)
		assert.Nil(t, component)
		assert.Contains(t, err.Error(), "invalid subscription endpoint")
	})
}

func TestCreateSubscription(t *testing.T) {
	component := &Component{
		gatewaySystem:     "urn:oid:2.16.840.1.113883.2.4.6.6.1",
		sourceSystem:      "urn:oid:2.16.840.1.113883.2.4.6.6.90000017",
		providerType:      "Z3",
		notifyCallbackUrl: "https://example.com/callback",
	}

	subscription := component.createSubscription("123456789", "01234567")

	// Verify basic fields
	assert.Equal(t, fhir.SubscriptionStatusRequested, subscription.Status)
	assert.Equal(t, "OTV", subscription.Reason)
	assert.Equal(t, "Consent?_query=otv&patientid=123456789&providerid=01234567&providertype=Z3", subscription.Criteria)

	// Verify channel
	assert.Equal(t, fhir.SubscriptionChannelTypeRestHook, subscription.Channel.Type)
	assert.NotNil(t, subscription.Channel.Endpoint)
	assert.Equal(t, "https://example.com/callback", *subscription.Channel.Endpoint)
	assert.NotNil(t, subscription.Channel.Payload)
	assert.Equal(t, "application/fhir+json", *subscription.Channel.Payload)

	// Verify extensions
	assert.Len(t, subscription.Extension, 2)

	// Check patient birth date extension
	var foundGateway, foundSource bool
	for _, ext := range subscription.Extension {
		switch ext.Url {
		//case "http://fhir.nl/StructureDefinition/Patient.birthDate":
		//	foundBirthDate = true
		//	assert.NotNil(t, ext.ValueDate)
		//	assert.Equal(t, "2012-03-07", *ext.ValueDate)
		case "http://fhir.nl/StructureDefinition/GatewaySystem":
			foundGateway = true
			assert.NotNil(t, ext.ValueOid)
			assert.Equal(t, "urn:oid:2.16.840.1.113883.2.4.6.6.1", *ext.ValueOid)
		case "http://fhir.nl/StructureDefinition/SourceSystem":
			foundSource = true
			assert.NotNil(t, ext.ValueOid)
			assert.Equal(t, "urn:oid:2.16.840.1.113883.2.4.6.6.90000017", *ext.ValueOid)
		}
	}
	assert.True(t, foundGateway, "Gateway system extension not found")
	assert.True(t, foundSource, "Source system extension not found")
}

func TestCreateSubscription_WithoutOptionalFields(t *testing.T) {
	component := &Component{
		notifyCallbackUrl: "https://example.com/callback",
	}

	subscription := component.createSubscription("123456789", "01234567")

	// Should have no extensions when optional fields are missing
	assert.Len(t, subscription.Extension, 0)
}

func TestRegisterHttpHandlers(t *testing.T) {
	config := Config{
		MitzBase: "http://example.com",
	}
	component, err := New(config)
	require.NoError(t, err)

	mux := http.NewServeMux()

	component.RegisterHttpHandlers(nil, mux)

	// Test that handlers are registered
	bundle := fhir.Bundle{Type: fhir.BundleTypeTransaction}
	body, _ := json.Marshal(bundle)
	req := httptest.NewRequest(http.MethodPost, "/mitz/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/fhir+json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Should not be 404
	assert.NotEqual(t, http.StatusNotFound, w.Code)
}

func TestComponentLifecycle(t *testing.T) {
	component := &Component{}

	err := component.Start()
	assert.NoError(t, err)

	err = component.Stop(context.Background())
	assert.NoError(t, err)
}

// Helper function
func toPtr(s string) *string {
	return &s
}
