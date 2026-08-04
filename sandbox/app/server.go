package main

import (
	"encoding/json"
	"net/http"
)

var notices = map[string]string{
	"reset-pending": "Reset arrives with the seeded dataset (E5)",
	"signed-out":    "Signed out. The Dezi session has been cleared.",
}

// NewMux returns the GF Sandbox HTTP handler. The sandbox is a standalone
// application on its own port; it is not a knooppunt component (DESIGN.md
// standing decision 5).
func NewMux() *http.ServeMux {
	sessions := newSessionStore()
	client := newDeziClient(deziConfigFromEnv())
	pending := newPendingStore()
	secure := secureCookies()

	// signedIn resolves the session cookie, returning nil when absent or expired.
	signedIn := func(r *http.Request) *authSession {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			return nil
		}
		session, ok := sessions.get(cookie.Value)
		if !ok {
			return nil
		}
		return &session
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	})
	mux.Handle("GET /static/", http.FileServerFS(staticFS))

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		render(w, "landing.html", page{Title: "GF Sandbox", Guise: "shell"})
	})

	mux.HandleFunc("GET /demo", func(w http.ResponseWriter, r *http.Request) {
		render(w, "demo-start.html", page{
			Title: "Demo · GF Sandbox", Guise: "shell", Scenario: scenario,
			ShowReset: true, Notice: notices[r.URL.Query().Get("notice")],
		})
	})

	mux.HandleFunc("GET /demo/login", func(w http.ResponseWriter, r *http.Request) {
		if signedIn(r) != nil {
			http.Redirect(w, r, "/demo/ehr", http.StatusSeeOther)
			return
		}
		render(w, "login.html", page{
			Title: "Sign in · Plataan EHR", Guise: "ehr", Scenario: scenario, ShowReset: true,
		})
	})

	mux.HandleFunc("POST /demo/login", func(w http.ResponseWriter, r *http.Request) {
		state, verifier := randomURLSafe(), randomURLSafe()
		pending.put(state, verifier)
		http.Redirect(w, r, client.authorizeURL(state, verifier), http.StatusSeeOther)
	})

	mux.HandleFunc("GET /demo/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if oauthErr := q.Get("error"); oauthErr != "" {
			http.Error(w, "Dezi returned an error: "+oauthErr, http.StatusBadRequest)
			return
		}

		// State is consumed exactly once, so a replayed callback fails.
		attempt, ok := pending.take(q.Get("state"))
		if !ok {
			http.Error(w, "Unknown or expired sign-in attempt", http.StatusBadRequest)
			return
		}

		session, err := client.exchange(r.Context(), q.Get("code"), attempt.verifier)
		if err != nil {
			http.Error(w, "Sign-in failed", http.StatusBadGateway)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookie,
			Value:    sessions.create(session),
			Path:     "/",
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/demo/ehr", http.StatusSeeOther)
	})

	mux.HandleFunc("POST /demo/logout", func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookie); err == nil {
			sessions.drop(cookie.Value)
		}
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: secure, MaxAge: -1,
		})
		http.Redirect(w, r, "/demo?notice=signed-out", http.StatusSeeOther)
	})

	mux.HandleFunc("POST /demo/reset", func(w http.ResponseWriter, r *http.Request) {
		if crossSiteRequest(r) {
			http.Error(w, "cross-site reset is not allowed", http.StatusForbidden)
			return
		}
		// Stub until E5 lands the seeded dataset and real reset semantics, but
		// the session half of reset is real from here on.
		sessions.dropAll()
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: secure, MaxAge: -1,
		})
		http.Redirect(w, r, "/demo?notice=reset-pending", http.StatusSeeOther)
	})

	mux.HandleFunc("GET /demo/ehr", requireSession(signedIn, func(w http.ResponseWriter, r *http.Request, session *authSession) {
		view := session.view()
		render(w, "ehr-home.html", page{
			Title: "Home · Plataan EHR", Guise: "ehr",
			Scenario: scenario, ShowReset: true,
			BodyClass: "hood-open", BodyAttrs: viewerBodyAttrs(true),
			Active: "dossier", TopTitle: "Home", ViewerOpen: true,
			Session: &view,
		})
	}))

	return mux
}

// requireSession wraps a handler that needs a signed-in practitioner. It
// resolves the session via resolve and redirects to /demo/login when absent,
// so every route under /demo/ehr can share one guard instead of each new
// screen having to remember to check.
func requireSession(resolve func(*http.Request) *authSession, next func(http.ResponseWriter, *http.Request, *authSession)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session := resolve(r)
		if session == nil {
			http.Redirect(w, r, "/demo/login", http.StatusSeeOther)
			return
		}
		next(w, r, session)
	}
}

// crossSiteRequest reports whether r shows browser evidence of originating
// from another site. The server listens on all interfaces and /demo/reset
// carries no CSRF token, so this header is the only guard between an
// arbitrary web page and signing out every practitioner in the demo.
//
// Every modern browser sets Sec-Fetch-Site on navigations and form
// submissions, and a page cannot override it, so a value other than
// same-origin/same-site is a reliable positive signal. The header's absence
// is not a reliable negative signal, though: non-browser clients (curl,
// this repo's tests, the demo's own tooling) never send it, so an absent
// header must stay allowed. That limits this guard to stopping
// browser-driven cross-site requests; it is not a substitute for a real
// CSRF token if this endpoint ever needs one.
func crossSiteRequest(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	return site != "" && site != "same-origin" && site != "same-site"
}

func render(w http.ResponseWriter, template string, data page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := renderPage(w, template, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
