package main

import (
	"sync"
	"time"
)

// pendingAuthTTL is how long a sign-in attempt started at POST /demo/login
// stays valid for its callback.
const pendingAuthTTL = 10 * time.Minute

// pendingAuth is a login attempt awaiting its callback, keyed by state.
type pendingAuth struct {
	verifier  string
	expiresAt time.Time
}

// pendingStore tracks in-flight sign-in attempts keyed by the OAuth state
// value. Like sessionStore (session.go) and the mock's store
// (mock-components/dezi/store.go), it carries an injectable clock, so a test
// can reach the expiry branch without sleeping the suite for pendingAuthTTL.
type pendingStore struct {
	mu      sync.Mutex
	pending map[string]pendingAuth
	now     func() time.Time
}

func newPendingStore() *pendingStore {
	return &pendingStore{pending: map[string]pendingAuth{}, now: time.Now}
}

// put records a new sign-in attempt, valid until pendingAuthTTL elapses.
func (p *pendingStore) put(state, verifier string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending[state] = pendingAuth{verifier: verifier, expiresAt: p.now().Add(pendingAuthTTL)}
}

// take consumes state exactly once: a replayed or expired state is rejected.
func (p *pendingStore) take(state string) (pendingAuth, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, ok := p.pending[state]
	delete(p.pending, state)
	if !ok || p.now().After(attempt.expiresAt) {
		return pendingAuth{}, false
	}
	return attempt, true
}
