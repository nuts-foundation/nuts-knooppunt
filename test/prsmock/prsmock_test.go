package prsmock_test

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudflare/circl/oprf"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/nuts-foundation/nuts-knooppunt/test/prsmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	recipientURA   = "90000901"
	recipientScope = "nationale-verwijsindex"
	accessToken    = "access-token-issued-for-the-prs"
)

// blind produces what a client sends the service for input: the blinded
// ristretto255 element (padded base64url, the encoding the service's own
// reference client uses) and the blind factor the recipient needs afterwards.
func blind(t *testing.T, input []byte) (blindedInput string, blindFactor string) {
	t.Helper()
	finalizeData, request, err := oprf.NewClient(oprf.SuiteRistretto255).Blind([][]byte{input})
	require.NoError(t, err)
	element, err := request.Elements[0].MarshalBinary()
	require.NoError(t, err)
	scalar, err := finalizeData.CopyBlinds()[0].MarshalBinary()
	require.NoError(t, err)
	return base64.URLEncoding.EncodeToString(element), base64.URLEncoding.EncodeToString(scalar)
}

// newClient returns what a caller of the mock needs: trust in the mock's
// certificate and a client certificate to present.
func newClient(t *testing.T, prs *prsmock.Service) (*http.Client, tls.Certificate) {
	t.Helper()
	certificate, err := prsmock.NewClientCertificate()
	require.NoError(t, err)
	return &http.Client{Transport: &http.Transport{TLSClientConfig: prs.TLSClientConfig(certificate)}}, certificate
}

func newEvalRequest(t *testing.T, prs *prsmock.Service, body string) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, prs.GetURL()+"/oprf/eval", strings.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	return request
}

func send(t *testing.T, client *http.Client, request *http.Request) (status int, contentType string, body string) {
	t.Helper()
	response, err := client.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, response.Header.Get("Content-Type"), string(responseBody)
}

func postEval(t *testing.T, prs *prsmock.Service, body string) (int, string) {
	t.Helper()
	client, _ := newClient(t, prs)
	status, contentType, responseBody := send(t, client, newEvalRequest(t, prs, body))
	assert.Equal(t, "application/json", contentType)
	return status, responseBody
}

func evalRequest(blindedInput, recipientOrganization, scope string) string {
	return `{"encryptedPersonalId":"` + blindedInput + `","recipientOrganization":"` + recipientOrganization + `","recipientScope":"` + scope + `"}`
}

func newRegisteredService(t *testing.T) *prsmock.Service {
	t.Helper()
	prs := prsmock.NewService(t)
	prs.RegisterRecipient(recipientURA, recipientScope)
	return prs
}

func TestService_Eval(t *testing.T) {
	t.Run("evaluates a blind into a JWE for the recipient", func(t *testing.T) {
		prs := newRegisteredService(t)
		input := []byte("hkdf output stands here")
		blindedInput, blindFactor := blind(t, input)

		status, body := postEval(t, prs, evalRequest(blindedInput, "ura:"+recipientURA, recipientScope))
		require.Equal(t, http.StatusOK, status, body)

		var response map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(body), &response))
		require.Len(t, response, 1, "v0.0.18 answers with the jwe field only")
		var token string
		require.NoError(t, json.Unmarshal(response["jwe"], &token))
		require.Len(t, strings.Split(token, "."), 5, "compact JWE serialization")

		// Protected header as BlindJwe.build sets it (jwe_token.py:24-29).
		protectedHeader, err := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[0])
		require.NoError(t, err)
		var header map[string]any
		require.NoError(t, json.Unmarshal(protectedHeader, &header))
		recipientKey, err := jwk.FromRaw(prs.RecipientPublicKey())
		require.NoError(t, err)
		thumbprint, err := recipientKey.Thumbprint(prsmock.ThumbprintHash)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{
			"alg": "RSA-OAEP-256",
			"enc": "A256GCM",
			"cty": "application/json",
			"kid": base64.RawURLEncoding.EncodeToString(thumbprint),
		}, header)

		// Claims as the recipient sees them after decryption (jwe_token.py:14-22).
		payload, err := prs.DecryptJWE(token)
		require.NoError(t, err)
		var claims map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &claims))
		assert.ElementsMatch(t, []string{"subject", "aud", "scope", "version", "iat", "exp"}, keys(claims))
		assert.Equal(t, `"ura:90000901"`, string(claims["aud"]))
		assert.Equal(t, `"nationale-verwijsindex"`, string(claims["scope"]))
		assert.Equal(t, `"1.1"`, string(claims["version"]))
		var subject string
		require.NoError(t, json.Unmarshal(claims["subject"], &subject))
		require.True(t, strings.HasPrefix(subject, "pseudonym:eval:"), subject)
		evaluated := strings.TrimPrefix(subject, "pseudonym:eval:")
		evaluatedBytes, err := base64.URLEncoding.Strict().DecodeString(evaluated)
		require.NoError(t, err, "evaluation is padded base64url, as Python's urlsafe_b64encode writes it")
		assert.Len(t, evaluatedBytes, 32)

		// De-blinding yields the pseudonym the mock computes for the bare input.
		pseudonym, err := prsmock.Finalize(blindFactor, evaluated)
		require.NoError(t, err)
		expected, err := prs.Pseudonym(input)
		require.NoError(t, err)
		assert.Equal(t, base64.URLEncoding.EncodeToString(expected), pseudonym)
	})

	t.Run("accepts the standard base64 alphabet like Python's urlsafe decoder", func(t *testing.T) {
		prs := newRegisteredService(t)
		// Keep blinding until the standard encoding differs from base64url, so
		// the request really exercises the tolerance.
		var element []byte
		for {
			_, request, err := oprf.NewClient(oprf.SuiteRistretto255).Blind([][]byte{[]byte("input")})
			require.NoError(t, err)
			element, err = request.Elements[0].MarshalBinary()
			require.NoError(t, err)
			if strings.ContainsAny(base64.StdEncoding.EncodeToString(element), "+/") {
				break
			}
		}
		status, body := postEval(t, prs, evalRequest(base64.StdEncoding.EncodeToString(element), "ura:"+recipientURA, recipientScope))
		assert.Equal(t, http.StatusOK, status, body)
	})

	t.Run("discards characters outside the base64 alphabet like Python's decoder", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		// Python's decoder skips these; the validator's padding arithmetic
		// counts them, and the value still decodes to the same 32 bytes. The
		// line break is JSON-escaped so it reaches the decoder as CR LF.
		garbled := blindedInput[:10] + "!" + blindedInput[10:] + `\r\n`
		status, body := postEval(t, prs, evalRequest(garbled, "ura:"+recipientURA, recipientScope))
		assert.Equal(t, http.StatusOK, status, body)
	})

	t.Run("rejects a body without the v0.0.18 field names with 422", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		status, body := postEval(t, prs, `{"blinded_input":"`+blindedInput+`","recipient_organization":"ura:`+recipientURA+`","recipient_scope":"`+recipientScope+`"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, status)
		for _, field := range []string{"encryptedPersonalId", "recipientOrganization", "recipientScope"} {
			assert.Contains(t, body, `{"type":"missing","loc":["body","`+field+`"],"msg":"Field required"}`)
		}
	})

	t.Run("rejects malformed JSON with 422", func(t *testing.T) {
		prs := newRegisteredService(t)
		status, body := postEval(t, prs, `{"encryptedPersonalId":`)
		assert.Equal(t, http.StatusUnprocessableEntity, status)
		assert.Equal(t, `{"detail":[{"type":"json_invalid","loc":["body"],"msg":"JSON decode error"}]}`, body)
	})

	t.Run("rejects a recipient organization without the ura prefix with 400", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		status, body := postEval(t, prs, evalRequest(blindedInput, "oin:00000099000000001000", recipientScope))
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, `{"error":"Invalid recipient organization. Format: ura:<ura_number>"}`, body)
	})

	t.Run("rejects an unknown URA with 404", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		status, body := postEval(t, prs, evalRequest(blindedInput, "ura:12345678", recipientScope))
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, `{"error":"No organization found for this ura"}`, body)
	})

	t.Run("rejects a scope without a registered key with 404", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		status, body := postEval(t, prs, evalRequest(blindedInput, "ura:"+recipientURA, "other-scope"))
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, `{"error":"No public key found for this organization and/or scope"}`, body)
	})

	t.Run("serves any scope with a key registered under *", func(t *testing.T) {
		prs := prsmock.NewService(t)
		prs.RegisterRecipient(recipientURA, "*")
		blindedInput, _ := blind(t, []byte("input"))
		status, body := postEval(t, prs, evalRequest(blindedInput, "ura:"+recipientURA, "any-scope"))
		assert.Equal(t, http.StatusOK, status, body)
	})

	t.Run("rejects garbage that only the validator's padding arithmetic absorbs with 400", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		// The validator pads by the length of the whole string, garbage
		// included, so this passes validation; evaluation decodes the value
		// as sent, without padding, and fails (oprf_service.py:17-26,45).
		garbled := strings.TrimRight(blindedInput, "=") + "!!"
		status, body := postEval(t, prs, evalRequest(garbled, "ura:"+recipientURA, recipientScope))
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, `{"error":"Unable to evaluate blind"}`, body)
	})

	t.Run("rejects an unpadded blinded input with 400", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		status, body := postEval(t, prs, evalRequest(strings.TrimRight(blindedInput, "="), "ura:"+recipientURA, recipientScope))
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, `{"error":"Unable to evaluate blind"}`, body)
	})

	t.Run("rejects bytes that are not a ristretto255 element with 400", func(t *testing.T) {
		prs := newRegisteredService(t)
		notAnElement := base64.URLEncoding.EncodeToString(bytes.Repeat([]byte{0xff}, 32))
		status, body := postEval(t, prs, evalRequest(notAnElement, "ura:"+recipientURA, recipientScope))
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, `{"error":"Unable to evaluate blind"}`, body)
	})

	t.Run("answers with the injected failure until it is cleared", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		client, _ := newClient(t, prs)

		prs.Fail(http.StatusInternalServerError, "Internal Server Error")
		status, contentType, body := send(t, client, newEvalRequest(t, prs, evalRequest(blindedInput, "ura:"+recipientURA, recipientScope)))
		assert.Equal(t, http.StatusInternalServerError, status)
		assert.Equal(t, "text/plain; charset=utf-8", contentType)
		assert.Equal(t, "Internal Server Error", body)
		assert.Equal(t, http.StatusInternalServerError, prs.GetLastExchange().Status)

		prs.Fail(0, "")
		status, _, body = send(t, client, newEvalRequest(t, prs, evalRequest(blindedInput, "ura:"+recipientURA, recipientScope)))
		assert.Equal(t, http.StatusOK, status, body)
	})

	t.Run("records every exchange", func(t *testing.T) {
		prs := newRegisteredService(t)
		client, certificate := newClient(t, prs)
		body := evalRequest("AA", "ura:12345678", recipientScope)
		send(t, client, newEvalRequest(t, prs, body))
		require.Equal(t, 1, prs.ExchangeCount())
		exchange := prs.GetLastExchange()
		assert.Equal(t, http.MethodPost, exchange.Method)
		assert.Equal(t, "/oprf/eval", exchange.Path)
		assert.Equal(t, "application/json", exchange.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer "+accessToken, exchange.Header.Get("Authorization"))
		require.NotNil(t, exchange.ClientCertificate)
		assert.Equal(t, certificate.Leaf.Raw, exchange.ClientCertificate.Raw)
		assert.Equal(t, body, string(exchange.Body))
		assert.Equal(t, http.StatusNotFound, exchange.Status)
		assert.Equal(t, `{"error":"No organization found for this ura"}`, string(exchange.Response))
	})
}

func TestService_Transport(t *testing.T) {
	t.Run("serves an https URL", func(t *testing.T) {
		prs := newRegisteredService(t)
		assert.True(t, strings.HasPrefix(prs.GetURL(), "https://"), prs.GetURL())
	})

	t.Run("refuses a request without a bearer token with 401", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		client, _ := newClient(t, prs)
		request := newEvalRequest(t, prs, evalRequest(blindedInput, "ura:"+recipientURA, recipientScope))
		request.Header.Del("Authorization")
		status, contentType, body := send(t, client, request)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.Equal(t, "application/json", contentType)
		assert.Equal(t, `{"detail":"Missing bearer token"}`, body)
		assert.Equal(t, http.StatusUnauthorized, prs.GetLastExchange().Status)
	})

	t.Run("refuses a handshake without a client certificate", func(t *testing.T) {
		prs := newRegisteredService(t)
		pool := x509.NewCertPool()
		pool.AddCert(prs.ServerCertificate())
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
		_, err := client.Do(newEvalRequest(t, prs, evalRequest("AA", "ura:"+recipientURA, recipientScope)))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "certificate required")
		assert.Equal(t, 0, prs.ExchangeCount())
	})

	t.Run("refuses plaintext HTTP", func(t *testing.T) {
		prs := newRegisteredService(t)
		response, err := http.Post("http://"+strings.TrimPrefix(prs.GetURL(), "https://")+"/oprf/eval", "application/json", strings.NewReader("{}"))
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Contains(t, string(body), "Client sent an HTTP request to an HTTPS server")
		assert.Equal(t, 0, prs.ExchangeCount())
	})
}

func keys(m map[string]json.RawMessage) []string {
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, key)
	}
	return result
}
