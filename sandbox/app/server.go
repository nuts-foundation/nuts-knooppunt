package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/pool"
)

var notices = map[string]string{
	"reset-done":     "Dataset restored to the seeded fixtures.",
	"recycle-done":   "Patient restored to the seeded state.",
	"reset-disabled": "Reset is unavailable: the sandbox is not wired to the Knooppunt in this environment.",
	"signed-out":     "Signed out. The Dezi session has been cleared.",
}

// Config holds the sandbox backend's runtime dependencies. The two URLs point at
// the Knooppunt internal API and the HAPI FHIR base; when either is empty the
// reset/recycle actions are disabled (e.g. in unit tests, or a shell-only
// deployment). The reset functions are injectable so tests can exercise the
// handlers without a live HAPI/Knooppunt.
type Config struct {
	KnooppuntInternalURL string
	HAPIBaseURL          string
	Locks                *Registry

	// resetGlobal and recyclePatient default to the vectors implementations when
	// the URLs are set (see NewConfigFromEnv); tests may override them.
	resetGlobal    func(ctx context.Context) error
	recyclePatient func(ctx context.Context, patientKey string) error

	// The NVI half of the share flow, wired in NewConfigFromEnv and injectable so
	// handler tests need no live NVI.
	nviCategories func(ctx context.Context, bsn string) ([]string, error)
	nviRegister   func(ctx context.Context, bsn string, categories []string) error

	// sessions and secureCookie carry the session half of reset, which E2 owns.
	// A reset restores the dataset AND signs everyone out; doing only the first
	// would leave practitioners holding sessions for a world that no longer
	// exists. NewMux sets these; reset_test.go builds a Config directly, so
	// handleReset treats a nil store as "no sessions to drop".
	sessions     *sessionStore
	secureCookie bool
}

// configured reports whether reset/recycle are available.
func (c Config) configured() bool {
	return c.resetGlobal != nil && c.recyclePatient != nil
}

// NewConfigFromEnv builds a Config from the KNOOPPUNT_INTERNAL_URL and
// HAPI_BASE_URL environment variables.
//
// The two features have different needs and are wired separately. The NVI
// client talks only to the Knooppunt, so it comes up as soon as
// KNOOPPUNT_INTERNAL_URL is set. Reset and recycle also rewrite the FHIR store
// directly and need HAPI_BASE_URL as well. Gating both on both would disable the
// NVI over a variable it never reads, and the only symptom would be every
// patient reading "status unknown" with nothing to say why.
func NewConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		KnooppuntInternalURL: getenv("KNOOPPUNT_INTERNAL_URL"),
		HAPIBaseURL:          getenv("HAPI_BASE_URL"),
		Locks:                NewRegistry(),
	}
	if cfg.KnooppuntInternalURL == "" {
		return cfg, nil
	}

	knooppuntURL, err := url.Parse(cfg.KnooppuntInternalURL)
	if err != nil {
		return Config{}, fmt.Errorf("invalid KNOOPPUNT_INTERNAL_URL: %w", err)
	}

	clientID := getenv("SANDBOX_NVI_CLIENT_ID")
	if clientID == "" {
		clientID = pool.PlataanClientID
	}
	cfg.nviCategories, cfg.nviRegister = nviFuncs(knooppuntURL, clientID)

	if cfg.HAPIBaseURL == "" {
		return cfg, nil
	}
	hapiURL, err := url.Parse(cfg.HAPIBaseURL)
	if err != nil {
		return Config{}, fmt.Errorf("invalid HAPI_BASE_URL: %w", err)
	}
	cfg.resetGlobal = func(ctx context.Context) error {
		return vectors.ResetGlobal(ctx, hapiURL, knooppuntURL)
	}
	cfg.recyclePatient = func(ctx context.Context, patientKey string) error {
		return vectors.RecyclePatient(ctx, hapiURL, knooppuntURL, patientKey)
	}
	return cfg, nil
}

// nviConfigured reports whether the sandbox can reach the NVI. main uses it to
// say which feature is off, rather than letting one message stand for both.
func (c Config) nviConfigured() bool { return c.nviCategories != nil }

// patientStatus is the JSON shape returned by GET /demo/patients.
type patientStatus struct {
	Key       string `json:"key"`
	BSN       string `json:"bsn"`
	Name      string `json:"name"`
	BirthDate string `json:"birthDate"`
	Locked    bool   `json:"locked"`
}

// NewMux returns the GF Sandbox HTTP handler. The sandbox is a standalone
// application on its own port; it is not a knooppunt component (DESIGN.md
// standing decision 5).
func NewMux(cfg Config) *http.ServeMux {
	if cfg.Locks == nil {
		cfg.Locks = NewRegistry()
	}

	sessions := newSessionStore()
	// A session ending releases whatever patient it held.
	sessions.onDrop = func(sessionID string) {
		cfg.Locks.ReleaseOwner(lockOwner(&authSession{ID: sessionID}))
	}
	client := newDeziClient(deziConfigFromEnv())
	nuts := newNutsClient(nutsConfigFromEnv())
	clientStates := newClientStateStore()
	secure := secureCookies()

	// Hand the session half of reset to the Config, so handleReset can restore
	// the dataset and end every session in one place.
	cfg.sessions = sessions
	cfg.secureCookie = secure

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
	mux.HandleFunc("GET /demo/login", func(w http.ResponseWriter, r *http.Request) {
		if signedIn(r) != nil {
			http.Redirect(w, r, "/demo/ehr", http.StatusSeeOther)
			return
		}
		render(w, "login.html", page{
			Title: "Sign in · Plataan EHR", Guise: "ehr", Scenario: scenario, ShowReset: true,
		})
	})
	mux.HandleFunc("GET /demo", func(w http.ResponseWriter, r *http.Request) {
		render(w, "demo-start.html", page{
			Title: "Demo · GF Sandbox", Guise: "shell", Scenario: scenario,
			ShowReset: true, Notice: demoNotice(r),
		})
	})
	mux.HandleFunc("POST /demo/login", func(w http.ResponseWriter, r *http.Request) {
		state, verifier := randomURLSafe(), randomURLSafe()
		clientStates.put(state, verifier)
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
		attempt, ok := clientStates.take(q.Get("state"))
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
		// SameSite=Lax already withholds the cookie from a cross-site POST,
		// which leaves the server-side session intact, but the response still
		// carries the deletion cookie and the browser may apply it, so a
		// cross-site form post can sign a practitioner out of the demo.
		if crossSiteRequest(r) {
			http.Error(w, "cross-site logout is not allowed", http.StatusForbidden)
			return
		}
		if cookie, err := r.Cookie(sessionCookie); err == nil {
			sessions.drop(cookie.Value)
		}
		clearSessionCookie(w, secure)
		http.Redirect(w, r, "/demo?notice=signed-out", http.StatusSeeOther)
	})

	// Every destructive demo control needs a session. Without one they were
	// reachable unauthenticated: any browser on a shared deployment could reset
	// the dataset out from under a running demo, or reclaim its patient. The
	// cross-site check inside each handler stops another site driving them; it
	// says nothing about who the caller is.
	mux.HandleFunc("POST /demo/reset", requireSession(signedIn, func(w http.ResponseWriter, r *http.Request, _ *authSession) {
		cfg.handleReset(w, r)
	}))
	mux.HandleFunc("POST /demo/patients/{key}/recycle", requireSession(signedIn, func(w http.ResponseWriter, r *http.Request, _ *authSession) {
		cfg.handleRecycle(w, r)
	}))
	mux.HandleFunc("POST /demo/patients/{key}/lock", requireSession(signedIn, cfg.handleLock))
	mux.HandleFunc("POST /demo/patients/{key}/release", requireSession(signedIn, cfg.handleRelease))
	mux.HandleFunc("GET /demo/patients", cfg.handleListPatients)
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

// demoNotice resolves the notice query param to display text, handling the
// dynamic "locked" case (which carries the offending patient keys).
func demoNotice(r *http.Request) string {
	switch r.URL.Query().Get("notice") {
	case "reset-locked":
		locked := r.URL.Query().Get("locked")
		if locked == "" {
			return "Reset refused: a demo is in progress. Retry with override to proceed."
		}
		return "Reset refused: demo(s) in progress for " + locked + ". Retry with override to proceed."
	default:
		return notices[r.URL.Query().Get("notice")]
	}
}

// handleReset performs a global reset. With active locks it refuses unless
// override=true is passed; this is a demo tool, so override stays possible.
func (c Config) handleReset(w http.ResponseWriter, r *http.Request) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site reset is not allowed", http.StatusForbidden)
		return
	}
	if !c.configured() {
		http.Redirect(w, r, "/demo?notice=reset-disabled", http.StatusSeeOther)
		return
	}

	overrideValue, err := formOrQuery(r, "override")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	override := overrideValue == "true"
	if active := c.Locks.Active(); len(active) > 0 && !override {
		q := url.Values{"notice": {"reset-locked"}, "locked": {strings.Join(active, ", ")}}
		http.Redirect(w, r, "/demo?"+q.Encode(), http.StatusSeeOther)
		return
	}

	resetErr := c.resetGlobal(r.Context())

	// Sessions go whether or not the restore succeeded. ResetGlobal is not
	// transactional: it clears both patient tenants, reloads the fixtures, then
	// reseeds the NVI, so a failure anywhere after the first delete leaves
	// mutated data behind. A session outliving the data it was created against
	// is precisely what this endpoint exists to prevent, and a failed reset is
	// more likely to have mutated something, not less. nil when a test builds a
	// Config directly.
	if c.sessions != nil {
		c.sessions.dropAll()
		clearSessionCookie(w, c.secureCookie)
	}
	// The locks go with them. Ownership is derived from the session, so a lock
	// outliving its owner is one nobody can release: it would refuse recycle and
	// the next non-override reset until the 15-minute TTL ran out. An override
	// reset is exactly the case that produces them.
	c.Locks.ReleaseAll()

	if resetErr != nil {
		http.Error(w, "reset failed: "+resetErr.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/demo?notice=reset-done", http.StatusSeeOther)
}

// handleRecycle restores a single patient. It refuses (409) a locked patient.
// Like handleReset, the "is reset even available here?" check comes first, so
// both endpoints report a disabled backend the same way regardless of the key.
func (c Config) handleRecycle(w http.ResponseWriter, r *http.Request) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site recycle is not allowed", http.StatusForbidden)
		return
	}
	if !c.configured() {
		http.Redirect(w, r, "/demo?notice=reset-disabled", http.StatusSeeOther)
		return
	}
	key := r.PathValue("key")
	if _, ok := pool.PatientByKey(key); !ok {
		http.Error(w, "unknown patient: "+key, http.StatusNotFound)
		return
	}
	if c.Locks.IsLocked(key) {
		http.Error(w, "patient "+key+" is locked (demo in progress)", http.StatusConflict)
		return
	}
	if err := c.recyclePatient(r.Context(), key); err != nil {
		http.Error(w, "recycle failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/demo?notice=recycle-done", http.StatusSeeOther)
}

// handleLock claims a pool patient for the caller's session. Ownership comes
// from the session and nothing else: it used to be a request field defaulting
// to "manual", which made every caller the same owner, so any browser could
// release any other run's lock and the 409 below was true only by accident.
func (c Config) handleLock(w http.ResponseWriter, r *http.Request, session *authSession) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site lock is not allowed", http.StatusForbidden)
		return
	}
	key := r.PathValue("key")
	if _, ok := pool.PatientByKey(key); !ok {
		http.Error(w, "unknown patient: "+key, http.StatusNotFound)
		return
	}
	owner := lockOwner(session)
	if !c.Locks.Lock(key, owner) {
		http.Error(w, "patient "+key+" is already locked by another session", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "locked": true, "owner": owner})
}

// handleRelease drops the caller's own lock. A session that does not hold the
// lock gets released=false rather than an error: releasing something you never
// held is a no-op, not a failure.
func (c Config) handleRelease(w http.ResponseWriter, r *http.Request, session *authSession) {
	if crossSiteRequest(r) {
		http.Error(w, "cross-site release is not allowed", http.StatusForbidden)
		return
	}
	key := r.PathValue("key")
	if _, ok := pool.PatientByKey(key); !ok {
		http.Error(w, "unknown patient: "+key, http.StatusNotFound)
		return
	}
	released := c.Locks.Release(key, lockOwner(session))
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "locked": c.Locks.IsLocked(key), "released": released})
}

// handleListPatients returns the pool with per-patient lock status as JSON, a
// data source for the reset UI and the future E3 patient list.
func (c Config) handleListPatients(w http.ResponseWriter, _ *http.Request) {
	patients := pool.Patients()
	out := make([]patientStatus, 0, len(patients))
	for _, p := range patients {
		name := strings.TrimSpace(strings.Join(p.Name.Given, " ") + " " + derefName(p.Name.Family))
		out = append(out, patientStatus{
			Key:       p.Key,
			BSN:       p.BSN,
			Name:      name,
			BirthDate: p.BirthDate,
			Locked:    c.Locks.IsLocked(p.Key),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	writeJSON(w, http.StatusOK, out)
}

func derefName(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// formOrQuery returns the first value for key from the POST form or the URL
// query. A malformed form body is reported rather than silently read as an
// absent value: for "override" that difference decides whether a global reset
// proceeds, so the caller must be able to tell "not passed" from "unparseable".
func formOrQuery(r *http.Request, key string) (string, error) {
	if err := r.ParseForm(); err != nil {
		return "", fmt.Errorf("parse request form: %w", err)
	}
	return r.Form.Get(key), nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
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
// from another site. The server listens on all interfaces and /demo/logout
// carries no CSRF token, so this header is the only thing between an arbitrary
// web page and signing a practitioner out of the demo.
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
// same registrable domain, so on a hosted deployment any other subdomain would
// qualify. Nothing here needs the wider allowance: the templates post to their
// own origin.
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
