package prsmock_test

import (
	"bytes"
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

func postEval(t *testing.T, prs *prsmock.Service, body string) (int, string) {
	t.Helper()
	response, err := http.Post(prs.GetURL()+"/oprf/eval", "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	return response.StatusCode, string(responseBody)
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

	t.Run("rejects a body without the v0.0.18 field names with 422", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		status, body := postEval(t, prs, `{"blinded_input":"`+blindedInput+`","recipient_organization":"ura:`+recipientURA+`","recipient_scope":"`+recipientScope+`"}`)
		assert.Equal(t, http.StatusUnprocessableEntity, status)
		for _, field := range []string{"encryptedPersonalId", "recipientOrganization", "recipientScope"} {
			assert.Contains(t, body, `{"type":"missing","loc":["body","`+field+`"],"msg":"Field required"}`)
		}
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

	t.Run("records every exchange", func(t *testing.T) {
		prs := newRegisteredService(t)
		body := evalRequest("AA", "ura:12345678", recipientScope)
		postEval(t, prs, body)
		require.Equal(t, 1, prs.ExchangeCount())
		exchange := prs.GetLastExchange()
		assert.Equal(t, http.MethodPost, exchange.Method)
		assert.Equal(t, "/oprf/eval", exchange.Path)
		assert.Equal(t, "application/json", exchange.Header.Get("Content-Type"))
		assert.Equal(t, body, string(exchange.Body))
		assert.Equal(t, http.StatusNotFound, exchange.Status)
		assert.Equal(t, `{"error":"No organization found for this ura"}`, string(exchange.Response))
	})
}

func keys(m map[string]json.RawMessage) []string {
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, key)
	}
	return result
}
