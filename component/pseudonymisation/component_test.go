package pseudonymisation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudflare/circl/group"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/test/prsmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const (
	// testBSN is the example identifier on the implementation guide's
	// pseudonymisation page; it is not an issued number.
	testBSN      = "999940003"
	testLocalURA = "00000030"
	// testNVIURA is the NVI's URA on the acceptance environment (docs/CONFIGURATION.md).
	testNVIURA = "90000901"
	testScope  = "nationale-verwijsindex"
)

// tokenRequest captures what the component asks the authn component for.
type tokenRequest struct {
	scope    []string
	ura      string
	audience string
}

func newTestComponent(t *testing.T) (*Component, *prsmock.Service, *tokenRequest) {
	t.Helper()
	prs := prsmock.NewService(t)
	prs.RegisterRecipient(testNVIURA, testScope)
	captured := &tokenRequest{}
	component := New(Config{PRSBaseURL: prs.GetURL()}, func(_ context.Context, scope []string, ura string, audience string) (*http.Client, error) {
		*captured = tokenRequest{scope: scope, ura: ura, audience: audience}
		return http.DefaultClient, nil
	})
	return component, prs, captured
}

func bsnIdentifier() fhir.Identifier {
	return fhir.Identifier{System: to.Ptr(coding.BSNNamingSystem), Value: to.Ptr(testBSN)}
}

func tokenize(t *testing.T, component *Component, scope string) *fhir.Identifier {
	t.Helper()
	result, err := component.IdentifierToToken(t.Context(), bsnIdentifier(), testLocalURA, testNVIURA, scope)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Value)
	return result
}

// decodeToken splits an NVI identifier value into its JSON members.
func decodeToken(t *testing.T, value string) map[string]json.RawMessage {
	t.Helper()
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	require.NoError(t, err, "identifier value must be unpadded base64url")
	var token map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(decoded, &token))
	return token
}

func decryptClaims(t *testing.T, prs *prsmock.Service, token string) map[string]json.RawMessage {
	t.Helper()
	payload, err := prs.DecryptJWE(token)
	require.NoError(t, err)
	var claims map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(payload, &claims))
	return claims
}

func jsonKeys(members map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(members))
	for key := range members {
		keys = append(keys, key)
	}
	return keys
}

func unquote(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var value string
	require.NoError(t, json.Unmarshal(raw, &value), "expected a JSON string, got %s", raw)
	return value
}

func TestComponent_IdentifierToToken(t *testing.T) {
	t.Run("requests an access token for the local organization with scope prs:read", func(t *testing.T) {
		component, prs, captured := newTestComponent(t)
		tokenize(t, component, testScope)

		assert.Equal(t, &tokenRequest{scope: []string{"prs:read"}, ura: testLocalURA, audience: prs.GetURL()}, captured)
	})

	t.Run("posts the v0.0.18 evaluate request", func(t *testing.T) {
		component, prs, _ := newTestComponent(t)
		tokenize(t, component, testScope)

		require.Equal(t, 1, prs.ExchangeCount())
		exchange := prs.GetLastExchange()
		assert.Equal(t, http.MethodPost, exchange.Method)
		assert.Equal(t, "/oprf/eval", exchange.Path)
		assert.Equal(t, "application/json", exchange.Header.Get("Content-Type"))

		var members map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(exchange.Body, &members))
		assert.ElementsMatch(t, []string{"encryptedPersonalId", "recipientOrganization", "recipientScope"}, jsonKeys(members))
		assert.Equal(t, `"ura:90000901"`, string(members["recipientOrganization"]))
		assert.Equal(t, `"nationale-verwijsindex"`, string(members["recipientScope"]))

		// The blinded element goes out as encoding/json writes a []byte:
		// standard alphabet, padded. The service reads it with Python's
		// urlsafe_b64decode, which accepts that; see docs/prs-contract.md.
		blinded := unquote(t, members["encryptedPersonalId"])
		assert.Len(t, blinded, 44)
		assert.True(t, strings.HasSuffix(blinded, "="), "padded")
		element, err := base64.StdEncoding.Strict().DecodeString(blinded)
		require.NoError(t, err, "standard base64 alphabet")
		require.Len(t, element, 32)
		require.NoError(t, group.Ristretto255.NewElement().UnmarshalBinary(element), "a ristretto255 element")

		// RFC 8785 canonical form: members sorted, no whitespace.
		assert.Equal(t, fmt.Sprintf(`{"encryptedPersonalId":%s,"recipientOrganization":"ura:90000901","recipientScope":"nationale-verwijsindex"}`, members["encryptedPersonalId"]), string(exchange.Body))
	})

	t.Run("hands the PRS output to the NVI as base64url JSON", func(t *testing.T) {
		component, prs, _ := newTestComponent(t)
		result := tokenize(t, component, testScope)

		assert.Equal(t, coding.BSNTransportTokenNamingSystem, *result.System)
		assert.NotContains(t, *result.Value, "=", "unpadded")
		token := decodeToken(t, *result.Value)
		assert.ElementsMatch(t, []string{"blind_factor", "evaluated_output"}, jsonKeys(token))

		// evaluated_output is the jwe member of the PRS response, untouched.
		var response map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(prs.GetLastExchange().Response, &response))
		assert.Equal(t, string(response["jwe"]), string(token["evaluated_output"]))

		// blind_factor is the blind scalar as encoding/json writes a []byte:
		// standard alphabet, padded. The NVI's crypto service reads it with
		// Python's urlsafe_b64decode; see docs/prs-contract.md.
		blindFactor := unquote(t, token["blind_factor"])
		assert.Len(t, blindFactor, 44)
		assert.True(t, strings.HasSuffix(blindFactor, "="), "padded")
		scalar, err := base64.StdEncoding.Strict().DecodeString(blindFactor)
		require.NoError(t, err, "standard base64 alphabet")
		require.Len(t, scalar, 32)
		blind := group.Ristretto255.NewScalar()
		require.NoError(t, blind.UnmarshalBinary(scalar))
		canonical, err := blind.MarshalBinary()
		require.NoError(t, err)
		assert.Equal(t, scalar, canonical, "a canonical ristretto255 scalar")
		assert.False(t, blind.IsZero())
	})

	t.Run("yields a token the recipient de-blinds to a stable pseudonym", func(t *testing.T) {
		component, prs, _ := newTestComponent(t)
		first := decodeToken(t, *tokenize(t, component, testScope).Value)
		second := decodeToken(t, *tokenize(t, component, testScope).Value)
		assert.NotEqual(t, first["blind_factor"], second["blind_factor"], "a fresh blind per call")
		assert.NotEqual(t, first["evaluated_output"], second["evaluated_output"])

		pseudonym := finalizeToken(t, prs, first, testScope)
		assert.Equal(t, pseudonym, finalizeToken(t, prs, second, testScope))

		// The stable value is the OPRF of the HKDF-derived input, so it is what
		// any other RFC 9497 client derives for this identifier and recipient.
		derived, err := deriveKey(prsIdentifier{LandCode: "NL", Type: "BSN", Value: testBSN}, testNVIURA, testScope)
		require.NoError(t, err)
		expected, err := prs.Pseudonym(derived)
		require.NoError(t, err)
		assert.Equal(t, base64.URLEncoding.EncodeToString(expected), pseudonym)
	})

	t.Run("binds the pseudonym to the recipient scope", func(t *testing.T) {
		component, prs, _ := newTestComponent(t)
		const otherScope = "other-scope"
		prs.RegisterRecipient(testNVIURA, otherScope)

		nvi := finalizeToken(t, prs, decodeToken(t, *tokenize(t, component, testScope).Value), testScope)
		other := finalizeToken(t, prs, decodeToken(t, *tokenize(t, component, otherScope).Value), otherScope)
		assert.NotEqual(t, nvi, other)
	})

	t.Run("leaves identifiers that are not a BSN untouched", func(t *testing.T) {
		component, prs, _ := newTestComponent(t)
		for name, identifier := range map[string]fhir.Identifier{
			"other system": {System: to.Ptr("http://example.com/other"), Value: to.Ptr("12345")},
			"no system":    {Value: to.Ptr("12345")},
			"no value":     {System: to.Ptr(coding.BSNNamingSystem)},
		} {
			t.Run(name, func(t *testing.T) {
				result, err := component.IdentifierToToken(t.Context(), identifier, testLocalURA, testNVIURA, testScope)
				require.NoError(t, err)
				assert.Equal(t, &identifier, result)
			})
		}
		assert.Equal(t, 0, prs.ExchangeCount())
	})

	t.Run("fails when the PRS does not know the recipient", func(t *testing.T) {
		component, prs, _ := newTestComponent(t)
		result, err := component.IdentifierToToken(t.Context(), bsnIdentifier(), testLocalURA, "12345678", testScope)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "PRS response: non-OK status code (status=404 Not Found")
		assert.Contains(t, err.Error(), `{"error":"No organization found for this ura"}`)
		assert.Equal(t, 1, prs.ExchangeCount())
	})
}

// finalizeToken plays the recipient: decrypt evaluated_output, check the JWE
// was issued for this recipient and scope, and de-blind with blind_factor.
func finalizeToken(t *testing.T, prs *prsmock.Service, token map[string]json.RawMessage, scope string) string {
	t.Helper()
	claims := decryptClaims(t, prs, unquote(t, token["evaluated_output"]))
	assert.Equal(t, `"ura:`+testNVIURA+`"`, string(claims["aud"]))
	assert.Equal(t, fmt.Sprintf("%q", scope), string(claims["scope"]))
	subject := unquote(t, claims["subject"])
	require.True(t, strings.HasPrefix(subject, "pseudonym:eval:"), subject)
	pseudonym, err := prsmock.Finalize(unquote(t, token["blind_factor"]), strings.TrimPrefix(subject, "pseudonym:eval:"))
	require.NoError(t, err)
	return pseudonym
}
