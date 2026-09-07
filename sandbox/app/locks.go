package main

import (
	"sort"
	"sync"
	"time"
)

// defaultLockTTL is how long a demo lock survives without a refresh. A running
// demo refreshes on activity; an abandoned run expires so the patient returns
// to the pool.
const defaultLockTTL = 15 * time.Minute

// lockInfo is a single held lock.
type lockInfo struct {
	owner     string
	expiresAt time.Time
}

// Registry is an in-memory advisory lock registry keyed by pool-patient key. It
// keeps a running demo safe on a shared environment: starting a run locks its
// patient, per-patient recycle refuses a locked patient, and a global reset
// warns on active locks. It is advisory only — not a correctness guarantee — and
// process-local (DESIGN §5.7).
type Registry struct {
	mu    sync.Mutex
	ttl   time.Duration
	now   func() time.Time // injectable for tests
	locks map[string]lockInfo
}

// NewRegistry returns an empty lock registry with the default TTL.
func NewRegistry() *Registry {
	return &Registry{
		ttl:   defaultLockTTL,
		now:   time.Now,
		locks: make(map[string]lockInfo),
	}
}

// Lock acquires the lock for key on behalf of owner. It succeeds (returns true)
// if the patient is unlocked, its lock has expired, or it is already held by the
// same owner (in which case the lease is extended). It fails (returns false) if
// a different owner holds a live lock.
func (r *Registry) Lock(key, owner string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	if existing, ok := r.locks[key]; ok && existing.expiresAt.After(now) && existing.owner != owner {
		return false
	}
	r.locks[key] = lockInfo{owner: owner, expiresAt: now.Add(r.ttl)}
	return true
}

// Refresh extends the lease for key if owner still holds it. Returns false if
// the lock is absent, expired, or held by someone else.
func (r *Registry) Refresh(key, owner string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	existing, ok := r.locks[key]
	if !ok || !existing.expiresAt.After(now) || existing.owner != owner {
		return false
	}
	existing.expiresAt = now.Add(r.ttl)
	r.locks[key] = existing
	return true
}

// Release drops the lock for key if owner holds it. Returns true if a lock held
// by owner was removed.
func (r *Registry) Release(key, owner string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.locks[key]
	if !ok || existing.owner != owner {
		return false
	}
	delete(r.locks, key)
	return true
}

// ReleaseAll drops every lock. It exists for reset, which invalidates every
// session: ownership is derived from the session, so a lock whose owner is gone
// can no longer be released by anyone and would block recycle and the next
// non-override reset until its TTL expired.
func (r *Registry) ReleaseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.locks)
}

// IsLocked reports whether key currently has a live lock held by any owner.
func (r *Registry) IsLocked(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.locks[key]
	return ok && existing.expiresAt.After(r.now())
}

// Active returns the keys of all patients with a live lock, sorted. Expired
// locks are pruned as a side effect.
func (r *Registry) Active() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	var active []string
	for key, info := range r.locks {
		if info.expiresAt.After(now) {
			active = append(active, key)
		} else {
			delete(r.locks, key)
		}
	}
	sort.Strings(active)
	return active
}

// Switch acquires key for owner and releases every other key that owner held, so
// one session holds at most one patient: a demo run consumes one patient.
//
// It establishes that key is acquirable before releasing anything. The obvious
// "release mine, then lock the target" ordering loses a valid lock whenever the
// target turns out to be taken, which is exactly when the user most wants the one
// they had.
//
// Lock itself is unchanged: POST /demo/patients/{key}/lock is documented as
// direct lock control and the registry supports several keys per owner
// (TestRegistry_ActiveSorted).
func (r *Registry) Switch(key, owner string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	if existing, ok := r.locks[key]; ok && existing.expiresAt.After(now) && existing.owner != owner {
		return false
	}
	for held, info := range r.locks {
		if held != key && info.owner == owner {
			delete(r.locks, held)
		}
	}
	r.locks[key] = lockInfo{owner: owner, expiresAt: now.Add(r.ttl)}
	return true
}

// ReleaseOwner drops every lock held by owner and returns the keys released.
//
// This exists for the end of a session. Ownership derives from the session id, so
// once the session is gone nobody can release its locks: they would block recycle
// and non-override reset until the TTL expired, and the same practitioner signing
// back in gets a new owner and cannot reclaim their own patient.
func (r *Registry) ReleaseOwner(owner string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var released []string
	for key, info := range r.locks {
		if info.owner == owner {
			released = append(released, key)
			delete(r.locks, key)
		}
	}
	return released
}

// HeldBy reports whether key has a live lock held by owner. The list needs it to
// tell "someone else is running a demo on this patient" from "this is mine", and
// the share POST needs it to reject a stale page.
func (r *Registry) HeldBy(key, owner string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.locks[key]
	return ok && existing.owner == owner && existing.expiresAt.After(r.now())
}
