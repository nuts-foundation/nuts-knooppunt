package main

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDesignTokensMatchWireframePalette(t *testing.T) {
	status, css := getPage(t, "/static/css/tokens.css")
	require.Equal(t, http.StatusOK, status)
	for _, token := range []string{"--rv-blauw: #154273", "--teal: #0f766e", "--bg: #f7f5f1", "--f-serif:"} {
		require.Contains(t, css, token)
	}
}

func TestFontsAreVendored(t *testing.T) {
	status, css := getPage(t, "/static/css/fonts.css")
	require.Equal(t, http.StatusOK, status)
	require.NotContains(t, css, "googleapis.com")
	require.NotContains(t, css, "gstatic.com")
	for _, fam := range []string{"'Fraunces'", "'Inter'", "'Fira Sans'", "'Fira Mono'"} {
		require.Contains(t, css, "font-family: "+fam)
	}
	m := regexp.MustCompile(`url\('([^']+)'\)`).FindStringSubmatch(css)
	require.NotNil(t, m, "no font url found in fonts.css")
	require.True(t, strings.HasPrefix(m[1], "/static/fonts/"), "font urls must be local, got %s", m[1])
	fontStatus, _ := getPage(t, m[1])
	require.Equal(t, http.StatusOK, fontStatus, "vendored font %s not served", m[1])
}
