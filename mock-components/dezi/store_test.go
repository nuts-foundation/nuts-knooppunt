package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// These tests drive the store's now seam directly rather than going through
// HTTP: it is the only way to observe the three TTLs (requestTTL, codeTTL,
// tokenTTL) without sleeping the test suite for real minutes.

func TestTakeRequestRejectsExpiredHandle(t *testing.T) {
	s := newStore()
	start := time.Now()
	s.now = func() time.Time { return start }

	handle := s.putRequest(authRequest{
		clientID:    "gf-sandbox",
		redirectURI: "http://localhost:8091/demo/auth/callback",
		challenge:   "challenge",
	})

	s.now = func() time.Time { return start.Add(requestTTL).Add(time.Second) }

	_, ok := s.takeRequest(handle)
	require.False(t, ok, "a request handle must not survive past requestTTL")
}

func TestTakeCodeRejectsExpiredCode(t *testing.T) {
	s := newStore()
	start := time.Now()
	s.now = func() time.Time { return start }

	code := s.putCode(authCode{
		clientID:    "gf-sandbox",
		redirectURI: "http://localhost:8091/demo/auth/callback",
		challenge:   "challenge",
	})

	s.now = func() time.Time { return start.Add(codeTTL).Add(time.Second) }

	_, ok := s.takeCode(code)
	require.False(t, ok, "an authorization code must not survive past codeTTL")
}

func TestValidTokenRejectsExpiredToken(t *testing.T) {
	s := newStore()
	start := time.Now()
	s.now = func() time.Time { return start }

	token := s.putToken()

	s.now = func() time.Time { return start.Add(tokenTTL).Add(time.Second) }

	require.False(t, s.validToken(token), "an access token must not validate past tokenTTL")
}

func TestSweepBoundsEveryMap(t *testing.T) {
	s := newStore()
	base := time.Now()
	s.now = func() time.Time { return base }

	s.putRequest(authRequest{clientID: "gf-sandbox"})
	s.putCode(authCode{clientID: "gf-sandbox"})
	s.putToken()
	require.Len(t, s.requests, 1)
	require.Len(t, s.codes, 1)
	require.Len(t, s.tokens, 1)

	// takeRequest and takeCode only drop what they are handed, and validToken
	// drops nothing at all, so a successful login used to leave its token here
	// permanently. Inserting anything must clear what has expired.
	s.now = func() time.Time { return base.Add(24 * time.Hour) }
	s.putRequest(authRequest{clientID: "gf-sandbox"})
	require.Len(t, s.requests, 1, "only the fresh request may remain")
	require.Empty(t, s.codes, "the expired code must be swept")
	require.Empty(t, s.tokens, "the expired token must be swept")
}

// TestEveryInsertSweeps covers each insertion separately. Driving the sweep
// through one method only proves that sweep visits every map, not that every
// method calls it, so a missing call in the other two would go unnoticed.
func TestEveryInsertSweeps(t *testing.T) {
	for name, insert := range map[string]func(*store){
		"putRequest": func(s *store) { s.putRequest(authRequest{clientID: "gf-sandbox"}) },
		"putCode":    func(s *store) { s.putCode(authCode{clientID: "gf-sandbox"}) },
		"putToken":   func(s *store) { s.putToken() },
	} {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			base := time.Now()
			s.now = func() time.Time { return base }
			s.putRequest(authRequest{clientID: "stale"})
			s.putCode(authCode{clientID: "stale"})
			s.putToken()

			s.now = func() time.Time { return base.Add(24 * time.Hour) }
			insert(s)

			total := len(s.requests) + len(s.codes) + len(s.tokens)
			require.Equal(t, 1, total, "%s must sweep the three expired entries, leaving only its own", name)
		})
	}
}

func TestTakeConsumesOnlyTheEntryItIsGiven(t *testing.T) {
	t.Run("takeRequest", func(t *testing.T) {
		s := newStore()
		first := s.putRequest(authRequest{clientID: "first"})
		second := s.putRequest(authRequest{clientID: "second"})

		_, ok := s.takeRequest(first)
		require.True(t, ok)

		r, ok := s.takeRequest(second)
		require.True(t, ok, "one consent submit must not cancel another in flight")
		require.Equal(t, "second", r.clientID)
	})

	t.Run("takeCode", func(t *testing.T) {
		s := newStore()
		first := s.putCode(authCode{clientID: "first"})
		second := s.putCode(authCode{clientID: "second"})

		_, ok := s.takeCode(first)
		require.True(t, ok)

		c, ok := s.takeCode(second)
		require.True(t, ok, "redeeming one code must not invalidate another")
		require.Equal(t, "second", c.clientID)
	})
}
