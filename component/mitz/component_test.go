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
			IsEnabled:     true,
			FHIRBaseURL:   "http://example.com/fhir",
			GatewaySystem: "urn:oid:2.16.840.1.113883.2.4.6.6.1",
			SourceSystem:  "urn:oid:2.16.840.1.113883.2.4.6.6.90000017",
		}

		component, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, component)
		assert.Equal(t, "urn:oid:2.16.840.1.113883.2.4.6.6.1", component.gatewaySystem)
		assert.Equal(t, "urn:oid:2.16.840.1.113883.2.4.6.6.90000017", component.sourceSystem)
	})

	t.Run("missing FHIR base URL", func(t *testing.T) {
		config := Config{
			IsEnabled: true,
		}

		component, err := New(config)
		require.Error(t, err)
		assert.Nil(t, component)
		assert.Contains(t, err.Error(), "FHIR base URL must be configured")
	})

	t.Run("invalid FHIR base URL", func(t *testing.T) {
		config := Config{
			IsEnabled:   true,
			FHIRBaseURL: "://invalid-url",
		}

		component, err := New(config)
		require.Error(t, err)
		assert.Nil(t, component)
		assert.Contains(t, err.Error(), "invalid FHIR base URL")
	})
}

func TestCreateSubscription(t *testing.T) {
	component := &Component{
		gatewaySystem: "urn:oid:2.16.840.1.113883.2.4.6.6.1",
		sourceSystem:  "urn:oid:2.16.840.1.113883.2.4.6.6.90000017",
	}

	req := SubscribeRequest{
		PatientID:        "123456789",
		PatientBirthDate: "2012-03-07",
		ProviderID:       "01234567",
		ProviderType:     "Z3",
		CallbackURL:      "https://example.com/callback",
	}

	subscription := component.createSubscription(req)

	// Verify basic fields
	assert.Equal(t, fhir.SubscriptionStatusRequested, subscription.Status)
	assert.Equal(t, "OTV", subscription.Reason)
	assert.Equal(t, "Consent?_query=otv&patientid=123456789&providerid=01234567&providertype=Z3", subscription.Criteria)

	// Verify channel
	assert.Equal(t, fhir.SubscriptionChannelTypeRestHook, subscription.Channel.Type)
	assert.NotNil(t, subscription.Channel.Endpoint)
	assert.Equal(t, "https://example.com/callback", *subscription.Channel.Endpoint)
	assert.NotNil(t, subscription.Channel.Payload)
	assert.Equal(t, "application/fhir+xml", *subscription.Channel.Payload)

	// Verify extensions
	assert.Len(t, subscription.Extension, 3)

	// Check patient birth date extension
	var foundBirthDate, foundGateway, foundSource bool
	for _, ext := range subscription.Extension {
		switch ext.Url {
		case "http://fhir.nl/StructureDefinition/Patient.birthDate":
			foundBirthDate = true
			assert.NotNil(t, ext.ValueDate)
			assert.Equal(t, "2012-03-07", *ext.ValueDate)
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
	assert.True(t, foundBirthDate, "Patient birth date extension not found")
	assert.True(t, foundGateway, "Gateway system extension not found")
	assert.True(t, foundSource, "Source system extension not found")
}

func TestCreateSubscription_WithoutOptionalFields(t *testing.T) {
	component := &Component{}

	req := SubscribeRequest{
		PatientID:    "123456789",
		ProviderID:   "01234567",
		ProviderType: "Z3",
		CallbackURL:  "https://example.com/callback",
		// PatientBirthDate is omitted
	}

	subscription := component.createSubscription(req)

	// Should have no extensions when optional fields are missing
	assert.Len(t, subscription.Extension, 0)
}

func TestHandleNotify(t *testing.T) {
	component := &Component{}

	t.Run("valid transaction bundle", func(t *testing.T) {
		bundle := fhir.Bundle{
			Type: fhir.BundleTypeTransaction,
		}

		body, err := json.Marshal(bundle)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/mitz/notify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/fhir+json")
		w := httptest.NewRecorder()

		component.handleNotify(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid bundle type", func(t *testing.T) {
		bundle := fhir.Bundle{
			Type: fhir.BundleTypeCollection,
		}

		body, err := json.Marshal(bundle)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/mitz/notify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/fhir+json")
		w := httptest.NewRecorder()

		component.handleNotify(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var outcome fhir.OperationOutcome
		err = json.Unmarshal(w.Body.Bytes(), &outcome)
		require.NoError(t, err)
		assert.NotEmpty(t, outcome.Issue)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/mitz/notify", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/fhir+json")
		w := httptest.NewRecorder()

		component.handleNotify(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestRegisterHttpHandlers(t *testing.T) {
	config := Config{
		IsEnabled:   true,
		FHIRBaseURL: "http://example.com/fhir",
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
