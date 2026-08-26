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

// clientStateStore tracks in-flight sign-in attempts keyed by the OAuth state
// value. Like sessionStore (session.go) and the mock's store
// (mock-components/dezi/store.go), it carries an injectable clock, so a test
// can reach the expiry branch without sleeping the suite for pendingAuthTTL.
type clientStateStore struct {
	mu     sync.Mutex
	states map[string]pendingAuth
	now    func() time.Time
}

func newClientStateStore() *clientStateStore {
	return &clientStateStore{states: map[string]pendingAuth{}, now: time.Now}
}

// put records a new sign-in attempt, valid until pendingAuthTTL elapses, and
// sweeps whatever has expired in the meantime.
//
// The sweep is what bounds this map. take only ever removes the one state it
// is handed, so an attempt that is abandoned, denied by Dezi, or lost to a
// truncated callback is never consumed and would sit here for the lifetime of
// the process. POST /demo/login is an unauthenticated single request, which
// makes this the cheapest way to grow the sandbox's memory. Sweeping on
// insert costs a walk of a map that this very sweep keeps small.
func (p *clientStateStore) put(state, verifier string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	for key, attempt := range p.states {
		if now.After(attempt.expiresAt) {
			delete(p.states, key)
		}
	}
	p.states[state] = pendingAuth{verifier: verifier, expiresAt: now.Add(pendingAuthTTL)}
}

// take consumes state exactly once: a replayed or expired state is rejected.
func (p *clientStateStore) take(state string) (pendingAuth, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	attempt, ok := p.states[state]
	delete(p.states, state)
	if !ok || p.now().After(attempt.expiresAt) {
		return pendingAuth{}, false
	}
	return attempt, true
}
