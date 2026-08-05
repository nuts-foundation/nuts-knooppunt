package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestPendingTakeRejectsExpiredState drives pendingStore's now seam directly,
// mirroring mock-components/dezi/store_test.go: NewMux does not expose the
// pending store it builds, so this is the only way to reach the expiry
// branch without sleeping the suite for pendingAuthTTL.
func TestPendingTakeRejectsExpiredState(t *testing.T) {
	p := newPendingStore()
	start := time.Now()
	p.now = func() time.Time { return start }

	p.put("state-1", "verifier-1")

	p.now = func() time.Time { return start.Add(pendingAuthTTL).Add(time.Second) }

	_, ok := p.take("state-1")
	require.False(t, ok, "a pending sign-in attempt must not survive past pendingAuthTTL")
}

func TestPutSweepsExpiredAttempts(t *testing.T) {
	store := newPendingStore()
	base := time.Now()
	store.now = func() time.Time { return base }
	store.put("abandoned-state", "verifier-one")
	require.Len(t, store.pending, 1)

	// take only removes the state it is handed, so an attempt nobody ever
	// calls back would otherwise stay for the lifetime of the process.
	store.now = func() time.Time { return base.Add(pendingAuthTTL + time.Second) }
	store.put("fresh-state", "verifier-two")
	require.Len(t, store.pending, 1, "starting a login must sweep the expired attempts")
	_, ok := store.pending["abandoned-state"]
	require.False(t, ok)
}

func TestTakeConsumesOnlyTheStateItIsGiven(t *testing.T) {
	store := newPendingStore()
	store.put("first-state", "verifier-one")
	store.put("second-state", "verifier-two")

	// take's comment calls the state consumed "exactly once", which is a claim
	// about that one entry. Clearing the whole map satisfies every other
	// assertion here while cancelling a concurrent sign-in.
	_, ok := store.take("first-state")
	require.True(t, ok)

	attempt, ok := store.take("second-state")
	require.True(t, ok, "a concurrent sign-in must survive someone else's callback")
	require.Equal(t, "verifier-two", attempt.verifier)
}
