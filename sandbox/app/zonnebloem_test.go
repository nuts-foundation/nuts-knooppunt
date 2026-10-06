package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecord_LinksToZonnebloemsEHRInANewTab(t *testing.T) {
	cfg, _, _ := fakeConfig()
	cfg.ZonnebloemEHRURL = "http://localhost:3001"
	srv, client := demoServer(t, cfg)

	status, body := getBody(t, client, srv, "/demo/ehr/patients/anna")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Proof it really works")
	require.Contains(t, body, `href="http://localhost:3001" target="_blank" rel="noopener"`)
}

func TestRecord_NoZonnebloemCardWithoutAURL(t *testing.T) {
	cfg, _, _ := fakeConfig()
	srv, client := demoServer(t, cfg)

	status, body := getBody(t, client, srv, "/demo/ehr/patients/anna")

	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "<h2>Patient record</h2>", "the record page itself rendered")
	require.NotContains(t, body, "Proof it really works")
}

func TestNewConfigFromEnv_ReadsTheZonnebloemEHRURL(t *testing.T) {
	cfg, err := NewConfigFromEnv(envOf(map[string]string{"ZONNEBLOEM_EHR_PUBLIC_URL": "http://localhost:3001"}))

	require.NoError(t, err)
	require.Equal(t, "http://localhost:3001", cfg.ZonnebloemEHRURL)
}

// Read before the Knooppunt settings, which end configuration early when unset:
// the link has nothing to do with the Knooppunt.
func TestNewConfigFromEnv_RejectsAZonnebloemEHRURLThatIsNotAbsolute(t *testing.T) {
	// "javascript:alert(1)" and "ftp://zonnebloem" only fail the scheme check,
	// "http:///zonnebloem" only the host check.
	for _, raw := range []string{"localhost:3001", "/zonnebloem", "javascript:alert(1)", "ftp://zonnebloem", "http:///zonnebloem"} {
		_, err := NewConfigFromEnv(envOf(map[string]string{"ZONNEBLOEM_EHR_PUBLIC_URL": raw}))
		require.ErrorContainsf(t, err, "ZONNEBLOEM_EHR_PUBLIC_URL", "%q must be rejected", raw)
	}
}

// Without a store De Zonnebloem's EHR has nothing to show, so gf-sandbox does
// not serve it.
func TestNewConfigFromEnv_LeavesZonnebloemsEHROffWithoutAStore(t *testing.T) {
	cfg, err := NewConfigFromEnv(envOf(map[string]string{"ZONNEBLOEM_EHR_PORT": "4000"}))

	require.NoError(t, err)
	require.Nil(t, cfg.zonnebloemFHIRBaseURL)
}

// Read before the Knooppunt settings, like the link. PORT is the sandbox's own
// listener, so it must not move the EHR.
func TestNewConfigFromEnv_ReadsZonnebloemsStoreAndDefaultsTheEHRPort(t *testing.T) {
	cfg, err := NewConfigFromEnv(envOf(map[string]string{
		"ZONNEBLOEM_FHIR_BASE_URL": "http://hapi-fhir:7050/fhir/sunflower-patients", "PORT": "9000",
	}))

	require.NoError(t, err)
	require.NotNil(t, cfg.zonnebloemFHIRBaseURL)
	require.Equal(t, "http://hapi-fhir:7050/fhir/sunflower-patients", cfg.zonnebloemFHIRBaseURL.String())
	require.Equal(t, "3001", cfg.zonnebloemEHRPort)
}

func TestNewConfigFromEnv_HonoursZONNEBLOEM_EHR_PORT(t *testing.T) {
	cfg, err := NewConfigFromEnv(envOf(map[string]string{
		"ZONNEBLOEM_FHIR_BASE_URL": "http://hapi-fhir:7050/fhir/sunflower-patients", "ZONNEBLOEM_EHR_PORT": "4000",
	}))

	require.NoError(t, err)
	require.Equal(t, "4000", cfg.zonnebloemEHRPort)
}

func TestNewConfigFromEnv_RejectsAZonnebloemStoreURLThatIsNotAbsolute(t *testing.T) {
	// "ftp://hapi/fhir" only fails the scheme check, "http:///fhir" only the
	// host check.
	for _, raw := range []string{"hapi-fhir:7050/fhir/sunflower-patients", "/fhir/sunflower-patients", "ftp://hapi/fhir", "http:///fhir"} {
		_, err := NewConfigFromEnv(envOf(map[string]string{"ZONNEBLOEM_FHIR_BASE_URL": raw}))
		require.ErrorContainsf(t, err, "ZONNEBLOEM_FHIR_BASE_URL", "%q must be rejected", raw)
	}
}
