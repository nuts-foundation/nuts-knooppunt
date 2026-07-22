package main

import (
	"net/http"
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
