package main

import (
	"fmt"
	"html/template"
)

// viewerBodyAttrs returns the Datastar signal wiring for the GF viewer on
// <body>. Trusted constant markup — no user data flows in.
func viewerBodyAttrs(open bool) template.HTMLAttr {
	return template.HTMLAttr(fmt.Sprintf(
		`data-signals='{"hood":{"open":%t,"tech":false}}' data-class='{"hood-open":$hood.open,"hood-tech":$hood.tech}'`, open))
}
