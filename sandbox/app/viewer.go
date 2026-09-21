package main

import (
	"fmt"
	"html/template"
)

// viewerReserveBreakpoint is the viewport width from which the layout can spare
// the dock its 574px, and therefore also the width from which the dock may open
// by default. viewer.css reserves the space at exactly this width; the two are
// held together by TestViewerBreakpointMatchesTheStylesheet.
const viewerReserveBreakpoint = "1400"

// viewerBodyAttrs returns the Datastar signal wiring for the GF viewer on
// <body>. Trusted constant markup, no user data flows in.
//
// The open default is conditioned on the viewport rather than fixed, because
// the dock is 560px wide: below the breakpoint it would cover most of the
// screen it was supposed to annotate. Expressing that here keeps Datastar to
// data-* attributes, which the frontend contract requires, and keeps the
// decision out of static JS. A screen that asks for it closed stays closed at
// every width.
func viewerBodyAttrs(open bool) template.HTMLAttr {
	openExpr := "false"
	if open {
		openExpr = "window.innerWidth>=" + viewerReserveBreakpoint
	}
	return template.HTMLAttr(fmt.Sprintf(
		`data-signals='{"hood":{"open":%s,"tech":false}}' data-class='{"hood-open":$hood.open,"hood-tech":$hood.tech}'`, openExpr))
}
