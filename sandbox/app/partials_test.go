package main

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func renderPartialForTest(t *testing.T, define string, data page, files ...string) string {
	t.Helper()
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = "templates/" + f
	}
	tpl, err := template.ParseFS(tmplFS, paths...)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(t, tpl.ExecuteTemplate(&buf, define, data))
	return buf.String()
}

func TestSidebarRendersBrandSectionsAndActiveItem(t *testing.T) {
	s := renderPartialForTest(t, "sidebar", page{Active: "dossier"}, "_sidebar.html")
	require.Contains(t, s, "Plataan<span>.</span>ehr")
	require.Contains(t, s, "Ziekenhuis De Plataan")
	for _, label := range []string{"Care", "Administration", "Schedule", "Patient record", "Patient registration", "Orders", "Correspondence"} {
		require.Contains(t, s, label)
	}
	require.Equal(t, 1, strings.Count(s, "nav-item active"), "exactly one active item")
	require.Contains(t, s, "Test environment")
}

func TestTopbarSessionSlot(t *testing.T) {
	anon := renderPartialForTest(t, "topbar", page{TopTitle: "Home"}, "_topbar.html")
	require.Contains(t, anon, "<h2>Home</h2>")
	require.Contains(t, anon, "Not signed in")
	require.NotContains(t, anon, "Dezi ✓")

	signed := renderPartialForTest(t, "topbar", page{TopTitle: "Home", Session: &Session{
		Initials: "SA", Name: "S. el Amrani", Description: "Klinisch geriater · UZI 900001234 · Ziekenhuis De Plataan",
	}}, "_topbar.html")
	require.Contains(t, signed, "S. el Amrani")
	require.Contains(t, signed, "Dezi ✓")
}

func TestTopbarSignOutControlOnlyWhenSignedIn(t *testing.T) {
	anon := renderPartialForTest(t, "topbar", page{TopTitle: "Home"}, "_topbar.html")
	require.NotContains(t, anon, `action="/demo/logout"`, "no sign-out control when not signed in")

	signed := renderPartialForTest(t, "topbar", page{TopTitle: "Home", Session: &Session{
		Initials: "SA", Name: "S. el Amrani", Description: "Klinisch geriater · UZI 900001234 · Ziekenhuis De Plataan",
	}}, "_topbar.html")
	require.Contains(t, signed, `action="/demo/logout"`, "a sign-out control must be reachable when signed in")
	require.Contains(t, signed, "Sign out")
}
