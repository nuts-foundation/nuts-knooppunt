package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// newTestRegistry returns a registry whose clock is controlled by the returned
// pointer, so tests can advance time deterministically.
func newTestRegistry(ttl time.Duration) (*Registry, *time.Time) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	r := NewRegistry()
	r.ttl = ttl
	r.now = func() time.Time { return now }
	return r, &now
}

func TestRegistry_LockAndOwnership(t *testing.T) {
	r, _ := newTestRegistry(time.Minute)

	require.True(t, r.Lock("anna", "session-a"), "first lock should succeed")
	require.True(t, r.IsLocked("anna"))

	require.False(t, r.Lock("anna", "session-b"), "different owner cannot steal a live lock")
	require.True(t, r.Lock("anna", "session-a"), "same owner re-locking extends the lease")

	require.False(t, r.IsLocked("pool-02"), "unrelated patient is not locked")
}

func TestRegistry_Release(t *testing.T) {
	r, _ := newTestRegistry(time.Minute)

	require.True(t, r.Lock("anna", "session-a"))
	require.False(t, r.Release("anna", "session-b"), "cannot release someone else's lock")
	require.True(t, r.Release("anna", "session-a"))
	require.False(t, r.IsLocked("anna"))
	require.False(t, r.Release("anna", "session-a"), "releasing an absent lock returns false")
}

func TestRegistry_Refresh(t *testing.T) {
	r, now := newTestRegistry(time.Minute)

	require.True(t, r.Lock("anna", "session-a"))

	// Advance almost to expiry, then refresh.
	*now = now.Add(50 * time.Second)
	require.True(t, r.Refresh("anna", "session-a"))

	// Past the original expiry but within the refreshed lease.
	*now = now.Add(30 * time.Second)
	require.True(t, r.IsLocked("anna"), "refresh should have extended the lease")

	require.False(t, r.Refresh("anna", "session-b"), "cannot refresh another owner's lock")
}

func TestRegistry_Expiry(t *testing.T) {
	r, now := newTestRegistry(time.Minute)

	require.True(t, r.Lock("anna", "session-a"))
	require.Equal(t, []string{"anna"}, r.Active())

	// After the TTL the lock is gone and a different owner can acquire it.
	*now = now.Add(2 * time.Minute)
	require.False(t, r.IsLocked("anna"))
	require.Empty(t, r.Active(), "expired locks are pruned")
	require.True(t, r.Lock("anna", "session-b"), "expired lock is reacquirable by anyone")
}

func TestRegistry_ActiveSorted(t *testing.T) {
	r, _ := newTestRegistry(time.Minute)

	require.True(t, r.Lock("pool-03", "s"))
	require.True(t, r.Lock("anna", "s"))
	require.True(t, r.Lock("pool-02", "s"))

	require.Equal(t, []string{"anna", "pool-02", "pool-03"}, r.Active())
}
