package pseudonymisation

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudflare/circl/group"
	"github.com/nuts-foundation/nuts-knooppunt/component/authn"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tlsutil"
	"github.com/nuts-foundation/nuts-knooppunt/mock-components/prs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
	"golang.org/x/oauth2"
)

const (
	// testBSN is the example identifier on the implementation guide's
	// pseudonymisation page; it is not an issued number.
	testBSN = "999940003"
	// otherBSN fails the BSN 11-check, like testBSN, so it cannot be an
	// issued number either.
	otherBSN      = "999940005"
	testLocalURA  = "00000030"
	otherLocalURA = "00000031"
	// testNVIURA is the NVI's URA on the acceptance environment (docs/CONFIGURATION.md).
	testNVIURA        = "90000901"
	otherRecipientURA = "90000902"
	testScope         = "nationale-verwijsindex"
	// testAccessToken is what the stubbed authn component's client presents.
	testAccessToken = "access-token-issued-for-the-prs"
)

// providerContextKey marks the context a test hands to the component, so the
// provider stub can tell whether it received that context.
type providerContextKey struct{}

// providerCall records what the component asks the authn component for, and
// the credentials the client it got back carries.
type providerCall struct {
	scope     []string
	ura       string
	audience  string
	ctxMarker any
	// transport is the returned client's base transport, which records
	// whether the component closed the response body.
	transport *closeTracking
	// clientCertificate is what the returned client presents on the TLS
	// handshake, next to testAccessToken in the Authorization header. The
	// mock PRS requires both, so only a request made through that client
	// succeeds.
	clientCertificate tls.Certificate
}

func newTestComponent(t *testing.T) (*Component, *prsmock.Service, *providerCall) {
	t.Helper()
	prs := prsmock.NewService(t)
	prs.RegisterRecipient(testNVIURA, testScope)
	component, call := newComponentFor(t, prs, prs.GetURL())
	return component, prs, call
}

// newComponentFor builds a component for baseURL whose authn provider is
// stubbed with a client that carries a fresh client certificate and
// testAccessToken, and that records what it was asked for.
func newComponentFor(t *testing.T, prs *prsmock.Service, baseURL string) (*Component, *providerCall) {
	t.Helper()
	clientCertificate, err := prsmock.NewClientCertificate()
	require.NoError(t, err)
	call := &providerCall{
		clientCertificate: clientCertificate,
		transport:         &closeTracking{base: &http.Transport{TLSClientConfig: prs.TLSClientConfig(clientCertificate)}},
	}
	component := New(Config{PRSBaseURL: baseURL}, func(ctx context.Context, scope []string, ura string, audience string) (*http.Client, error) {
		call.scope, call.ura, call.audience = scope, ura, audience
		call.ctxMarker = ctx.Value(providerContextKey{})
		return authenticatedClient(ctx, call.transport), nil
	})
	return component, call
}

// authenticatedClient is shaped like what authn.HTTPClient returns
// (component/authn/oauth2.go:66-80): an oauth2 client that puts the access
// token in the Authorization header, on a transport that presents the client
// certificate.
func authenticatedClient(ctx context.Context, transport http.RoundTripper) *http.Client {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Transport: transport})
	return oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: testAccessToken, TokenType: "Bearer"}))
}

// closeTracking is a transport that records whether the body of the last
// response it produced was closed.
type closeTracking struct {
	base   http.RoundTripper
	closed atomic.Bool
}

func (c *closeTracking) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := c.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	c.closed.Store(false)
	response.Body = trackedBody{ReadCloser: response.Body, closed: &c.closed}
	return response, nil
}

type trackedBody struct {
	io.ReadCloser
	closed *atomic.Bool
}

func (b trackedBody) Close() error {
	b.closed.Store(true)
	return b.ReadCloser.Close()
}

func bsnIdentifier() fhir.Identifier {
	return identifierFor(testBSN)
}

func identifierFor(bsn string) fhir.Identifier {
	return fhir.Identifier{System: to.Ptr(coding.BSNNamingSystem), Value: to.Ptr(bsn)}
}

func tokenize(t *testing.T, component *Component, scope string) *fhir.Identifier {
	t.Helper()
	return tokenizeWith(t, t.Context(), component, bsnIdentifier(), testLocalURA, testNVIURA, scope)
}

func tokenizeWith(t *testing.T, ctx context.Context, component *Component, identifier fhir.Identifier, localURA string, recipientURA string, scope string) *fhir.Identifier {
	t.Helper()
	result, err := component.IdentifierToToken(ctx, identifier, localURA, recipientURA, scope)
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
		component, prs, call := newTestComponent(t)
		for _, localURA := range []string{testLocalURA, otherLocalURA} {
			ctx := context.WithValue(t.Context(), providerContextKey{}, "context of the call for "+localURA)
			tokenizeWith(t, ctx, component, bsnIdentifier(), localURA, testNVIURA, testScope)

			assert.Equal(t, []string{"prs:read"}, call.scope)
			assert.Equal(t, localURA, call.ura, "the calling organization, not a fixed one")
			assert.Equal(t, prs.GetURL(), call.audience)
			assert.Equal(t, "context of the call for "+localURA, call.ctxMarker, "the caller's context reaches the authn component")
		}
	})

	t.Run("closes the PRS response body", func(t *testing.T) {
		component, _, call := newTestComponent(t)
		tokenize(t, component, testScope)
		assert.True(t, call.transport.closed.Load())

		_, err := component.IdentifierToToken(t.Context(), bsnIdentifier(), testLocalURA, "12345678", testScope)
		require.Error(t, err)
		assert.True(t, call.transport.closed.Load(), "also after an error response")
	})

	t.Run("keeps the configured base path", func(t *testing.T) {
		prs := prsmock.NewService(t)
		prs.RegisterRecipient(testNVIURA, testScope)
		component, _ := newComponentFor(t, prs, prs.GetURL()+"/prs")

		// The mock serves nothing under /prs and answers 404 like FastAPI;
		// what matters is where the request went.
		result, err := component.IdentifierToToken(t.Context(), bsnIdentifier(), testLocalURA, testNVIURA, testScope)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "status=404")
		require.Equal(t, 1, prs.ExchangeCount())
		assert.Equal(t, "/prs/oprf/eval", prs.GetLastExchange().Path)
	})

	t.Run("sends the request over TLS through the client the authn component returned", func(t *testing.T) {
		component, prs, call := newTestComponent(t)
		tokenize(t, component, testScope)

		// The mock only speaks TLS and refuses a handshake without a client
		// certificate and a request without a bearer token, so the call
		// above already fails if the component drops the https scheme or
		// uses another client. What is asserted here is that the credentials
		// the mock saw are the ones the provider's client carries.
		require.True(t, strings.HasPrefix(prs.GetURL(), "https://"), prs.GetURL())
		exchange := prs.GetLastExchange()
		require.NotNil(t, exchange.ClientCertificate, "mTLS")
		assert.Equal(t, call.clientCertificate.Leaf.Raw, exchange.ClientCertificate.Raw)
		assert.Equal(t, "Bearer "+testAccessToken, exchange.Header.Get("Authorization"))
	})

	t.Run("obtains its token from the ministry-style endpoint and presents it", func(t *testing.T) {
		// The real authn client against a mock that only accepts tokens its
		// own token endpoint issued: the certificate signs the token request,
		// the token endpoint binds the token to it, the PRS route requires it.
		prs, err := prsmock.New(prsmock.Options{ListenAddr: "localhost:0"})
		require.NoError(t, err)
		t.Cleanup(func() { _ = prs.Stop() })
		prs.RegisterRecipient(testNVIURA, testScope)
		clientCertificate, err := prsmock.NewClientCertificate()
		require.NoError(t, err)
		dir := t.TempDir()
		key, err := x509.MarshalPKCS8PrivateKey(clientCertificate.PrivateKey)
		require.NoError(t, err)
		writePEM(t, filepath.Join(dir, "cert.pem"), "CERTIFICATE", clientCertificate.Certificate[0])
		writePEM(t, filepath.Join(dir, "key.pem"), "PRIVATE KEY", key)
		writePEM(t, filepath.Join(dir, "ca.pem"), "CERTIFICATE", prs.ServerCertificate().Raw)
		ministry := authn.MinistryAuthConfig{
			Config: tlsutil.Config{
				TLSCertFile: filepath.Join(dir, "cert.pem"),
				TLSKeyFile:  filepath.Join(dir, "key.pem"),
				TLSCAFile:   filepath.Join(dir, "ca.pem"),
			},
			TokenEndpoint: prs.GetURL() + "/oauth/token",
		}
		component := New(Config{PRSBaseURL: prs.GetURL()}, func(ctx context.Context, scope []string, ura string, audience string) (*http.Client, error) {
			return authn.HTTPClient(ctx, scope, ura, audience, ministry)
		})

		tokenize(t, component, testScope)
		bearer := strings.TrimPrefix(prs.GetLastExchange().Header.Get("Authorization"), "Bearer ")
		assert.True(t, prs.TokenIssued(bearer), "the PRS must see a token the token endpoint issued")
	})

	t.Run("fails when the PRS certificate is not trusted", func(t *testing.T) {
		prs := prsmock.NewService(t)
		prs.RegisterRecipient(testNVIURA, testScope)
		clientCertificate, err := prsmock.NewClientCertificate()
		require.NoError(t, err)
		component := New(Config{PRSBaseURL: prs.GetURL()}, func(ctx context.Context, _ []string, _ string, _ string) (*http.Client, error) {
			untrusting := prs.TLSClientConfig(clientCertificate)
			untrusting.RootCAs = x509.NewCertPool()
			return authenticatedClient(ctx, &http.Transport{TLSClientConfig: untrusting}), nil
		})

		result, err := component.IdentifierToToken(t.Context(), bsnIdentifier(), testLocalURA, testNVIURA, testScope)
		var unknownAuthority x509.UnknownAuthorityError
		require.ErrorAs(t, err, &unknownAuthority, "server verification must not be bypassed")
		assert.Nil(t, result)
		assert.Equal(t, 0, prs.ExchangeCount())
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

		// The alphabet only shows in an encoding that contains one of its
		// distinguishing characters, which about three in four elements do,
		// so keep tokenizing until one turns up before pinning it.
		assertStandardAlphabet(t, func() string {
			tokenize(t, component, testScope)
			var again map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(prs.GetLastExchange().Body, &again))
			return unquote(t, again["encryptedPersonalId"])
		})
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

		assertStandardAlphabet(t, func() string {
			return unquote(t, decodeToken(t, *tokenize(t, component, testScope).Value)["blind_factor"])
		})
	})

	t.Run("yields a token the recipient de-blinds to a stable pseudonym", func(t *testing.T) {
		component, prs, _ := newTestComponent(t)
		first := decodeToken(t, *tokenize(t, component, testScope).Value)
		second := decodeToken(t, *tokenize(t, component, testScope).Value)
		assert.NotEqual(t, first["blind_factor"], second["blind_factor"], "a fresh blind per call")
		assert.NotEqual(t, first["evaluated_output"], second["evaluated_output"])

		pseudonym := finalizeToken(t, prs, first, testScope)
		assert.Equal(t, pseudonym, finalizeToken(t, prs, second, testScope))

		// The stable value is k * HashToGroup(HKDF output): the unblinded
		// element of RFC 9497's OPRF(ristretto255, SHA-512) suite before its
		// Finalize step, which the recipient does not apply.
		derived, err := deriveKey(prsIdentifier{LandCode: "NL", Type: "BSN", Value: testBSN}, testNVIURA, testScope)
		require.NoError(t, err)
		expected, err := prs.Pseudonym(derived)
		require.NoError(t, err)
		assert.Equal(t, base64.URLEncoding.EncodeToString(expected), pseudonym)
	})

	t.Run("yields different pseudonyms for different BSNs", func(t *testing.T) {
		component, prs, _ := newTestComponent(t)
		first := finalizeToken(t, prs, decodeToken(t, *tokenize(t, component, testScope).Value), testScope)
		other := finalizeToken(t, prs, decodeToken(t, *tokenizeWith(t, t.Context(), component, identifierFor(otherBSN), testLocalURA, testNVIURA, testScope).Value), testScope)
		assert.NotEqual(t, first, other)
	})

	t.Run("binds the pseudonym to the recipient", func(t *testing.T) {
		component, prs, _ := newTestComponent(t)
		prs.RegisterRecipient(otherRecipientURA, testScope)

		nvi := finalizeToken(t, prs, decodeToken(t, *tokenize(t, component, testScope).Value), testScope)
		token := decodeToken(t, *tokenizeWith(t, t.Context(), component, bsnIdentifier(), testLocalURA, otherRecipientURA, testScope).Value)
		other := finalizeTokenFor(t, prs, token, otherRecipientURA, testScope)
		assert.NotEqual(t, nvi, other)

		// And it is the OPRF of the input derived for that recipient.
		derived, err := deriveKey(prsIdentifier{LandCode: "NL", Type: "BSN", Value: testBSN}, otherRecipientURA, testScope)
		require.NoError(t, err)
		expected, err := prs.Pseudonym(derived)
		require.NoError(t, err)
		assert.Equal(t, base64.URLEncoding.EncodeToString(expected), other)
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

	t.Run("fails when the PRS answers with a server error", func(t *testing.T) {
		for _, tc := range []struct {
			status int
			body   string
		}{
			// Starlette's error middleware on an unhandled exception.
			{http.StatusInternalServerError, "Internal Server Error"},
			// The ingress in front of the service while it is down.
			{http.StatusServiceUnavailable, "503 Service Temporarily Unavailable"},
		} {
			t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
				component, prs, _ := newTestComponent(t)
				prs.Fail(tc.status, tc.body)

				result, err := component.IdentifierToToken(t.Context(), bsnIdentifier(), testLocalURA, testNVIURA, testScope)
				require.Error(t, err)
				assert.Nil(t, result)
				assert.Contains(t, err.Error(), fmt.Sprintf("PRS response: non-OK status code (status=%d", tc.status))
				assert.Contains(t, err.Error(), tc.body)
			})
		}
	})

	t.Run("stops when the context is cancelled", func(t *testing.T) {
		component, prs, _ := newTestComponent(t)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		result, err := component.IdentifierToToken(ctx, bsnIdentifier(), testLocalURA, testNVIURA, testScope)
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result)
		assert.Equal(t, 0, prs.ExchangeCount())
	})
}

func writePEM(t *testing.T, path string, blockType string, der []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600))
}

// assertStandardAlphabet calls next until it yields a base64 value that
// contains a character the standard and URL-safe alphabets encode
// differently, and requires that character to be the standard one. A random
// 32-byte value has about a three-in-four chance per attempt.
func assertStandardAlphabet(t *testing.T, next func() string) {
	t.Helper()
	for attempt := 0; ; attempt++ {
		require.Less(t, attempt, 100, "no value with an alphabet-specific character")
		value := next()
		if !strings.ContainsAny(value, "+/-_") {
			continue
		}
		_, err := base64.StdEncoding.Strict().DecodeString(value)
		require.NoError(t, err, "standard base64 alphabet, not base64url: %s", value)
		return
	}
}

// finalizeToken plays the NVI as recipient; see finalizeTokenFor.
func finalizeToken(t *testing.T, prs *prsmock.Service, token map[string]json.RawMessage, scope string) string {
	t.Helper()
	return finalizeTokenFor(t, prs, token, testNVIURA, scope)
}

// finalizeTokenFor plays the recipient: decrypt evaluated_output, check the
// JWE was issued for this recipient and scope, and de-blind with blind_factor.
func finalizeTokenFor(t *testing.T, prs *prsmock.Service, token map[string]json.RawMessage, recipientURA string, scope string) string {
	t.Helper()
	claims := decryptClaims(t, prs, unquote(t, token["evaluated_output"]))
	assert.Equal(t, `"ura:`+recipientURA+`"`, string(claims["aud"]))
	assert.Equal(t, fmt.Sprintf("%q", scope), string(claims["scope"]))
	subject := unquote(t, claims["subject"])
	require.True(t, strings.HasPrefix(subject, "pseudonym:eval:"), subject)
	pseudonym, err := prsmock.Finalize(unquote(t, token["blind_factor"]), strings.TrimPrefix(subject, "pseudonym:eval:"))
	require.NoError(t, err)
	return pseudonym
}
