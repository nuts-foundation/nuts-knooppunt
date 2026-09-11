package main

import (
	"crypto/rsa"
	"os"
	"path/filepath"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/mock-components/prs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ensureRecipientKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "recipient.key")

	created, err := ensureRecipientKey(file)
	require.NoError(t, err)
	info, err := os.Stat(file)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	again, err := ensureRecipientKey(file)
	require.NoError(t, err)
	assert.True(t, created.Equal(again), "the second start reads the key the first one wrote")

	require.NoError(t, os.WriteFile(file, []byte("not a key"), 0o600))
	_, err = ensureRecipientKey(file)
	assert.Error(t, err, "a file that exists but holds no key is an error, not replaced")
}

func Test_ensureOPRFKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "oprf.key")

	created, err := ensureOPRFKey(file)
	require.NoError(t, err)
	require.Len(t, created, 32)
	_, err = prsmock.New(prsmock.Options{ListenAddr: "localhost:0", OPRFKey: created})
	require.NoError(t, err, "the mock accepts what was written")

	again, err := ensureOPRFKey(file)
	require.NoError(t, err)
	assert.Equal(t, created, again)

	require.NoError(t, os.WriteFile(file, []byte("zz"), 0o600))
	_, err = ensureOPRFKey(file)
	assert.Error(t, err)
}

var _ *rsa.PrivateKey
