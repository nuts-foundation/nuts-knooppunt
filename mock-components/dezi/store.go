package main

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"
)

const (
	requestTTL = 10 * time.Minute
	codeTTL    = 2 * time.Minute
	tokenTTL   = time.Hour
)

// authRequest is the state bound at /authorize and consumed at POST /authorize.
type authRequest struct {
	clientID    string
	redirectURI string
	state       string
	challenge   string
	expiresAt   time.Time
}

// authCode is issued after consent and redeemed exactly once at /token.
type authCode struct {
	clientID    string
	redirectURI string
	challenge   string
	expiresAt   time.Time
}

type store struct {
	mu       sync.Mutex
	requests map[string]authRequest
	codes    map[string]authCode
	tokens   map[string]time.Time
	now      func() time.Time
}

func newStore() *store {
	return &store{
		requests: map[string]authRequest{},
		codes:    map[string]authCode{},
		tokens:   map[string]time.Time{},
		now:      time.Now,
	}
}

func opaque() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

// sweep drops everything that has expired. Callers must hold s.mu.
//
// The take* methods only remove the one entry they are handed, and validToken
// removes nothing at all, so without this the three maps grow for the lifetime
// of the process: an abandoned consent screen leaves a request behind, an
// unredeemed code leaves a code, and every completed login leaves its access
// token permanently. GET /authorize is a single unauthenticated request, so
// the cheapest of those needs no login at all.
func (s *store) sweep() {
	now := s.now()
	for handle, r := range s.requests {
		if now.After(r.expiresAt) {
			delete(s.requests, handle)
		}
	}
	for code, c := range s.codes {
		if now.After(c.expiresAt) {
			delete(s.codes, code)
		}
	}
	for token, expiry := range s.tokens {
		if now.After(expiry) {
			delete(s.tokens, token)
		}
	}
}

func (s *store) putRequest(r authRequest) string {
	handle := opaque()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	r.expiresAt = s.now().Add(requestTTL)
	s.requests[handle] = r
	return handle
}

// takeRequest consumes the handle; a handle is valid at most once.
func (s *store) takeRequest(handle string) (authRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.requests[handle]
	delete(s.requests, handle)
	if !ok || s.now().After(r.expiresAt) {
		return authRequest{}, false
	}
	return r, true
}

func (s *store) putCode(c authCode) string {
	code := opaque()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	c.expiresAt = s.now().Add(codeTTL)
	s.codes[code] = c
	return code
}

func (s *store) takeCode(code string) (authCode, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.codes[code]
	delete(s.codes, code)
	if !ok || s.now().After(c.expiresAt) {
		return authCode{}, false
	}
	return c, true
}

func (s *store) putToken() string {
	token := opaque()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep()
	s.tokens[token] = s.now().Add(tokenTTL)
	return token
}

func (s *store) validToken(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.tokens[token]
	return ok && s.now().Before(expiry)
}
