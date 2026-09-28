package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func envOf(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestConfigFromEnv_RequiresTheFHIRBaseURL(t *testing.T) {
	_, err := configFromEnv(envOf(nil))

	require.ErrorContains(t, err, "ZONNEBLOEM_FHIR_BASE_URL")
}

// "ftp://hapi/fhir" only fails the scheme check and "http:///fhir" only the host
// check, so each condition is pinned on its own.
func TestConfigFromEnv_RejectsAURLWithoutAnHTTPScheme(t *testing.T) {
	for _, raw := range []string{"hapi-fhir:7050/fhir/sunflower-patients", "/fhir/sunflower-patients", "ftp://hapi/fhir", "http:///fhir"} {
		_, err := configFromEnv(envOf(map[string]string{"ZONNEBLOEM_FHIR_BASE_URL": raw}))
		require.Errorf(t, err, "%q must be rejected", raw)
	}
}

func TestConfigFromEnv_ReadsTheStoreAndDefaultsThePort(t *testing.T) {
	cfg, err := configFromEnv(envOf(map[string]string{
		"ZONNEBLOEM_FHIR_BASE_URL": "http://hapi-fhir:7050/fhir/sunflower-patients",
	}))

	require.NoError(t, err)
	require.Equal(t, "3001", cfg.port)
	require.Equal(t, "http://hapi-fhir:7050/fhir/sunflower-patients", cfg.fhirBaseURL.String())
}

func TestConfigFromEnv_HonoursPORT(t *testing.T) {
	cfg, err := configFromEnv(envOf(map[string]string{
		"ZONNEBLOEM_FHIR_BASE_URL": "http://hapi-fhir:7050/fhir/sunflower-patients", "PORT": "4000",
	}))

	require.NoError(t, err)
	require.Equal(t, "4000", cfg.port)
}
