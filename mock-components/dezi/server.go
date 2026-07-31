package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"time"
)

//go:embed templates
var templateFS embed.FS

var consentTemplate = template.Must(template.ParseFS(templateFS, "templates/consent.html"))

// consentView is the data the consent screen renders. It deliberately mirrors
// what the attestation will contain, so the screen cannot promise more than the
// token delivers.
type consentView struct {
	Practitioner
	Handle string
}

// NewMux returns the mock Dezi handler. This is a sandbox flow double, not a
// conformant Dezi provider: userinfo returns a plain JSON envelope rather than
// the encrypted JWE the v0.7 specification defines.
func NewMux(signer *Signer, persona Practitioner) *http.ServeMux {
	st := newStore()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	mux.HandleFunc("GET /dezi/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		set, err := signer.JWKSet()
		if err != nil {
			http.Error(w, "jwks unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(set)
	})

	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" {
			http.Error(w, "unsupported request", http.StatusBadRequest)
			return
		}
		if q.Get("redirect_uri") == "" || q.Get("code_challenge") == "" {
			http.Error(w, "missing redirect_uri or code_challenge", http.StatusBadRequest)
			return
		}
		// This mock binds one hardcoded persona and has no login screen, so
		// anyone who can reach /authorize can already self-serve a code for
		// that persona; this check is not an access control boundary and does
		// not validate that redirect_uri belongs to any particular client.
		// It only rejects non-http(s) schemes (e.g. "javascript:") so a
		// browser is never redirected to an executable target, and so this
		// permissive pattern does not get copied verbatim into a component
		// that has real authentication to protect.
		redirectURI, err := url.Parse(q.Get("redirect_uri"))
		if err != nil || (redirectURI.Scheme != "http" && redirectURI.Scheme != "https") {
			http.Error(w, "redirect_uri must use http or https", http.StatusBadRequest)
			return
		}
		handle := st.putRequest(authRequest{
			clientID:    q.Get("client_id"),
			redirectURI: q.Get("redirect_uri"),
			state:       q.Get("state"),
			challenge:   q.Get("code_challenge"),
		})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := consentTemplate.Execute(w, consentView{Practitioner: persona, Handle: handle}); err != nil {
			http.Error(w, "render failed", http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("POST /authorize", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "malformed form", http.StatusBadRequest)
			return
		}
		req, ok := st.takeRequest(r.PostFormValue("request"))
		if !ok {
			http.Error(w, "unknown or expired request", http.StatusBadRequest)
			return
		}
		code := st.putCode(authCode{
			clientID:    req.clientID,
			redirectURI: req.redirectURI,
			challenge:   req.challenge,
		})
		target, err := url.Parse(req.redirectURI)
		if err != nil {
			http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
			return
		}
		q := target.Query()
		q.Set("code", code)
		if req.state != "" {
			q.Set("state", req.state)
		}
		target.RawQuery = q.Encode()
		http.Redirect(w, r, target.String(), http.StatusSeeOther)
	})

	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "malformed form", http.StatusBadRequest)
			return
		}
		if r.PostFormValue("grant_type") != "authorization_code" {
			http.Error(w, "unsupported_grant_type", http.StatusBadRequest)
			return
		}
		code, ok := st.takeCode(r.PostFormValue("code"))
		if !ok {
			http.Error(w, "invalid_grant", http.StatusBadRequest)
			return
		}
		if code.redirectURI != r.PostFormValue("redirect_uri") || code.clientID != r.PostFormValue("client_id") {
			http.Error(w, "invalid_grant", http.StatusBadRequest)
			return
		}
		if !pkceMatches(code.challenge, r.PostFormValue("code_verifier")) {
			http.Error(w, "invalid_grant", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": st.putToken(),
			"token_type":   "Bearer",
			"expires_in":   int(tokenTTL.Seconds()),
		})
	})

	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if len(header) <= len(prefix) || header[:len(prefix)] != prefix || !st.validToken(header[len(prefix):]) {
			w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
			http.Error(w, "invalid_token", http.StatusUnauthorized)
			return
		}
		attestation, err := signer.Attestation(persona, time.Now(), tokenTTL)
		if err != nil {
			http.Error(w, "attestation failed", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"verklaring_id": opaque(),
			"verklaring":    attestation,
		})
	})

	return mux
}

func pkceMatches(challenge, verifier string) bool {
	if challenge == "" || verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
