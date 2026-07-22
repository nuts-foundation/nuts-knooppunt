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

func TestResetStubRedirectsWithNotice(t *testing.T) {
	srv := httptest.NewServer(NewMux())
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Post(srv.URL+"/demo/reset", "application/x-www-form-urlencoded", nil)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusSeeOther, res.StatusCode)
	require.Equal(t, "/demo?notice=reset-pending", res.Header.Get("Location"))

	_, body := getPage(t, "/demo?notice=reset-pending")
	require.Contains(t, body, "Reset arrives with the seeded dataset (E5)")
}
