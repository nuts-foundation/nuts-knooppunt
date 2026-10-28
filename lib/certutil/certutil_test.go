package certutil

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"software.sslmate.com/src/go-pkcs12"
)

// issue creates a certificate for key, signed by parent's key (self-signed when parent is nil).
func issue(t *testing.T, name string, key crypto.Signer, parent *x509.Certificate, parentKey crypto.Signer) *x509.Certificate {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: name},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  parent == nil,
		BasicConstraintsValid: true,
	}
	if parent == nil {
		parent, parentKey = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, key.Public(), parentKey)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return certificate
}

func writePEM(t *testing.T, name string, blocks ...*pem.Block) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), name)
	var data []byte
	for _, block := range blocks {
		data = append(data, pem.EncodeToMemory(block)...)
	}
	require.NoError(t, os.WriteFile(file, data, 0600))
	return file
}

func TestLoad(t *testing.T) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca := issue(t, "CA", caKey, nil, nil)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsaLeaf := issue(t, "RSA leaf", rsaKey, ca, caKey)
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecLeaf := issue(t, "EC leaf", ecKey, ca, caKey)
	pkcs8, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	require.NoError(t, err)
	sec1, err := x509.MarshalECPrivateKey(ecKey)
	require.NoError(t, err)

	t.Run("PEM chain with a PKCS#1 key", func(t *testing.T) {
		certFile := writePEM(t, "chain.pem", &pem.Block{Type: "CERTIFICATE", Bytes: rsaLeaf.Raw}, &pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
		keyFile := writePEM(t, "key.pem", &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)})

		keyPair, err := Load(certFile, keyFile, "")

		require.NoError(t, err)
		assert.Equal(t, [][]byte{rsaLeaf.Raw, ca.Raw}, keyPair.Chain, "leaf first, then intermediates")
		assert.Equal(t, "RSA leaf", keyPair.Leaf.Subject.CommonName)
		assert.True(t, rsaKey.Equal(keyPair.PrivateKey))
	})

	t.Run("PEM with a PKCS#8 key", func(t *testing.T) {
		certFile := writePEM(t, "cert.pem", &pem.Block{Type: "CERTIFICATE", Bytes: rsaLeaf.Raw})
		keyFile := writePEM(t, "key.pem", &pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

		keyPair, err := Load(certFile, keyFile, "")

		require.NoError(t, err)
		assert.True(t, rsaKey.Equal(keyPair.PrivateKey))
	})

	t.Run("PEM with an EC key", func(t *testing.T) {
		certFile := writePEM(t, "cert.pem", &pem.Block{Type: "CERTIFICATE", Bytes: ecLeaf.Raw})
		keyFile := writePEM(t, "key.pem", &pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1})

		keyPair, err := Load(certFile, keyFile, "")

		require.NoError(t, err)
		assert.True(t, ecKey.Equal(keyPair.PrivateKey))
	})

	t.Run("PKCS#12 with an intermediate", func(t *testing.T) {
		data, err := pkcs12.Modern.Encode(ecKey, ecLeaf, []*x509.Certificate{ca}, "secret")
		require.NoError(t, err)
		file := filepath.Join(t.TempDir(), "cert.p12")
		require.NoError(t, os.WriteFile(file, data, 0600))

		keyPair, err := Load(file, "", "secret")

		require.NoError(t, err)
		assert.Equal(t, [][]byte{ecLeaf.Raw, ca.Raw}, keyPair.Chain)
		assert.True(t, ecKey.Equal(keyPair.PrivateKey))
	})

	t.Run("PKCS#12 with the wrong password", func(t *testing.T) {
		data, err := pkcs12.Modern.Encode(ecKey, ecLeaf, nil, "secret")
		require.NoError(t, err)
		file := filepath.Join(t.TempDir(), "cert.pfx")
		require.NoError(t, os.WriteFile(file, data, 0600))

		_, err = Load(file, "", "wrong")

		assert.ErrorContains(t, err, "failed to load PKCS#12")
	})

	t.Run("a key that doesn't belong to the certificate", func(t *testing.T) {
		certFile := writePEM(t, "cert.pem", &pem.Block{Type: "CERTIFICATE", Bytes: rsaLeaf.Raw})
		keyFile := writePEM(t, "key.pem", &pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1})

		_, err := Load(certFile, keyFile, "")

		assert.ErrorContains(t, err, "private key does not match the certificate")
	})

	t.Run("a PEM file without certificates", func(t *testing.T) {
		certFile := writePEM(t, "cert.pem")
		keyFile := writePEM(t, "key.pem", &pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1})

		_, err := Load(certFile, keyFile, "")

		assert.ErrorContains(t, err, "no certificates found")
	})

	t.Run("missing files", func(t *testing.T) {
		_, err := Load("", "", "")
		assert.EqualError(t, err, "certificate file not specified")
		_, err = Load("cert.pem", "", "")
		assert.EqualError(t, err, "key file required when using PEM certificate")
	})
}
