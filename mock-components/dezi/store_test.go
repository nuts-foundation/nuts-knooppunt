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
