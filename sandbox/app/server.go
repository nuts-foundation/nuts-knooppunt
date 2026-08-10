package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
	nuts := newNutsClient(nutsConfigFromEnv())
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

		// Checked before the state is consumed, so a truncated callback URL
		// does not burn the sign-in attempt and force a restart. Without this,
		// a missing code reaches /token and comes back as a 502, reporting a
		// client mistake as a Dezi failure.
		code := q.Get("code")
		if code == "" {
			http.Error(w, "Callback is missing the authorization code", http.StatusBadRequest)
			return
		}

		// State is consumed exactly once, so a replayed callback fails.
		attempt, ok := pending.take(q.Get("state"))
		if !ok {
			http.Error(w, "Unknown or expired sign-in attempt", http.StatusBadRequest)
			return
		}

		session, err := client.exchange(r.Context(), code, attempt.verifier)
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
		// Same guard as /demo/reset. Both mutate session state, so both get
		// it; the asymmetry was an oversight when reset gained the check.
		// SameSite=Lax already withholds the cookie from a cross-site POST,
		// which leaves the server-side session intact, but the response still
		// carries the deletion cookie and the browser may apply it.
		if crossSiteRequest(r) {
			http.Error(w, "cross-site logout is not allowed", http.StatusForbidden)
			return
		}
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

	mux.HandleFunc("POST /demo/authorize", requireSession(signedIn, func(w http.ResponseWriter, r *http.Request, session *authSession) {
		if crossSiteRequest(r) {
			http.Error(w, "cross-site authorization is not allowed", http.StatusForbidden)
			return
		}
		token, err := nuts.requestToken(r.Context(), *session)
		if err != nil {
			// The error already names its step; surfacing it verbatim is the
			// point of this route.
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		claims, err := nuts.introspect(r.Context(), token)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		// A response of null, one without claims, and the node's own
		// {"active": false} all decode without error, so nothing before this
		// point reports them. Rendering any of them would report a flow that
		// produced nothing as a success, which is the failure this page exists
		// to make visible.
		//
		// Two guards follow, in the order the node answers them. active is its
		// verdict on the token: RFC 7662 section 2.2 makes it a required boolean
		// and the indication of whether the token is currently active, and the
		// node's own contract marks it required for the same reason. The section
		// only recommends against an inactive response carrying anything else,
		// so a consumer has to honour active rather than read liveness off the
		// rest of the response: {"active": false} alongside a full set of claims
		// is still a refusal. That shape is the node's canonical one, returned by
		// auth/api/iam/api.go for an empty token, for one absent from its store
		// and for an expired one.
		active, isBool := claims["active"].(bool)
		switch {
		case !isBool:
			// Absent, null, or some other type. This also covers a JSON null
			// response, which decodes to a nil map. Whatever answers this way is
			// not a conforming introspection endpoint, which is a different fault
			// from a token the node declines to vouch for, so it is worth its own
			// wording.
			http.Error(w, "introspect access token: response carried no boolean active", http.StatusBadGateway)
			return
		case !active:
			http.Error(w, "introspect access token: the node reported the token as not active", http.StatusBadGateway)
			return
		}

		// The claims are the node's answer about the credential behind the token,
		// so they are asked for only once it has vouched for the token itself.
		// The set is the one the bgz presentation definition emits, and all but
		// one of them is a name component/pdp reads into PolicySubject, so a name
		// absent here is one the PDP would never receive. Indexing rather than
		// sizing also keeps a JSON null off the page, where fmt.Sprint would
		// render it as the "<nil>" no reader can tell from a claim the node
		// really returned.
		//
		// organization_ura_dezi is the exception, and is required anyway. The PDP
		// keeps it only among the properties it does not name and decides on
		// organization_ura (component/pdp/shared.go), because nothing compares the
		// two yet: the presentation definition carries both and cannot reject a
		// mismatch, a field filter seeing one path in one credential. Until that
		// comparison exists somewhere, this claim is the only evidence on this
		// page that the practitioner was authenticated for the organisation the
		// token names. A response without it is one in which that reading became
		// impossible, so rendering it would present an unverifiable page as a
		// complete one.
		claimNames := []string{"user_id", "user_role", "organization_ura", "organization_ura_dezi", "organization_name", "organization_facility_type"}
		var missing []string
		for _, name := range claimNames {
			if claims[name] == nil {
				missing = append(missing, name)
			}
		}
		switch {
		case len(missing) == len(claimNames):
			// Worth its own wording: an active token with no claims at all points
			// at the scope or the policy behind it, where a shortfall means the
			// node vouched for a token carrying the wrong credential.
			http.Error(w, "introspect access token: response carried no claims", http.StatusBadGateway)
			return
		case len(missing) > 0:
			http.Error(w, "introspect access token: response carried no "+strings.Join(missing, ", "), http.StatusBadGateway)
			return
		}
		view := session.view()
		rendered := page{
			Title: "Authorization · Plataan EHR", Guise: "ehr",
			Scenario: scenario, ShowReset: true,
			Active: "dossier", TopTitle: "Authorization",
			Session: &view,
		}
		for _, name := range claimNames {
			rendered.Claims = append(rendered.Claims, claim{Name: name, Value: fmt.Sprint(claims[name])})
		}
		render(w, "authorize.html", rendered)
	}))

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
// same-origin is a reliable positive signal. The header's absence is not a
// reliable negative signal, though: non-browser clients (curl, this repo's
// tests, the demo's own tooling) never send it, so an absent header must stay
// allowed. That limits this guard to stopping browser-driven cross-site
// requests; it is not a substitute for a real CSRF token if this endpoint
// ever needs one.
//
// same-site is deliberately not allowed. It covers sibling origins under the
// same registrable domain, so on a hosted deployment any other subdomain
// would qualify, and /demo/reset needs no session or cookie before clearing
// every practitioner's. Nothing here needs the wider allowance: the templates
// post to their own origin.
func crossSiteRequest(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	return site != "" && site != "same-origin"
}

func render(w http.ResponseWriter, template string, data page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := renderPage(w, template, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
