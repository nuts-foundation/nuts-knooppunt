package main

import (
	"bytes"
	"embed"
	"html/template"
	"log/slog"
	"net/http"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// page is everything a template may read.
type page struct {
	Title      string
	Clients    []client
	Client     *client
	Record     *record
	Form       allergyForm
	Substances []substance
	Statuses   []status
	FormError  string
	Saved      string // "marker" | "plain" | ""
}

// render executes a page into a buffer first, so a template error answers 500
// rather than a half-written page under a success status.
func render(w http.ResponseWriter, status int, name string, data page) {
	t, err := template.ParseFS(templateFS, "templates/base.html", "templates/"+name)
	if err != nil {
		http.Error(w, "parse "+name+": "+err.Error(), http.StatusInternalServerError)
		return
	}
	var body bytes.Buffer
	if err := t.ExecuteTemplate(&body, "base", data); err != nil {
		slog.Error("zonnebloem-ehr: render failed", "template", name, "error", err)
		http.Error(w, "render "+name+": "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body.Bytes())
}
