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

	_, body := getBody(t, client, srv, "/demo/ehr/patients/anna")

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
