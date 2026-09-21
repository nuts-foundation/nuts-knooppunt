package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestViewerShellStructure(t *testing.T) {
	open := renderPartialForTest(t, "viewer", page{ViewerOpen: true}, "_viewer.html", "_journey_svg.html")
	for _, s := range []string{"hood-dock", "gf-tab", "GF Viewer", "dock-map", "Functional", "Technical", `id="gf-viewer-steps"`, "Under the hood", "step-events.js", `id="gf-pause"`, `id="gf-replay"`, `id="gf-skip"`} {
		require.Contains(t, open, s)
	}
	require.Contains(t, open, `class="hood-dock on"`)
	require.Contains(t, open, `data-on:click="$hood.open = !$hood.open"`)
	require.Contains(t, open, `aria-controls="hood-content"`)
	require.Contains(t, open, `data-attr:aria-expanded="$hood.open ? 'true' : 'false'"`)
	require.Contains(t, open, `aria-label="Close GF viewer"`)
	require.Contains(t, open, `data-attr:inert="!$hood.open"`)
	collapsed := renderPartialForTest(t, "viewer", page{ViewerOpen: false}, "_viewer.html", "_journey_svg.html")
	require.Contains(t, collapsed, `class="hood-dock peek"`)
}

func TestViewerBodyAttrsCarryHoodSignals(t *testing.T) {
	attrs := string(viewerBodyAttrs(true))
	require.Contains(t, attrs, "data-signals")
	require.Contains(t, attrs, "hood-open")
}

// The dock is 560px and reserves 574px beside the content. Opening it by default
// on a narrow screen leaves less room for the record than one card needs, so the
// default is conditioned on the same width at which viewer.css starts reserving
// that room. Below it the dock stays a tab the presenter can still open.
func TestViewerBodyAttrs_DefaultsOpenOnlyOnAWideViewport(t *testing.T) {
	open := string(viewerBodyAttrs(true))
	require.Contains(t, open, "window.innerWidth>="+viewerReserveBreakpoint)

	// Screens that ask for it closed stay closed at every width.
	closed := string(viewerBodyAttrs(false))
	require.Contains(t, closed, `"open":false`)
	require.NotContains(t, closed, "window.innerWidth")
}

// The breakpoint has to be the same on both sides: the CSS reserves the space
// and the signal decides whether to occupy it. If they drift, one width opens a
// dock the layout has not made room for.
func TestViewerBreakpointMatchesTheStylesheet(t *testing.T) {
	css, err := staticFS.ReadFile("static/css/viewer.css")
	require.NoError(t, err)
	require.Contains(t, string(css), "@media (min-width: "+viewerReserveBreakpoint+"px)")
}

func TestJourneyModelCoversAllGfLabels(t *testing.T) {
	js, err := staticFS.ReadFile("static/js/journey-model.js")
	require.NoError(t, err)
	for _, gf := range []string{"pseudonym", "localization", "addressing", "authentication", "consent", "authorization", "exchange"} {
		require.True(t, strings.Contains(string(js), "case '"+gf+"':"), "journey-model.js must map gf label %q", gf)
	}
}
