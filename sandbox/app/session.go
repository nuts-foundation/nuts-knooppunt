package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const sessionCookie = "gf_sandbox_session"

// secureCookies reports whether the session cookie carries Secure. It reads
// SANDBOX_PUBLIC_URL rather than the scheme of the incoming request, because
// the hosted sandbox sits behind a TLS-terminating proxy: the browser speaks
// https while the request reaching this process is plain http. The public URL
// is the only value here that describes what the browser actually uses.
//
// Setting it unconditionally is not an option: over plain http a Secure cookie
// is dropped by every browser except on localhost, which would silently break
// the local demo.
//
// The scheme is compared case-insensitively because RFC 3986 section 3.1
// makes it so; "HTTPS://host" is a valid https URL and must not quietly
// produce a cookie without Secure.
func secureCookies() bool {
	parsed, err := url.Parse(sandboxPublicURL())
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https")
}

// clearSessionCookie writes the deletion cookie. Both /demo/logout and
// /demo/reset end a session and each carried its own copy of these attributes.
// A cookie is only deleted when the Name and Path match the one that was set,
// so two copies were two chances to drift out of that agreement.
func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: secure, MaxAge: -1,
	})
}

// authSession is the internal record of a signed-in practitioner. It holds the
// raw attestation, which E4 sends to the Nuts node as id_token. It is never
// rendered and never logged; templates receive the Session view model instead.
type authSession struct {
	DeziNumber    string
	Initials      string
	SurnamePrefix string
	Surname       string
	OrgURA        string
	OrgName       string
	RoleCode      string
	RoleName      string
	RoleRegistry  string
	Attestation   string
	ExpiresAt     time.Time
}

// englishRoleNames maps RoleCodeNL codes to the English labels the demo shows.
// The attestation keeps the official Dutch name; the interface is English
// (sandbox/DESIGN.md section 4).
var englishRoleNames = map[string]string{
	"01.022": "Clinical geriatrician",
	"01.016": "Internist",
	"01.047": "Elderly care physician",
}

// englishOrgNames maps the legal organization name in the attestation to the
// display name used in the interface and the wireframe.
var englishOrgNames = map[string]string{
	"Ziekenhuis De Plataan": "De Plataan Hospital",
}

func displayOr(m map[string]string, key, fallback string) string {
	if value, ok := m[key]; ok {
		return value
	}
	return fallback
}

// firstLetter returns the uppercased first rune of s, or "" if s is blank
// once trimmed. It decodes a full rune rather than slicing the first byte,
// so a surname starting with a multi-byte character (e.g. "Özdemir") yields
// its actual initial instead of an invalid, truncated byte.
func firstLetter(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(trimmed)
	return strings.ToUpper(string(r))
}

// view derives the small, render-safe projection the top bar consumes.
func (a authSession) view() Session {
	initials := firstLetter(a.Initials)
	if fields := strings.Fields(a.Surname); len(fields) > 0 {
		// The surname's own leading words can be a Dutch tussenvoegsel
		// (e.g. "el" in "el Amrani"); the avatar initial comes from the
		// capitalized core name, i.e. the last word, not the first byte
		// of the whole field.
		initials += firstLetter(fields[len(fields)-1])
	}
	name := strings.TrimSpace(strings.Join([]string{"Dr.", a.Initials, a.SurnamePrefix, a.Surname}, " "))
	name = strings.Join(strings.Fields(name), " ")
	return Session{
		Initials: initials,
		Name:     name,
		Description: fmt.Sprintf("%s · UZI %s · %s",
			displayOr(englishRoleNames, a.RoleCode, a.RoleName),
			a.DeziNumber,
			displayOr(englishOrgNames, a.OrgName, a.OrgName),
		),
	}
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]authSession
	now      func() time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: map[string]authSession{}, now: time.Now}
}

func newSessionID() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// create stores a signed-in practitioner and sweeps expired sessions, which is
// what actually bounds this map. Reaping in get only reaches sessions that are
// presented again; one whose cookie is discarded after expiry is never looked
// up and would otherwise stay for the lifetime of the process.
func (s *sessionStore) create(a authSession) string {
	id := newSessionID()
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, session := range s.sessions {
		if now.After(session.ExpiresAt) {
			delete(s.sessions, key)
		}
	}
	s.sessions[id] = a
	return id
}

func (s *sessionStore) get(id string) (authSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.sessions[id]
	if !ok {
		return authSession{}, false
	}
	if s.now().After(a.ExpiresAt) {
		// Reap on read, the same way clientStateStore.take drops what it hands
		// back. create sweeps the rest, so this only saves the map from
		// holding an entry until the next sign-in.
		delete(s.sessions, id)
		return authSession{}, false
	}
	return a, true
}

func (s *sessionStore) drop(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

func (s *sessionStore) dropAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.sessions)
}
