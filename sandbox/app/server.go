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
}

// configured reports whether reset/recycle are available.
func (c Config) configured() bool {
	return c.resetGlobal != nil && c.recyclePatient != nil
}

// NewConfigFromEnv builds a Config from the KNOOPPUNT_INTERNAL_URL and
// HAPI_BASE_URL environment variables. When both are set, the reset functions
// are wired to the vectors package; otherwise they are left nil (reset disabled).
func NewConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		KnooppuntInternalURL: getenv("KNOOPPUNT_INTERNAL_URL"),
		HAPIBaseURL:          getenv("HAPI_BASE_URL"),
		Locks:                NewRegistry(),
	}
	if cfg.KnooppuntInternalURL == "" || cfg.HAPIBaseURL == "" {
		return cfg, nil
	}

	knooppuntURL, err := url.Parse(cfg.KnooppuntInternalURL)
	if err != nil {
		return Config{}, fmt.Errorf("invalid KNOOPPUNT_INTERNAL_URL: %w", err)
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
		p := page{Title: "Demo · GF Sandbox", Guise: "shell", Scenario: scenario, ShowReset: true, Notice: demoNotice(r)}
		if err := renderPage(w, "demo-start.html", p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("POST /demo/reset", cfg.handleReset)
	mux.HandleFunc("POST /demo/patients/{key}/recycle", cfg.handleRecycle)
	mux.HandleFunc("POST /demo/patients/{key}/lock", cfg.handleLock)
	mux.HandleFunc("POST /demo/patients/{key}/release", cfg.handleRelease)
	mux.HandleFunc("GET /demo/patients", cfg.handleListPatients)
	mux.HandleFunc("GET /demo/ehr", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		p := page{
			Title: "Home · Plataan EHR", Guise: "ehr",
			Scenario: scenario, ShowReset: true,
			BodyClass: "hood-open", BodyAttrs: viewerBodyAttrs(true),
			Active: "dossier", TopTitle: "Home", ViewerOpen: true,
		}
		if err := renderPage(w, "ehr-home.html", p); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
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

	if err := c.resetGlobal(r.Context()); err != nil {
		http.Error(w, "reset failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/demo?notice=reset-done", http.StatusSeeOther)
}

// handleRecycle restores a single patient. It refuses (409) a locked patient.
// Like handleReset, the "is reset even available here?" check comes first, so
// both endpoints report a disabled backend the same way regardless of the key.
func (c Config) handleRecycle(w http.ResponseWriter, r *http.Request) {
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

// handleLock manually acquires a lock (testability; the real trigger is E4/E6).
// The lock owner is taken from the "owner" form/query value, defaulting to
// "manual".
func (c Config) handleLock(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if _, ok := pool.PatientByKey(key); !ok {
		http.Error(w, "unknown patient: "+key, http.StatusNotFound)
		return
	}
	owner, err := formOrQueryDefault(r, "owner", "manual")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !c.Locks.Lock(key, owner) {
		http.Error(w, "patient "+key+" is already locked by another session", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "locked": true, "owner": owner})
}

// handleRelease manually releases a lock.
func (c Config) handleRelease(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if _, ok := pool.PatientByKey(key); !ok {
		http.Error(w, "unknown patient: "+key, http.StatusNotFound)
		return
	}
	owner, err := formOrQueryDefault(r, "owner", "manual")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	released := c.Locks.Release(key, owner)
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

func formOrQueryDefault(r *http.Request, key, def string) (string, error) {
	v, err := formOrQuery(r, key)
	if err != nil {
		return "", err
	}
	if v == "" {
		return def, nil
	}
	return v, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
