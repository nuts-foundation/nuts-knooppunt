package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadOrGenerateKey_EmptyPathIsInMemoryOnly(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(cwd)) })

	key, err := loadOrGenerateKey("")
	require.NoError(t, err)
	require.NotNil(t, key)
	require.NoError(t, key.Validate(), "generated key must be a usable RSA key")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries, "empty path must not write anything to disk")
}

func TestLoadOrGenerateKey_PersistsAndReloadsSameKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signing-key.pem")

	first, err := loadOrGenerateKey(path)
	require.NoError(t, err)
	require.NoError(t, first.Validate())
	require.FileExists(t, path)

	second, err := loadOrGenerateKey(path)
	require.NoError(t, err)

	// Compare the modulus rather than pointer identity: the second call reads
	// a fresh key back off disk, so it must be a distinct value with the same
	// key material, not the same object or an unrelated freshly generated key.
	require.Equal(t, 0, first.N.Cmp(second.N), "reloaded key must have the same modulus as the persisted one")
}

func TestLoadOrGenerateKey_PersistsWithOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signing-key.pem")

	_, err := loadOrGenerateKey(path)
	require.NoError(t, err)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestLoadOrGenerateKey_AcceptsPKCS8Key(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signing-key.pem")

	generated, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(generated)
	require.NoError(t, err)
	encoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	require.NoError(t, os.WriteFile(path, encoded, 0o600))

	// OpenSSL 3 defaults genrsa to PKCS#8; only LibreSSL (the macOS default)
	// still writes PKCS#1. A key in either format must load, since operators
	// run the setup script on both.
	key, err := loadOrGenerateKey(path)
	require.NoError(t, err)
	require.NoError(t, key.Validate())
	require.Equal(t, 0, generated.N.Cmp(key.N), "loaded key must have the same modulus as the PKCS#8 fixture")
}

func TestLoadOrGenerateKey_MalformedPEMReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signing-key.pem")
	require.NoError(t, os.WriteFile(path, []byte("this is not PEM encoded"), 0o600))

	// A malformed key on disk must surface as an error, not be silently
	// replaced by a freshly generated key: that would make the file on disk
	// diverge from whatever key the operator or a previous run expected.
	key, err := loadOrGenerateKey(path)
	require.Error(t, err)
	require.Nil(t, key)
}

func TestLoadOrGenerateKey_UnexpectedReadErrorPropagates(t *testing.T) {
	// A directory at the expected file path produces a read error that is not
	// os.IsNotExist. That error must propagate instead of being treated as
	// "file missing" and triggering silent key generation.
	path := filepath.Join(t.TempDir(), "signing-key.pem")
	require.NoError(t, os.Mkdir(path, 0o700))

	key, err := loadOrGenerateKey(path)
	require.Error(t, err)
	require.Nil(t, key)
	require.Contains(t, err.Error(), "read signing key")
}
