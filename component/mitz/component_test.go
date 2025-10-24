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
		}

		component, err := New(config)
		require.NoError(t, err)
		require.NotNil(t, component)
		assert.Equal(t, "urn:oid:2.16.840.1.113883.2.4.6.6.1", component.gatewaySystem)
		assert.Equal(t, "urn:oid:2.16.840.1.113883.2.4.6.6.90000017", component.sourceSystem)
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
		assert.Contains(t, err.Error(), "invalid mitzbase URL")
	})
}

func TestRegisterHttpHandlers(t *testing.T) {
	config := Config{
		MitzBase: "http://example.com",
	}
	component, err := New(config)
	require.NoError(t, err)

	publicMux := http.NewServeMux()
	internalMux := http.NewServeMux()

	component.RegisterHttpHandlers(publicMux, internalMux)

	// Test that notify handler is registered on publicMux
	bundle := fhir.Bundle{Type: fhir.BundleTypeTransaction}
	body, _ := json.Marshal(bundle)
	req := httptest.NewRequest(http.MethodPost, "/mitz/notify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/fhir+json")
	w := httptest.NewRecorder()
	publicMux.ServeHTTP(w, req)
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
