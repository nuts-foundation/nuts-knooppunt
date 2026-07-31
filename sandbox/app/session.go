package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const sessionCookie = "gf_sandbox_session"

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

func (s *sessionStore) create(a authSession) string {
	id := newSessionID()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = a
	return id
}

func (s *sessionStore) get(id string) (authSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.sessions[id]
	if !ok || s.now().After(a.ExpiresAt) {
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
