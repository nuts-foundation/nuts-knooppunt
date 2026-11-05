// Package certutil loads an X.509 certificate with its private key from files, whatever it is used
// for: TLS client authentication (see tlsutil) or signing.
package certutil

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"software.sslmate.com/src/go-pkcs12"
)

// KeyPair is a certificate chain with the private key of its leaf.
type KeyPair struct {
	// Chain is the certificate chain in DER form, leaf first.
	Chain [][]byte
	// Leaf is the parsed first certificate of Chain.
	Leaf       *x509.Certificate
	PrivateKey crypto.Signer
}

// Load loads a certificate and its private key: from a PKCS#12 file (.p12/.pfx, which holds both, so
// keyFile is ignored), or else from a PEM certificate file (the leaf followed by any intermediates)
// and a PEM key file. password decrypts a PKCS#12 file.
func Load(certFile, keyFile, password string) (*KeyPair, error) {
	if certFile == "" {
		return nil, errors.New("certificate file not specified")
	}
	ext := strings.ToLower(filepath.Ext(certFile))
	if ext == ".p12" || ext == ".pfx" {
		keyPair, err := loadPKCS12(certFile, password)
		if err != nil {
			return nil, fmt.Errorf("failed to load PKCS#12: %w", err)
		}
		return keyPair, nil
	}
	if keyFile == "" {
		return nil, errors.New("key file required when using PEM certificate")
	}
	keyPair, err := loadPEM(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}
	return keyPair, nil
}

func loadPKCS12(file, password string) (*KeyPair, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read PKCS#12 file: %w", err)
	}
	key, leaf, intermediates, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return nil, fmt.Errorf("failed to decode PKCS#12: %w", err)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("unsupported private key type %T", key)
	}
	chain := [][]byte{leaf.Raw}
	for _, intermediate := range intermediates {
		chain = append(chain, intermediate.Raw)
	}
	return newKeyPair(chain, signer)
}

func loadPEM(certFile, keyFile string) (*KeyPair, error) {
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	var chain [][]byte
	for block, rest := pem.Decode(certPEM); block != nil; block, rest = pem.Decode(rest) {
		if block.Type == "CERTIFICATE" {
			chain = append(chain, block.Bytes)
		}
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	for block, rest := pem.Decode(keyPEM); block != nil; block, rest = pem.Decode(rest) {
		if strings.HasSuffix(block.Type, "PRIVATE KEY") {
			key, err := parsePrivateKey(block.Bytes)
			if err != nil {
				return nil, err
			}
			return newKeyPair(chain, key)
		}
	}
	return nil, errors.New("no private key found in key file")
}

// parsePrivateKey parses a DER private key in any of the forms PEM files commonly hold: PKCS#8,
// PKCS#1 (RSA) or SEC 1 (EC).
func parsePrivateKey(der []byte) (crypto.Signer, error) {
	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("unsupported private key type %T", key)
		}
		return signer, nil
	}
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, nil
	}
	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, nil
	}
	return nil, errors.New("failed to parse private key")
}

// newKeyPair checks that the chain has a leaf that belongs to the key.
func newKeyPair(chain [][]byte, key crypto.Signer) (*KeyPair, error) {
	if len(chain) == 0 {
		return nil, errors.New("no certificates found in file")
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse leaf certificate: %w", err)
	}
	publicKey, ok := leaf.PublicKey.(interface{ Equal(crypto.PublicKey) bool })
	if !ok || !publicKey.Equal(key.Public()) {
		return nil, errors.New("private key does not match the certificate")
	}
	return &KeyPair{Chain: chain, Leaf: leaf, PrivateKey: key}, nil
}
