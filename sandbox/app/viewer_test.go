package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestViewerShellStructure(t *testing.T) {
	open := renderPartialForTest(t, "viewer", page{ViewerOpen: true}, "_viewer.html", "_journey_svg.html")
	for _, s := range []string{"hood-dock", "gf-tab", "GF Viewer", "dock-map", "Functional", "Technical", `id="gf-viewer-steps"`, "Under the hood", "journey-strip.js"} {
		require.Contains(t, open, s)
	}
	require.Contains(t, open, `class="hood-dock on"`)
	collapsed := renderPartialForTest(t, "viewer", page{ViewerOpen: false}, "_viewer.html", "_journey_svg.html")
	require.Contains(t, collapsed, `class="hood-dock peek"`)
}

func TestViewerBodyAttrsCarryHoodSignals(t *testing.T) {
	attrs := string(viewerBodyAttrs(true))
	require.Contains(t, attrs, "data-signals")
	require.Contains(t, attrs, "hood-open")
	require.Contains(t, attrs, `"open":true`)
}

func TestJourneyModelCoversAllGfLabels(t *testing.T) {
	js, err := staticFS.ReadFile("static/js/journey-model.js")
	require.NoError(t, err)
	for _, gf := range []string{"pseudonym", "localization", "addressing", "authentication", "consent", "authorization", "exchange"} {
		require.True(t, strings.Contains(string(js), gf+":"), "journey-model.js must map gf label %q", gf)
	}
}
