package harness

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	dezimock "github.com/nuts-foundation/nuts-knooppunt/mock-components/dezi"
	"github.com/stretchr/testify/require"
)

// MockDezi is the signing half of the mock Dezi: a key for this run and the
// HTTPS address of its JWK set, which is the jku every attestation carries.
type MockDezi struct {
	Signer *dezimock.Signer
	JKU    string
}

// StartMockDezi serves a JWK set over TLS and returns a signer whose
// attestations name it. It must run before the Nuts node is configured: the node
// accepts a jku only if the whole URL, port included, is in its allowed list.
//
// The node fetches the set through http.DefaultClient, so that client is
// replaced, for the life of the test, by one that trusts the test CA. Setting
// SSL_CERT_FILE is not enough: Go reads the system roots once per process, and
// another test may have read them already. Tests using this do not run in
// parallel.
func StartMockDezi(t *testing.T) MockDezi {
	t.Helper()
	certsDir := PEPCertsDir(t)
	leaf, pool := issueLoopbackCert(t, certsDir)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	var signer *dezimock.Signer
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dezi/jwks.json" {
			http.NotFound(w, r)
			return
		}
		set, err := signer.JWKSet()
		if err != nil {
			http.Error(w, "jwks unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(set)
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{leaf}}
	server.StartTLS()
	t.Cleanup(server.Close)

	jku := server.URL + "/dezi/jwks.json"
	signer = dezimock.NewSigner(key, "sandbox-dezi-1", "https://abonnee.dezi.nl", jku)

	previous := http.DefaultClient
	http.DefaultClient = &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
	}
	t.Cleanup(func() { http.DefaultClient = previous })

	return MockDezi{Signer: signer, JKU: jku}
}

// issueLoopbackCert issues a server certificate for the loopback addresses from
// the committed test CA, in memory, and returns it with a pool holding that CA.
func issueLoopbackCert(t *testing.T, certsDir string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	caPEM, err := os.ReadFile(filepath.Join(certsDir, "ca.pem"))
	require.NoError(t, err)
	caBlock, _ := pem.Decode(caPEM)
	require.NotNil(t, caBlock, "ca.pem holds no PEM block")
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	require.NoError(t, err)

	keyPEM, err := os.ReadFile(filepath.Join(certsDir, "ca.key"))
	require.NoError(t, err)
	keyBlock, _ := pem.Decode(keyPEM)
	require.NotNil(t, keyBlock, "ca.key holds no PEM block")
	var caKey crypto.Signer
	if parsed, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes); err == nil {
		signer, ok := parsed.(crypto.Signer)
		require.True(t, ok, "the CA key cannot sign")
		caKey = signer
	} else {
		rsaKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
		require.NoError(t, err, "ca.key is neither PKCS#8 nor PKCS#1")
		caKey = rsaKey
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &leafKey.PublicKey, caKey)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(ca)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: leafKey}, pool
}
