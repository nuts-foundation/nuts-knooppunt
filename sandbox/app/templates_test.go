package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLandingPageOffersDemoAndDisabledConnect(t *testing.T) {
	status, body := getPage(t, "/")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "<!doctype html>")
	require.Contains(t, body, "GF Sandbox")
	require.Contains(t, body, `href="/demo"`)
	require.Contains(t, body, "Connect")
	require.Contains(t, body, `aria-disabled="true"`, "Connect must be visibly disabled")
	require.NotContains(t, body, `href="/connect"`, "Connect must not link anywhere yet")
	require.Contains(t, body, "/static/vendor/datastar.js")
	require.Contains(t, body, "synthetic")
}

func TestVendoredDatastarIsServed(t *testing.T) {
	status, js := getPage(t, "/static/vendor/datastar.js")
	require.Equal(t, http.StatusOK, status)
	require.Greater(t, len(js), 10_000, "bundle looks truncated")
}

func TestDemoStartIntroducesScenario(t *testing.T) {
	status, body := getPage(t, "/demo")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, "Scenario: a specialist retrieves the patient summary (BGZ) from elderly care (indexed pull)")
	require.Contains(t, body, `href="/demo/login"`)
	require.Contains(t, body, "Reset", "demo bar must carry the reset control")
}

func TestResetWithoutBackendReportsDisabled(t *testing.T) {
	// With no Knooppunt/HAPI wired (the default test config), reset is disabled
	// and redirects with an explanatory notice rather than erroring.
	srv := httptest.NewServer(NewMux(testConfig()))
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Post(srv.URL+"/demo/reset", "application/x-www-form-urlencoded", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=reset-disabled", res.Header.Get("Location"))

	_, body := getPage(t, "/demo?notice=reset-disabled")
	require.Contains(t, body, "Reset is unavailable")
}

func TestLoginScreenMatchesWireframeCopy(t *testing.T) {
	status, body := getPage(t, "/demo/login")
	require.Equal(t, http.StatusOK, status)
	for _, s := range []string{
		"Welcome to the", "Plataan EHR portal",
		"Sign in with your Dezi wallet to continue.",
		"Sign in with Dezi",
		"Sign in with UZI card · not available in this demo",
		"Simulated login (GF Authentication) · test environment with synthetic data only",
		"One overview, wherever the data lives.",
		"All people and data are synthetic.",
		// The organization is named as the attestation spells it. Without this
		// the login brand could drift back to an English translation and the
		// copy test would still pass.
		"Ziekenhuis De Plataan",
	} {
		require.Contains(t, body, s)
	}
}

func TestEhrHomeShowsFullChrome(t *testing.T) {
	// E2 protects /demo/ehr (sandbox/DESIGN.md section 4), so reaching it now
	// requires a real sign-in first; anonymous access is covered separately by
	// TestEhrRedirectsWhenNotSignedIn.
	dezi := fakeDezi(t)
	t.Setenv("DEZI_INTERNAL_BASE_URL", dezi.URL)
	srv := httptest.NewServer(NewMux(testConfig()))
	t.Cleanup(srv.Close)
	client := signInViaDezi(t, srv)

	status, body := getPageWithClient(t, client, srv.URL+"/demo/ehr")
	require.Equal(t, http.StatusOK, status)
	for _, s := range []string{"sb-bar", `class="app"`, `class="side"`, `class="top"`, "hood-dock", "gf-tab", `id="gf-viewer-steps"`, "/static/js/journey-strip.js"} {
		require.Contains(t, body, s)
	}
	require.Contains(t, body, "S. el Amrani", "the signed-in practitioner's name renders in the top bar")
	require.Contains(t, body, "Dezi ✓", "the top bar shows the signed-in badge")
	require.Contains(t, body, `class="hood-dock on"`, "viewer opens by default past login")
	require.Contains(t, body, "hood-open", "body binds the hood-open class")
}

func TestLoginPostsRatherThanLinks(t *testing.T) {
	status, body := getPage(t, "/demo/login")
	require.Equal(t, http.StatusOK, status)
	require.Contains(t, body, `action="/demo/login"`)
	require.Contains(t, body, `method="post"`)
	require.NotContains(t, body, `href="/demo/ehr"`,
		"signing in must create a session, not link past it")
}
