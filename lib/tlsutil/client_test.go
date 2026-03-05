package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
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

func TestLoadClientCertificate(t *testing.T) {
	t.Run("loads PEM certificate successfully", func(t *testing.T) {
		// Setup: create temporary PEM cert and key files
		certFile, keyFile := createTestPEMCertificate(t)

		cert, err := LoadClientCertificate(certFile, keyFile, "")

		require.NoError(t, err)
		assert.NotNil(t, cert.Leaf)
		assert.Equal(t, "CN=Test Certificate", cert.Leaf.Subject.String())
		assert.NotEmpty(t, cert.Certificate)
		assert.NotNil(t, cert.PrivateKey)
	})

	t.Run("loads PKCS#12 certificate successfully (.p12)", func(t *testing.T) {
		// Setup: create temporary PKCS#12 file
		p12File := createTestPKCS12Certificate(t, ".p12", "testpassword")

		cert, err := LoadClientCertificate(p12File, "", "testpassword")

		require.NoError(t, err)
		assert.NotNil(t, cert.Leaf)
		assert.Equal(t, "CN=Test Certificate", cert.Leaf.Subject.String())
		assert.NotEmpty(t, cert.Certificate)
		assert.NotNil(t, cert.PrivateKey)
	})

	t.Run("loads PKCS#12 certificate successfully (.pfx)", func(t *testing.T) {
		// Setup: create temporary PKCS#12 file with .pfx extension
		pfxFile := createTestPKCS12Certificate(t, ".pfx", "testpassword")

		cert, err := LoadClientCertificate(pfxFile, "", "testpassword")

		require.NoError(t, err)
		assert.NotNil(t, cert.Leaf)
		assert.NotEmpty(t, cert.Certificate)
	})

	t.Run("returns error when certificate file not specified", func(t *testing.T) {
		cert, err := LoadClientCertificate("", "", "")

		assert.EqualError(t, err, "certificate file not specified")
		assert.Empty(t, cert.Certificate)
	})

	t.Run("returns error when PEM key file not specified", func(t *testing.T) {
		// Setup: create temporary PEM cert file
		certFile, _ := createTestPEMCertificate(t)

		cert, err := LoadClientCertificate(certFile, "", "")

		assert.EqualError(t, err, "key file required when using PEM certificate")
		assert.Empty(t, cert.Certificate)
	})

	t.Run("returns error when PKCS#12 password is incorrect", func(t *testing.T) {
		// Setup: create PKCS#12 file with password
		p12File := createTestPKCS12Certificate(t, ".p12", "correctpassword")

		_, err := LoadClientCertificate(p12File, "", "wrongpassword")

		assert.ErrorContains(t, err, "failed to load PKCS#12")
	})
}

// createTestCertificate generates a test certificate and private key
func createTestCertificate(t *testing.T) (*ecdsa.PrivateKey, *x509.Certificate) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "Test Certificate",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	return privateKey, cert
}

// createTestPEMCertificate generates a test certificate and key in PEM format
func createTestPEMCertificate(t *testing.T) (certFile, keyFile string) {
	privateKey, cert := createTestCertificate(t)

	tmpDir := t.TempDir()
	certFile = filepath.Join(tmpDir, "cert.pem")
	keyFile = filepath.Join(tmpDir, "key.pem")

	certOut, err := os.Create(certFile)
	require.NoError(t, err)
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	require.NoError(t, err)
	certOut.Close()

	keyOut, err := os.Create(keyFile)
	require.NoError(t, err)
	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	require.NoError(t, err)
	err = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privateKeyBytes})
	require.NoError(t, err)
	keyOut.Close()

	return certFile, keyFile
}

// createTestPKCS12Certificate generates a test certificate in PKCS#12 format
func createTestPKCS12Certificate(t *testing.T, extension, password string) string {
	privateKey, cert := createTestCertificate(t)

	pfxData, err := pkcs12.Encode(rand.Reader, privateKey, cert, nil, password)
	require.NoError(t, err)

	tmpDir := t.TempDir()
	p12File := filepath.Join(tmpDir, "cert"+extension)
	err = os.WriteFile(p12File, pfxData, 0600)
	require.NoError(t, err)

	return p12File
}
