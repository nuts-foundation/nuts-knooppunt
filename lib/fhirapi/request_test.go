package fhirapi

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestReadRequest(t *testing.T) {
	t.Run("valid FHIR request", func(t *testing.T) {
		httpRequest, err := http.NewRequest(http.MethodGet, "http://localhost", bytes.NewReader([]byte(`{"id":"123"}`)))
		require.NoError(t, err)
		httpRequest.Header.Set("Content-Type", "application/fhir+json")

		fhirRequest, err := ReadRequest[fhir.Task](httpRequest)

		require.NoError(t, err)
		require.NotNil(t, fhirRequest)
		require.Equal(t, "123", *fhirRequest.Resource.Id)
	})
	t.Run("invalid Content-Type", func(t *testing.T) {
		httpRequest, err := http.NewRequest(http.MethodGet, "http://localhost", bytes.NewReader([]byte(`{"id":"123"}`)))
		require.NoError(t, err)
		httpRequest.Header.Set("Content-Type", "application/json")

		fhirRequest, err := ReadRequest[fhir.Task](httpRequest)

		require.EqualError(t, err, "invalid content type, expected application/fhir+json")
		require.Nil(t, fhirRequest)
	})
	t.Run("Content-Type with parameters", func(t *testing.T) {
		httpRequest, err := http.NewRequest(http.MethodGet, "http://localhost", bytes.NewReader([]byte(`{"id":"123"}`)))
		require.NoError(t, err)
		httpRequest.Header.Set("Content-Type", "application/fhir+json; charset=utf-8")

		fhirRequest, err := ReadRequest[fhir.Task](httpRequest)

		require.NoError(t, err)
		require.NotNil(t, fhirRequest)
		require.Equal(t, "123", *fhirRequest.Resource.Id)
	})
}
