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
