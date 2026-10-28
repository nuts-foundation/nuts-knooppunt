package lrza

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	lrzacomponent "github.com/nuts-foundation/nuts-knooppunt/component/lrza"
	"github.com/nuts-foundation/nuts-knooppunt/test/e2e/harness"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/lrza"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Test_MockLRZA checks the mock LRZA (mock-components/lrza) itself enforces the rules the
// registration test relies on, by writing to it directly: as the vendor operating the Knooppunt
// (LRZA_CALLER) unless a care provider is named.
func Test_MockLRZA(t *testing.T) {
	h := harness.Start(t)
	sunflowerEndpoint := lrza.CareHomeSunflowerEndpoints()[0]
	endpointPath := "Endpoint/" + *sunflowerEndpoint.Id

	t.Run("a resource is withdrawn, not deleted", func(t *testing.T) {
		resp := lrzaDirect(t, h, http.MethodDelete, endpointPath, "", "", nil)
		assert.Equal(t, http.StatusMethodNotAllowed, resp.status, string(resp.body))
	})
	t.Run("an update names the version it replaces", func(t *testing.T) {
		resp := lrzaDirect(t, h, http.MethodPut, endpointPath, "", "", sunflowerEndpoint)
		assert.Equal(t, 428, resp.status, string(resp.body))
	})
	t.Run("an update of a stale version is answered with 412", func(t *testing.T) {
		current := lrzaDirect(t, h, http.MethodGet, endpointPath, "", "", nil)
		require.Equal(t, http.StatusOK, current.status, string(current.body))
		etag := current.header.Get("ETag")

		changed := sunflowerEndpoint
		changed.Address = "https://zonnebloem.example.org/fhir/changed"
		first := lrzaDirect(t, h, http.MethodPut, endpointPath, "", etag, changed)
		require.Equal(t, http.StatusOK, first.status, string(first.body))

		changed.Address = "https://zonnebloem.example.org/fhir/changed-again"
		stale := lrzaDirect(t, h, http.MethodPut, endpointPath, "", etag, changed)
		assert.Equal(t, http.StatusPreconditionFailed, stale.status, "the IG's 412, not HAPI's 409: %s", stale.body)
	})
	t.Run("a transaction follows the same rules", func(t *testing.T) {
		resp := lrzaDirect(t, h, http.MethodPost, "", "", "", fhir.Bundle{
			Type: fhir.BundleTypeTransaction,
			Entry: []fhir.BundleEntry{{
				Request: &fhir.BundleEntryRequest{Method: fhir.HTTPVerbDELETE, Url: endpointPath},
			}},
		})
		assert.Equal(t, http.StatusMethodNotAllowed, resp.status, string(resp.body))
	})
	t.Run("a vendor writes only for care providers that granted it the mandate", func(t *testing.T) {
		endpoint := lrza.Care2CureEndpoints()[0]
		endpoint.Id = nil
		resp := lrzaDirect(t, h, http.MethodPost, "Endpoint", "", "", endpoint)
		assert.Equal(t, http.StatusForbidden, resp.status, string(resp.body))
	})
	t.Run("a care provider writes its own resources without a mandate", func(t *testing.T) {
		endpoint := lrza.Care2CureEndpoints()[0]
		endpoint.Id = nil
		resp := lrzaDirect(t, h, http.MethodPost, "Endpoint", lrza.CareProviderCaller(lrza.Care2CureURA), "", endpoint)
		assert.Equal(t, http.StatusCreated, resp.status, string(resp.body))
	})
	t.Run("only the care provider grants a mandate", func(t *testing.T) {
		mandate := lrza.Mandate("3a9b8c7d-6e5f-4a1b-9c2d-8e7f6a5b4c3d", *lrza.Care2Cure().Id, lrza.Care2CureURA)

		byVendor := lrzaDirect(t, h, http.MethodPost, "OrganizationAffiliation", "", "", mandate)
		assert.Equal(t, http.StatusForbidden, byVendor.status, string(byVendor.body))
		byAnother := lrzaDirect(t, h, http.MethodPost, "OrganizationAffiliation", lrza.CareProviderCaller(sunflower.URA), "", mandate)
		assert.Equal(t, http.StatusForbidden, byAnother.status, string(byAnother.body))
	})
	t.Run("practitioners are not in the LRZA", func(t *testing.T) {
		resp := lrzaDirect(t, h, http.MethodPost, "Practitioner", "", "", fhir.Practitioner{})
		assert.Equal(t, http.StatusForbidden, resp.status, string(resp.body))
	})
	t.Run("only the operator arms failures", func(t *testing.T) {
		resp := lrzaDirect(t, h, http.MethodPost, "$mock-fail?status=503&count=1", "", "", nil)
		assert.Equal(t, http.StatusForbidden, resp.status, string(resp.body))
		assert.Equal(t, 0, armedFailures(t, h))
	})
	t.Run("an armed failure answers in place of the LRZA", func(t *testing.T) {
		armFailure(t, h, http.StatusTooManyRequests, 1, 7, "")

		resp := lrzaDirect(t, h, http.MethodGet, endpointPath, "", "", nil)
		assert.Equal(t, http.StatusTooManyRequests, resp.status, string(resp.body))
		assert.Equal(t, "7", resp.header.Get("Retry-After"))
		assert.Equal(t, http.StatusOK, lrzaDirect(t, h, http.MethodGet, endpointPath, "", "", nil).status, "used up")
	})
}

type lrzaResponse struct {
	status int
	header http.Header
	body   []byte
}

// lrzaDirect sends a request straight to the mock LRZA, bypassing the Knooppunt, as the vendor or, when
// caller is set, as that caller (a system|value token, see lrza.CareProviderCaller).
func lrzaDirect(t *testing.T, h harness.Details, method, path, caller, ifMatch string, body any) lrzaResponse {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(data)
	}
	target := h.LRZaFHIRBaseURL.String()
	if path != "" {
		target += "/" + path
	}
	request, err := http.NewRequestWithContext(t.Context(), method, target, reader)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/fhir+json")
	if caller != "" {
		request.Header.Set("X-Mock-Caller", caller)
	}
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	response, err := http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return lrzaResponse{status: response.StatusCode, header: response.Header, body: data}
}

// armFailure makes the mock LRZA answer the next count requests (with the given method, or any when
// empty) with status, as the LRZA's rate limiter or an outage would. retryAfter is in seconds; 0 omits
// the header.
func armFailure(t *testing.T, h harness.Details, status, count, retryAfter int, method string) {
	t.Helper()
	query := "status=" + strconv.Itoa(status) + "&count=" + strconv.Itoa(count)
	if retryAfter > 0 {
		query += "&retryAfter=" + strconv.Itoa(retryAfter)
	}
	if method != "" {
		query += "&method=" + method
	}
	resp := mockFail(t, h, http.MethodPost, query)
	require.Equal(t, http.StatusOK, resp.status, string(resp.body))
	t.Cleanup(func() { mockFail(t, h, http.MethodDelete, "") })
}

// armedFailures is how many armed failures the mock LRZA has left to answer.
func armedFailures(t *testing.T, h harness.Details) int {
	t.Helper()
	resp := mockFail(t, h, http.MethodGet, "")
	require.Equal(t, http.StatusOK, resp.status, string(resp.body))
	var state struct {
		Remaining int `json:"remaining"`
	}
	require.NoError(t, json.Unmarshal(resp.body, &state))
	return state.Remaining
}

func mockFail(t *testing.T, h harness.Details, method, query string) lrzaResponse {
	t.Helper()
	target := h.LRZaFHIRBaseURL.String() + "/$mock-fail"
	if query != "" {
		target += "?" + query
	}
	// Not t.Context(): armFailure disarms in a cleanup, when that is cancelled already.
	request, err := http.NewRequestWithContext(context.Background(), method, target, nil)
	require.NoError(t, err)
	response, err := hapi.OperatorHTTPClient().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return lrzaResponse{status: response.StatusCode, header: response.Header, body: data}
}

// signingCertificate writes a self-signed certificate and key for a care provider to sign Provenances
// with. The mock LRZA verifies the signature against it, not its trust: it has no trust anchors.
func signingCertificate(t *testing.T) lrzacomponent.SigningConfig {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Zorgcentrum De Zonnebloem", Organization: []string{"Zorgcentrum De Zonnebloem"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	config := lrzacomponent.SigningConfig{CertFile: filepath.Join(dir, "signing.pem"), KeyFile: filepath.Join(dir, "signing.key")}
	require.NoError(t, os.WriteFile(config.CertFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600))
	require.NoError(t, os.WriteFile(config.KeyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600))
	return config
}
