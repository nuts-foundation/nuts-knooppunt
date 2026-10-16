package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// loadOrGenerateKey returns the RSA key at path, generating and persisting one
// if the file does not exist. No key is committed to the repository.
//
// Persistence matters: a key regenerated on every start would leave
// already-issued attestations unverifiable, because the published JWK Set would
// no longer contain the key that signed them. An empty path means "generate in
// memory", which is what `go run` and the tests use.
func loadOrGenerateKey(path string) (*rsa.PrivateKey, error) {
	if path == "" {
		return rsa.GenerateKey(rand.Reader, 2048)
	}

	switch existing, err := os.ReadFile(path); {
	case err == nil:
		block, _ := pem.Decode(existing)
		if block == nil {
			return nil, fmt.Errorf("signing key at %s is not PEM encoded", path)
		}
		if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			return key, nil
		}
		// OpenSSL 3 writes PKCS#8 ("BEGIN PRIVATE KEY") by default; only
		// LibreSSL, the macOS default, still writes PKCS#1 ("BEGIN RSA
		// PRIVATE KEY"). Accept whichever produced this key, so loading it
		// does not depend on which tool or OS generated it.
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse signing key at %s: not a valid PKCS#1 or PKCS#8 key: %w", path, err)
		}
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("signing key at %s is a %T, not an RSA private key", path, parsed)
		}
		return key, nil
	case !os.IsNotExist(err):
		return nil, fmt.Errorf("read signing key at %s: %w", path, err)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate signing key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create key directory: %w", err)
	}
	encoded := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		return nil, fmt.Errorf("persist signing key: %w", err)
	}
	return key, nil
}
