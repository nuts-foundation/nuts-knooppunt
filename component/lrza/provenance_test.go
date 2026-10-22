package lrza

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tlsutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanonicalize(t *testing.T) {
	stored := []byte(`{
		"resourceType": "Location",
		"id": "1",
		"meta": {"versionId": "2", "lastUpdated": "2026-10-20T12:00:00Z"},
		"text": {"status": "generated", "div": "<div>Location</div>"},
		"type": [{"text": "kept: CodeableConcept.text is not a narrative"}],
		"contained": [{"resourceType": "Organization", "id": "c", "meta": {"versionId": "1"}, "text": {"status": "generated", "div": "<div/>"}}],
		"name": "Centrum"
	}`)

	canonical, err := canonicalize(stored)

	require.NoError(t, err)
	assert.Equal(t, `{"contained":[{"id":"c","resourceType":"Organization"}],"id":"1","name":"Centrum","resourceType":"Location","type":[{"text":"kept: CodeableConcept.text is not a narrative"}]}`, string(canonical))
}

// testSigningKey creates an RSA key with a self-signed certificate.
func testSigningKey(t *testing.T) (*rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Sunflower"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	return key, der
}

func TestSigner(t *testing.T) {
	key, der := testSigningKey(t)
	s, err := newSigner(key, [][]byte{der})
	require.NoError(t, err)
	payload := []byte(`{"id":"1","resourceType":"Location"}`)

	compact, err := s.signDetached(payload, testNow)

	require.NoError(t, err)
	verified, err := jws.Verify(compact, jws.WithKey(jwa.RS256, &key.PublicKey), jws.WithDetachedPayload(payload))
	require.NoError(t, err, "the detached signature verifies against the payload")
	assert.Equal(t, payload, verified)
	_, err = jws.Verify(compact, jws.WithKey(jwa.RS256, &key.PublicKey), jws.WithDetachedPayload([]byte(`{"id":"2"}`)))
	assert.Error(t, err, "and not against another payload")

	message, err := jws.Parse(compact)
	require.NoError(t, err)
	headers := message.Signatures()[0].ProtectedHeaders()
	assert.Equal(t, jwa.RS256, headers.Algorithm())
	assert.Equal(t, "JOSE", headers.Type())
	chain := headers.X509CertChain()
	require.Equal(t, 1, chain.Len())
	certificate, _ := chain.Get(0)
	assert.Equal(t, base64.StdEncoding.EncodeToString(der), string(certificate))
	for name, expected := range map[string]any{
		"canon": staticCanonicalization,
		"sigT":  "2026-10-20T12:00:00Z",
		"srCms": []any{map[string]any{"commId": map[string]any{"id": "urn:oid:1.2.840.10065.1.12.1.1", "desc": "Author's Signature"}}},
	} {
		value, ok := headers.Get(name)
		assert.True(t, ok, name)
		assert.Equal(t, expected, value, name)
	}
}

func TestRegistration_Provenance(t *testing.T) {
	key, der := testSigningKey(t)
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600))
	withSigning := func(config *Config) {
		config.Registration.Signing = map[string]tlsutil.Config{sunflowerURA: {TLSCertFile: certFile, TLSKeyFile: keyFile}}
	}

	t.Run("a write is followed by a signed Provenance", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza, withSigning)

		resp := createResource(t, c, sunflowerURA, testLocation("", "sunflower"))

		require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
		assert.Empty(t, resp.Header().Get("Warning"))
		stored := resp.resource(t)
		provenances := lrza.requestsTo(http.MethodPost, "/Provenance")
		require.Len(t, provenances, 1)
		var provenance struct {
			Target []map[string]any `json:"target"`
			Agent  []struct {
				Who map[string]any `json:"who"`
			} `json:"agent"`
			Signature []struct {
				Type         []map[string]any `json:"type"`
				Who          map[string]any   `json:"who"`
				When         string           `json:"when"`
				TargetFormat string           `json:"targetFormat"`
				SigFormat    string           `json:"sigFormat"`
				Data         string           `json:"data"`
			} `json:"signature"`
		}
		require.NoError(t, json.Unmarshal(provenances[0].body, &provenance))
		assert.Equal(t, []map[string]any{{"reference": stored.reference() + "/_history/1"}}, provenance.Target)
		expectedWho := map[string]any{
			"reference":  "Organization/sunflower",
			"type":       "Organization",
			"identifier": map[string]any{"system": coding.URAOIDNamingSystem, "value": sunflowerURA},
		}
		require.Len(t, provenance.Agent, 1)
		assert.Equal(t, expectedWho, provenance.Agent[0].Who)
		require.Len(t, provenance.Signature, 1)
		signature := provenance.Signature[0]
		assert.Equal(t, expectedWho, signature.Who)
		assert.Equal(t, "2026-10-20T12:00:00Z", signature.When)
		assert.Equal(t, coding.AuthorSignatureCode, signature.Type[0]["code"])
		assert.Equal(t, "application/fhir+json;canonicalization=http://hl7.org/fhir/canonicalization/json#static", signature.TargetFormat)
		assert.Equal(t, "application/jose", signature.SigFormat)

		compact, err := base64.StdEncoding.DecodeString(signature.Data)
		require.NoError(t, err)
		canonical, err := canonicalize(resp.Body.Bytes())
		require.NoError(t, err)
		_, err = jws.Verify(compact, jws.WithKey(jwa.RS256, &key.PublicKey), jws.WithDetachedPayload(canonical))
		assert.NoError(t, err, "the signature verifies against the stored resource")
	})

	t.Run("a tenant without a signing key gets no Provenance", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza, withSigning)

		resp := createResource(t, c, care2CureURA, testLocation("", "care2cure"))

		require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
		assert.Empty(t, lrza.requestsTo(http.MethodPost, "/Provenance"))
	})

	t.Run("a signing failure doesn't undo the write, but warns", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza, withSigning)

		// The tenant's Organization disappears between the write and the signing, so the Provenance
		// has no agent to name.
		location := testLocation("", "sunflower")
		c.registrar.now = func() time.Time {
			lrza.mux.Lock()
			delete(lrza.resources, "Organization/sunflower")
			lrza.mux.Unlock()
			return testNow
		}
		resp := createResource(t, c, sunflowerURA, location)

		require.Equal(t, http.StatusCreated, resp.Code, resp.Body.String())
		assert.Contains(t, resp.Header().Get("Warning"), "Provenance signing failed")
	})

	t.Run("each changed entry of a transaction is signed", func(t *testing.T) {
		lrza := newFakeLRZA(t, testDirectory()...)
		c := newTestComponent(t, lrza, withSigning)

		bundle := map[string]any{
			"resourceType": "Bundle", "type": "transaction",
			"entry": []any{
				map[string]any{"resource": testLocation("", "sunflower"), "request": map[string]any{"method": "POST", "url": "Location"}},
				map[string]any{"request": map[string]any{"method": "DELETE", "url": "Endpoint/sunflower-endpoint"}},
			},
		}
		resp := submitTransaction(t, c, sunflowerURA, bundle)

		require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
		assert.Len(t, lrza.requestsTo(http.MethodPost, "/Provenance"), 2)
	})
}
