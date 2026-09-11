package prsmock_test

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/nuts-foundation/nuts-knooppunt/component/pseudonymisation"
	"github.com/nuts-foundation/nuts-knooppunt/mock-components/prs"
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
// It is the Knooppunt's own client step, not a copy of it, so a change there
// shows up here.
func blind(t *testing.T, input []byte) (blindedInput string, blindFactor string) {
	t.Helper()
	element, scalar, err := pseudonymisation.BlindInput(input)
	require.NoError(t, err)
	return base64.URLEncoding.EncodeToString(element), base64.URLEncoding.EncodeToString(scalar)
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
	client := &http.Client{}
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

	t.Run("de-blinding fails where liboprf's oprf_Unblind fails", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, blindFactor := blind(t, []byte("input"))
		status, body := postEval(t, prs, evalRequest(blindedInput, "ura:"+recipientURA, recipientScope))
		require.Equal(t, http.StatusOK, status, body)
		var response struct {
			JWE string `json:"jwe"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &response))
		payload, err := prs.DecryptJWE(response.JWE)
		require.NoError(t, err)
		var claims struct {
			Subject string `json:"subject"`
		}
		require.NoError(t, json.Unmarshal(payload, &claims))
		evaluated := strings.TrimPrefix(claims.Subject, "pseudonym:eval:")
		zero := base64.URLEncoding.EncodeToString(make([]byte, 32))

		_, err = prsmock.Finalize(zero, evaluated)
		assert.ErrorContains(t, err, "blind factor is zero")
		_, err = prsmock.Finalize(blindFactor, zero)
		assert.ErrorContains(t, err, "identity")
		_, err = prsmock.Finalize(blindFactor, evaluated)
		assert.NoError(t, err)
	})

	t.Run("accepts the standard base64 alphabet like Python's urlsafe decoder", func(t *testing.T) {
		prs := newRegisteredService(t)
		// Keep blinding until the standard encoding differs from base64url, so
		// the request really exercises the tolerance. About three in four
		// elements qualify, so the bound is never reached in practice.
		var element []byte
		for attempt := 0; ; attempt++ {
			require.Less(t, attempt, 100, "no element with + or / in its standard encoding")
			var err error
			element, _, err = pseudonymisation.BlindInput([]byte("input"))
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
		client := &http.Client{}

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

	t.Run("answers any other path with FastAPI's 404", func(t *testing.T) {
		prs := newRegisteredService(t)
		client := &http.Client{}
		request := newEvalRequest(t, prs, evalRequest("AA", "ura:"+recipientURA, recipientScope))
		request.URL.Path = "/evaluate"
		status, contentType, body := send(t, client, request)
		assert.Equal(t, http.StatusNotFound, status)
		assert.Equal(t, "application/json", contentType)
		assert.Equal(t, `{"detail":"Not Found"}`, body)
		require.Equal(t, 1, prs.ExchangeCount())
		assert.Equal(t, "/evaluate", prs.GetLastExchange().Path)
	})

	t.Run("records every exchange", func(t *testing.T) {
		prs := newRegisteredService(t)
		client := &http.Client{}
		body := evalRequest("AA", "ura:12345678", recipientScope)
		send(t, client, newEvalRequest(t, prs, body))
		require.Equal(t, 1, prs.ExchangeCount())
		exchange := prs.GetLastExchange()
		assert.Equal(t, http.MethodPost, exchange.Method)
		assert.Equal(t, "/oprf/eval", exchange.Path)
		assert.Equal(t, "application/json", exchange.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer "+accessToken, exchange.Header.Get("Authorization"))
		assert.Equal(t, body, string(exchange.Body))
		assert.Equal(t, http.StatusNotFound, exchange.Status)
		assert.Equal(t, `{"error":"No organization found for this ura"}`, string(exchange.Response))
	})
}

// TestService_Validation pins the mock against FastAPI 0.129.2 and pydantic
// 2.12.5, the versions v0.0.18 locks, running the v0.0.18 BlindRequest verbatim
// behind a bearer dependency; docs/prs-contract.md says how the answers were
// captured. Expected bodies of rejected inputs are FastAPI's with pydantic's
// "input" and "ctx" echoes and the JSON decode position removed. Inputs that
// FastAPI's stub route accepted go on to the mock's real evaluation: three
// rows expect a JWE, and the unpadded element expects the 400 that v0.0.18's
// evaluation gives it (oprf_service.py:45).
func TestService_Validation(t *testing.T) {
	prs := prsmock.NewService(t)
	prs.RegisterRecipient("1", "s1")
	element, _ := blind(t, []byte("input"))
	valid := func(element string) string {
		return `{"encryptedPersonalId":"` + element + `","recipientOrganization":"ura:1","recipientScope":"s1"}`
	}
	const (
		missingBody = `{"detail":[{"type":"missing","loc":["body"],"msg":"Field required"}]}`
		notAnObject = `{"detail":[{"type":"model_attributes_type","loc":["body"],"msg":"Input should be a valid dictionary or object to extract fields from"}]}`
		jsonInvalid = `{"detail":[{"type":"json_invalid","loc":["body"],"msg":"JSON decode error"}]}`
		noBearer    = `{"detail":"Missing bearer token"}`
		tooShort    = `{"type":"string_too_short","loc":["body","%s"],"msg":"String should have at least 2 characters"}`
	)
	for _, tc := range []struct {
		name        string
		body        string
		bearer      bool
		contentType string
		status      int
		response    string // empty: a JWE is expected
	}{
		{"valid", valid(element), true, "application/json", 200, ""},
		{"null body", `null`, true, "application/json", 422, missingBody},
		{"empty body", ``, true, "application/json", 422, missingBody},
		{"array body", `[]`, true, "application/json", 422, notAnObject},
		{"string body", `"x"`, true, "application/json", 422, notAnObject},
		{"number body", `1`, true, "application/json", 422, notAnObject},
		{"empty object", `{}`, true, "application/json", 422, `{"detail":[{"type":"missing","loc":["body","encryptedPersonalId"],"msg":"Field required"},{"type":"missing","loc":["body","recipientOrganization"],"msg":"Field required"},{"type":"missing","loc":["body","recipientScope"],"msg":"Field required"}]}`},
		{"one missing", `{"encryptedPersonalId":"AAAA","recipientOrganization":"ura:1"}`, true, "application/json", 422, `{"detail":[{"type":"missing","loc":["body","recipientScope"],"msg":"Field required"}]}`},
		{"number member", `{"encryptedPersonalId":1,"recipientOrganization":"ura:1","recipientScope":"s1"}`, true, "application/json", 422, `{"detail":[{"type":"string_type","loc":["body","encryptedPersonalId"],"msg":"Input should be a valid string"}]}`},
		{"null member", `{"encryptedPersonalId":null,"recipientOrganization":"ura:1","recipientScope":"s1"}`, true, "application/json", 422, `{"detail":[{"type":"string_type","loc":["body","encryptedPersonalId"],"msg":"Input should be a valid string"}]}`},
		{"object member", `{"encryptedPersonalId":{},"recipientOrganization":"ura:1","recipientScope":"s1"}`, true, "application/json", 422, `{"detail":[{"type":"string_type","loc":["body","encryptedPersonalId"],"msg":"Input should be a valid string"}]}`},
		{"bool member", `{"encryptedPersonalId":true,"recipientOrganization":"ura:1","recipientScope":"s1"}`, true, "application/json", 422, `{"detail":[{"type":"string_type","loc":["body","encryptedPersonalId"],"msg":"Input should be a valid string"}]}`},
		{"all one character", `{"encryptedPersonalId":"A","recipientOrganization":"u","recipientScope":"s"}`, true, "application/json", 422, `{"detail":[` + fmt.Sprintf(tooShort, "encryptedPersonalId") + `,` + fmt.Sprintf(tooShort, "recipientOrganization") + `,` + fmt.Sprintf(tooShort, "recipientScope") + `]}`},
		{"all empty strings", `{"encryptedPersonalId":"","recipientOrganization":"","recipientScope":""}`, true, "application/json", 422, `{"detail":[` + fmt.Sprintf(tooShort, "encryptedPersonalId") + `,` + fmt.Sprintf(tooShort, "recipientOrganization") + `,` + fmt.Sprintf(tooShort, "recipientScope") + `]}`},
		{"one non-ASCII character is short, not invalid base64", `{"encryptedPersonalId":"é","recipientOrganization":"ura:1","recipientScope":"s1"}`, true, "application/json", 422, `{"detail":[` + fmt.Sprintf(tooShort, "encryptedPersonalId") + `]}`},
		{"base64 with five data characters", `{"encryptedPersonalId":"AAAAA","recipientOrganization":"ura:1","recipientScope":"s1"}`, true, "application/json", 422, `{"detail":[{"type":"value_error","loc":["body","encryptedPersonalId"],"msg":"Value error, must be base64url: Invalid base64-encoded string: number of data characters (5) cannot be 1 more than a multiple of 4"}]}`},
		{"base64 padding interrupted by data", `{"encryptedPersonalId":"AA=A","recipientOrganization":"ura:1","recipientScope":"s1"}`, true, "application/json", 422, `{"detail":[{"type":"value_error","loc":["body","encryptedPersonalId"],"msg":"Value error, must be base64url: Incorrect padding"}]}`},
		{"base64 non-ASCII", `{"encryptedPersonalId":"AAé","recipientOrganization":"ura:1","recipientScope":"s1"}`, true, "application/json", 422, `{"detail":[{"type":"value_error","loc":["body","encryptedPersonalId"],"msg":"Value error, must be base64url: string argument should contain only ASCII characters"}]}`},
		{"base64 garbage that leaves the padding short", `{"encryptedPersonalId":"AA!!","recipientOrganization":"ura:1","recipientScope":"s1"}`, true, "application/json", 422, `{"detail":[{"type":"value_error","loc":["body","encryptedPersonalId"],"msg":"Value error, must be base64url: Incorrect padding"}]}`},
		{"unpadded element passes validation and fails evaluation", valid(strings.TrimRight(element, "=")), true, "application/json", 400, `{"error":"Unable to evaluate blind"}`},
		{"unknown member ignored", `{"encryptedPersonalId":"` + element + `","recipientOrganization":"ura:1","recipientScope":"s1","extra":1}`, true, "application/json", 200, ""},
		{"malformed JSON", `{"encryptedPersonalId":`, true, "application/json", 422, jsonInvalid},
		{"malformed JSON, missing delimiter", `{"a" 1}`, true, "application/json", 422, jsonInvalid},
		{"malformed JSON without bearer", `{"encryptedPersonalId":`, false, "application/json", 422, jsonInvalid},
		{"valid without bearer", valid(element), false, "application/json", 401, noBearer},
		{"empty object without bearer", `{}`, false, "application/json", 401, noBearer},
		{"null body without bearer", `null`, false, "application/json", 401, noBearer},
		{"empty body without bearer", ``, false, "application/json", 401, noBearer},
		{"no Content-Type", valid(element), true, "", 200, ""},
		{"text/plain Content-Type", valid(element), true, "text/plain", 422, notAnObject},
		{"no ura prefix", `{"encryptedPersonalId":"AAAA","recipientOrganization":"oin:1","recipientScope":"s1"}`, true, "application/json", 400, `{"error":"Invalid recipient organization. Format: ura:<ura_number>"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{}
			request, err := http.NewRequest(http.MethodPost, prs.GetURL()+"/oprf/eval", strings.NewReader(tc.body))
			require.NoError(t, err)
			if tc.contentType != "" {
				request.Header.Set("Content-Type", tc.contentType)
			}
			if tc.bearer {
				request.Header.Set("Authorization", "Bearer "+accessToken)
			}
			status, _, body := send(t, client, request)
			assert.Equal(t, tc.status, status, body)
			if tc.response == "" {
				assert.True(t, strings.HasPrefix(body, `{"jwe":"`), body)
			} else {
				assert.Equal(t, tc.response, body)
			}
		})
	}
}

func TestService_Options(t *testing.T) {
	t.Run("uses the configured OPRF key", func(t *testing.T) {
		key, err := hex.DecodeString("0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f0f")
		require.NoError(t, err)
		first, err := prsmock.New(prsmock.Options{ListenAddr: "localhost:0", OPRFKey: key})
		require.NoError(t, err)
		t.Cleanup(func() { _ = first.Stop() })
		second, err := prsmock.New(prsmock.Options{ListenAddr: "localhost:0", OPRFKey: key})
		require.NoError(t, err)
		t.Cleanup(func() { _ = second.Stop() })
		a, err := first.Pseudonym([]byte("input"))
		require.NoError(t, err)
		b, err := second.Pseudonym([]byte("input"))
		require.NoError(t, err)
		assert.Equal(t, a, b, "the same key yields the same pseudonyms across instances")
	})

	t.Run("accepts the keys NewOPRFKey draws", func(t *testing.T) {
		for i := 0; i < 8; i++ {
			key, err := prsmock.NewOPRFKey()
			require.NoError(t, err)
			require.Len(t, key, 32)
			prs, err := prsmock.New(prsmock.Options{ListenAddr: "localhost:0", OPRFKey: key})
			require.NoError(t, err)
			_ = prs.Stop()
		}
	})

	t.Run("refuses a public URL that is not a bare origin", func(t *testing.T) {
		for name, public := range map[string]string{
			"no scheme":    "mock-prs:8080",
			"other scheme": "ftp://mock-prs:8080",
			"path":         "http://mock-prs:8080/prs",
			"userinfo":     "http://user@mock-prs:8080",
			"slash only":   "/",
			"empty query":  "http://mock-prs:8080?",
			"double slash": "http://mock-prs:8080//",
		} {
			_, err := prsmock.New(prsmock.Options{ListenAddr: "localhost:0", PublicURL: public})
			assert.Error(t, err, name)
		}
	})

	t.Run("refuses an OPRF key that is zero, non-canonical or the wrong size", func(t *testing.T) {
		for name, key := range map[string]string{
			"zero":            "0000000000000000000000000000000000000000000000000000000000000000",
			"above the order": "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f89",
			"too short":       "0102",
		} {
			raw, err := hex.DecodeString(key)
			require.NoError(t, err)
			_, err = prsmock.New(prsmock.Options{ListenAddr: "localhost:0", OPRFKey: raw})
			assert.Error(t, err, name)
		}
	})
}

func TestService_Transport(t *testing.T) {
	t.Run("serves a plain HTTP URL, since mTLS is terminated in front of it", func(t *testing.T) {
		prs := newRegisteredService(t)
		assert.True(t, strings.HasPrefix(prs.GetURL(), "http://"), prs.GetURL())
	})

	t.Run("refuses a request without a bearer token with 401", func(t *testing.T) {
		prs := newRegisteredService(t)
		blindedInput, _ := blind(t, []byte("input"))
		client := &http.Client{}
		request := newEvalRequest(t, prs, evalRequest(blindedInput, "ura:"+recipientURA, recipientScope))
		request.Header.Del("Authorization")
		status, contentType, body := send(t, client, request)
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.Equal(t, "application/json", contentType)
		assert.Equal(t, `{"detail":"Missing bearer token"}`, body)
		assert.Equal(t, http.StatusUnauthorized, prs.GetLastExchange().Status)
	})

}

// newStrictService starts a mock that only accepts bearer tokens its own
// token endpoint issued, as the sandbox runs it.
func newStrictService(t *testing.T) *prsmock.Service {
	t.Helper()
	prs, err := prsmock.New(prsmock.Options{ListenAddr: "localhost:0", SandboxListenAddr: "localhost:0"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = prs.Stop() })
	prs.RegisterRecipient(recipientURA, recipientScope)
	return prs
}

const callerURA = "00000030"

func postForm(t *testing.T, client *http.Client, target string, form url.Values) (int, map[string]any) {
	t.Helper()
	response, err := client.PostForm(target, form)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(body, &parsed), string(body))
	return response.StatusCode, parsed
}

func postJSON(t *testing.T, target string, body string) (int, map[string]any) {
	t.Helper()
	response, err := http.Post(target, "application/json", strings.NewReader(body))
	require.NoError(t, err)
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "application/json", response.Header.Get("Content-Type"))
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed), string(raw))
	return response.StatusCode, parsed
}

func TestService_TokenEndpoint(t *testing.T) {
	// issue posts the ministry's documented token request, with form applied
	// to it first, and returns the response.
	issue := func(t *testing.T, prs *prsmock.Service, form func(url.Values)) (int, map[string]any) {
		t.Helper()
		values := url.Values{
			"grant_type":      {"client_credentials"},
			"scope":           {recipientScope},
			"target_audience": {prs.GetURL()},
		}
		if form != nil {
			form(values)
		}
		return postForm(t, &http.Client{}, prs.GetURL()+"/oauth/token", values)
	}
	tokenFor := func(t *testing.T, prs *prsmock.Service, target string) string {
		t.Helper()
		status, response := issue(t, prs, func(v url.Values) { v.Set("target_audience", target) })
		require.Equal(t, http.StatusOK, status, response)
		token, _ := response["access_token"].(string)
		require.NotEmpty(t, token)
		return token
	}
	evalWith := func(t *testing.T, prs *prsmock.Service, token string, host string) (int, string) {
		t.Helper()
		blindedInput, _ := blind(t, []byte("input"))
		request := newEvalRequest(t, prs, evalRequest(blindedInput, "ura:"+recipientURA, recipientScope))
		request.Header.Set("Authorization", "Bearer "+token)
		if host != "" {
			request.Host = host
		}
		status, _, body := send(t, &http.Client{}, request)
		return status, body
	}

	t.Run("issues a token for the documented request, which the PRS route then accepts", func(t *testing.T) {
		prs := newStrictService(t)
		status, response := issue(t, prs, nil)
		require.Equal(t, http.StatusOK, status, response)
		assert.Equal(t, "Bearer", response["token_type"])
		assert.EqualValues(t, 300, response["expires_in"])
		token, _ := response["access_token"].(string)
		require.NotEmpty(t, token)
		assert.True(t, prs.TokenIssued(token))

		status, body := evalWith(t, prs, token, "")
		assert.Equal(t, http.StatusOK, status, body)
	})

	t.Run("ignores the client assertion component/authn adds, which the ministry does not document", func(t *testing.T) {
		prs := newStrictService(t)
		status, response := issue(t, prs, func(v url.Values) {
			v.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
			v.Set("client_assertion", "not.a.jwt")
		})
		assert.Equal(t, http.StatusOK, status, response)
	})

	t.Run("refuses a bearer it did not issue with 401", func(t *testing.T) {
		prs := newStrictService(t)
		status, body := evalWith(t, prs, accessToken, "")
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.Equal(t, `{"detail":"Invalid token"}`, body)
	})

	t.Run("refuses an issued token requested for another target audience", func(t *testing.T) {
		prs := newStrictService(t)
		for name, target := range map[string]string{
			"other service": "https://other-service.example",
			// A URL whose authority is other-service.example; a prefix
			// comparison would accept it.
			"lookalike":   "http://" + strings.TrimPrefix(prs.GetURL(), "http://") + "@other-service.example",
			"empty query": prs.GetURL() + "?",
		} {
			status, body := evalWith(t, prs, tokenFor(t, prs, target), "")
			assert.Equal(t, http.StatusUnauthorized, status, name)
			assert.Equal(t, `{"detail":"Invalid token"}`, body, name)
		}
	})

	t.Run("checks the target against its configured origin, not the request's Host header", func(t *testing.T) {
		prs := newStrictService(t)
		token := tokenFor(t, prs, "https://other-service.example")
		status, body := evalWith(t, prs, token, "other-service.example")
		assert.Equal(t, http.StatusUnauthorized, status)
		assert.Equal(t, `{"detail":"Invalid token"}`, body)
	})

	t.Run("refuses a request without scope or target audience", func(t *testing.T) {
		prs := newStrictService(t)
		for name, form := range map[string]func(url.Values){
			"missing scope":  func(v url.Values) { v.Del("scope") },
			"missing target": func(v url.Values) { v.Del("target_audience") },
		} {
			status, response := issue(t, prs, form)
			assert.Equal(t, http.StatusBadRequest, status, name)
			assert.Equal(t, "invalid_request", response["error"], name)
		}
	})

	t.Run("refuses a grant that is not client_credentials", func(t *testing.T) {
		prs := newStrictService(t)
		status, response := issue(t, prs, func(v url.Values) { v.Set("grant_type", "authorization_code") })
		assert.Equal(t, http.StatusBadRequest, status)
		assert.Equal(t, "unsupported_grant_type", response["error"])
	})
}

// identifierValue is the Knooppunt's NVI identifier value before base64url.
type identifierValue struct {
	BlindFactor     []byte `json:"blind_factor"`
	EvaluatedOutput string `json:"evaluated_output"`
}

func TestService_Sandbox(t *testing.T) {
	t.Run("finalize turns a Knooppunt identifier value into the pseudonym of the input", func(t *testing.T) {
		prs := newRegisteredService(t)
		input := []byte("hkdf output stands here")
		blindedInput, blindFactor := blind(t, input)
		status, body := postEval(t, prs, evalRequest(blindedInput, "ura:"+recipientURA, recipientScope))
		require.Equal(t, http.StatusOK, status, body)
		var response struct {
			JWE string `json:"jwe"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &response))
		scalar, err := base64.URLEncoding.DecodeString(blindFactor)
		require.NoError(t, err)
		value, err := json.Marshal(identifierValue{BlindFactor: scalar, EvaluatedOutput: response.JWE})
		require.NoError(t, err)
		token := base64.RawURLEncoding.EncodeToString(value)

		status, result := postJSON(t, prs.SandboxURL()+"/sandbox/finalize", `{"token":`+strconv.Quote(token)+`}`)
		require.Equal(t, http.StatusOK, status, result)
		expected, err := prs.Pseudonym(input)
		require.NoError(t, err)
		assert.Equal(t, hex.EncodeToString(expected), result["pseudonym"])
	})

	t.Run("tokenize yields an identifier value for the audience that finalizes to the same pseudonym", func(t *testing.T) {
		prs := newRegisteredService(t)
		expected, err := prs.Pseudonym([]byte("input"))
		require.NoError(t, err)
		pseudonym := hex.EncodeToString(expected)

		status, result := postJSON(t, prs.SandboxURL()+"/sandbox/tokenize", `{"pseudonym":`+strconv.Quote(pseudonym)+`,"audience":"`+recipientURA+`"}`)
		require.Equal(t, http.StatusOK, status, result)
		token, _ := result["token"].(string)
		decoded, err := base64.RawURLEncoding.DecodeString(token)
		require.NoError(t, err, "unpadded base64url like the Knooppunt's")
		var members map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(decoded, &members))
		assert.ElementsMatch(t, []string{"blind_factor", "evaluated_output"}, keys(members))
		var jwe string
		require.NoError(t, json.Unmarshal(members["evaluated_output"], &jwe))
		payload, err := prs.DecryptJWE(jwe)
		require.NoError(t, err)
		var claims map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(payload, &claims))
		assert.Equal(t, `"ura:`+recipientURA+`"`, string(claims["aud"]))
		assert.Equal(t, `"nationale-verwijsindex"`, string(claims["scope"]))

		status, result = postJSON(t, prs.SandboxURL()+"/sandbox/finalize", `{"token":`+strconv.Quote(token)+`}`)
		require.Equal(t, http.StatusOK, status, result)
		assert.Equal(t, pseudonym, result["pseudonym"])
	})

	t.Run("rejects malformed input with 400", func(t *testing.T) {
		prs := newRegisteredService(t)
		status, result := postJSON(t, prs.SandboxURL()+"/sandbox/finalize", `{"token":"!!!"}`)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.NotEmpty(t, result["error"])
		status, result = postJSON(t, prs.SandboxURL()+"/sandbox/tokenize", `{"pseudonym":"zz","audience":"1"}`)
		assert.Equal(t, http.StatusBadRequest, status)
		assert.NotEmpty(t, result["error"])
	})
}

func keys(m map[string]json.RawMessage) []string {
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, key)
	}
	return result
}
