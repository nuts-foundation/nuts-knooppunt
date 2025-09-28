package bsnutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bsn = "123456789"

func Test_PseudonymToTransportToken(t *testing.T) {
	const pseudonym = "ab32d4a3ce9b902d7841800473014c18"
	t.Run("token differs from pseudonym", func(t *testing.T) {
		token, err := PseudonymToTransportToken(pseudonym, "nvi")
		require.NoError(t, err)
		assert.NotEqual(t, pseudonym, token)
	})
	t.Run("token differs each time for the same audience", func(t *testing.T) {
		token1, err := PseudonymToTransportToken(pseudonym, "nvi")
		require.NoError(t, err)
		token2, err := PseudonymToTransportToken(pseudonym, "nvi")
		require.NoError(t, err)
		assert.NotEqual(t, token1, token2)
	})
	t.Run("token for different audience differs", func(t *testing.T) {
		token1, err := PseudonymToTransportToken(pseudonym, "nvi")
		require.NoError(t, err)
		token2, err := PseudonymToTransportToken(pseudonym, "org-a")
		require.NoError(t, err)
		assert.NotEqual(t, token1, token2)
	})
	t.Run("roundtrip", func(t *testing.T) {
		token, err := PseudonymToTransportToken(pseudonym, "nvi")
		require.NoError(t, err)
		extractedPseudonym, err := TransportTokenToPseudonym(token, "nvi")
		require.NoError(t, err)
		assert.Equal(t, pseudonym, extractedPseudonym)
	})
}

func TestBSNToPseudonym(t *testing.T) {
	t.Run("stable pseudonym to BSN conversion", func(t *testing.T) {
		pseudonym1, err := BSNToPseudonym(bsn, "audience-1")
		require.NoError(t, err)
		pseudonym2, err := BSNToPseudonym(bsn, "audience-1")
		require.NoError(t, err)
		require.Equal(t, pseudonym1, pseudonym2)
		println(pseudonym1)
	})
	t.Run("different audiences yield different pseudonyms", func(t *testing.T) {
		pseudonym1, err := BSNToPseudonym(bsn, "audience-1")
		require.NoError(t, err)
		pseudonym2, err := BSNToPseudonym(bsn, "audience-2")
		require.NoError(t, err)
		require.NotEqual(t, pseudonym1, pseudonym2)
	})
	t.Run("roundtrip", func(t *testing.T) {
		pseudonym, err := BSNToPseudonym(bsn, "audience-1")
		require.NoError(t, err)
		extractedBSN, err := PseudonymToBSN(pseudonym, "audience-1")
		require.NoError(t, err)
		require.Equal(t, bsn, extractedBSN)
	})
}

func TestBSNToTransportToken(t *testing.T) {
	const bsn = "123456789"
	t.Run("roundtrip", func(t *testing.T) {
		token, err := BSNToTransportToken(bsn, "nvi")
		require.NoError(t, err)
		extractedBSN, err := TransportTokenToBSN(token, "nvi")
		require.NoError(t, err)
		require.Equal(t, bsn, extractedBSN)
	})
}
