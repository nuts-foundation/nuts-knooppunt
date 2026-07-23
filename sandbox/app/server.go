package main

import (
	"encoding/json"
	"net/http"
)

var notices = map[string]string{
	"reset-pending": "Reset arrives with the seeded dataset (E5)",
}

// NewMux returns the GF Sandbox HTTP handler. The sandbox is a standalone
// application on its own port; it is not a knooppunt component (DESIGN.md
// standing decision 5).
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.Handle("GET /static/", http.FileServerFS(staticFS))
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := renderPage(w, "landing.html", page{Title: "GF Sandbox", Guise: "shell"}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("GET /demo/login", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		p := page{Title: "Sign in · Plataan EHR", Guise: "ehr", Scenario: scenario, ShowReset: true}
		if err := renderPage(w, "login.html", p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("GET /demo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		p := page{Title: "Demo · GF Sandbox", Guise: "shell", Scenario: scenario, ShowReset: true, Notice: notices[r.URL.Query().Get("notice")]}
		if err := renderPage(w, "demo-start.html", p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("POST /demo/reset", func(w http.ResponseWriter, r *http.Request) {
		// Stub until E5 lands the seeded dataset and real reset semantics.
		http.Redirect(w, r, "/demo?notice=reset-pending", http.StatusSeeOther)
	})
	return mux
}
