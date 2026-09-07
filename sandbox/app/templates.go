package main

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"strings"
)

//go:embed all:templates
var tmplFS embed.FS

const scenario = "Scenario: a specialist retrieves the patient summary (BGZ) from elderly care (indexed pull)"

// Session holds the Dezi practitioner claims shown in the top bar.
// E2 (GF Authentication) produces it; nil renders the not-signed-in state.
type Session struct {
	Initials    string
	Name        string
	Description string
}

type page struct {
	Title      string
	Guise      string // "shell" | "ehr"
	BodyClass  string
	BodyAttrs  template.HTMLAttr
	Scenario   string
	ShowReset  bool
	Notice     string
	Session    *Session
	Active     string
	TopTitle   string
	Alert      bool
	ViewerOpen bool
	Claims     []claim
	Patients   []patientRow
	Patient    *patientRow
}

// claim is one introspected value, rendered on the authorization page.
type claim struct {
	Name  string
	Value string
}

func (p page) Stylesheets() []string {
	base := []string{"fonts", "tokens", "base", "shell"}
	if p.Guise == "ehr" {
		return append(base, "ehr", "viewer")
	}
	return base
}

var partials []string

func init() {
	entries, err := tmplFS.ReadDir("templates")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "_") {
			partials = append(partials, "templates/"+e.Name())
		}
	}
}

func renderPage(w io.Writer, pageFile string, data page) error {
	files := append([]string{"templates/base.html", "templates/" + pageFile}, partials...)
	t, err := template.ParseFS(tmplFS, files...)
	if err != nil {
		return fmt.Errorf("parse %s: %w", pageFile, err)
	}
	return t.ExecuteTemplate(w, "base", data)
}
