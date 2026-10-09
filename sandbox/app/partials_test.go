package main

import (
	"bytes"
	"html/template"
	"regexp"
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

// Patient record is the way back to the patient list from every EHR screen. An
// anchor, not any element with an href: only an anchor is followed on a click or
// reached with the keyboard.
func TestSidebarPatientRecordLinksToThePatientList(t *testing.T) {
	s := renderPartialForTest(t, "sidebar", page{Active: "dossier"}, "_sidebar.html")
	link := regexp.MustCompile(`(?s)<a [^>]*href="/demo/ehr"[^>]*>(.*?)</a>`).FindStringSubmatch(s)
	require.NotNil(t, link, "the sidebar must link to the patient list")
	require.Contains(t, link[1], "Patient record", "the link must be the Patient record item")
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
	require.Contains(t, signed, `class="session-details"`)
	require.Contains(t, signed, `class="session-description"`)
	require.Contains(t, signed, "Klinisch geriater · UZI 900001234 · Ziekenhuis De Plataan")
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
