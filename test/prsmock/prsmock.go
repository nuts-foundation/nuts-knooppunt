// Package prsmock is an in-process stand-in for the national pseudonymization
// service (PRS, "Pseudoniemendienst") as deployed on the Proeftuin acceptance
// environment (https://pseudoniemendienst.proeftuin.gf.irealisatie.nl).
//
// It is modelled on minvws/gfmodules-pseudoniemendienst at tag v0.0.18
// (commit 990cf81, 2026-02-24), the version data/services.yaml of
// minvws/gfmodules-technische-documentatie (commit 5a0027a) lists for that
// environment. File references in the comments below point into that tag.
//
// Only POST /oprf/eval is served. The mock validates a request in the same
// order and with the same status codes and error bodies as app/routers/oprf.py
// and app/services/oprf/oprf_service.py, evaluates the OPRF for real (RFC 9497
// OPRF(ristretto255, SHA-512), the suite pyoprf/liboprf implements for the
// service) and answers with a JWE encrypted to a recipient key pair it holds
// itself, so a test can also play the recipient and de-blind the result.
//
// It deliberately does not follow the service's main branch (v0.0.34 at the
// time of writing), which moved to "oin:" recipient identifiers and a prs:oprf
// scope, nor the draft implementation guide's /evaluate dialect.
// docs/prs-contract.md has the three side by side.
package prsmock

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudflare/circl/group"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwe"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/stretchr/testify/require"
)

const (
	evalPath = "/oprf/eval"
	// subjectPrefix precedes the evaluation in the JWE subject claim (oprf_service.py:51).
	subjectPrefix = "pseudonym:eval:"
	// jweLifetime is how long the service makes its JWE valid (jwe_token.py:20).
	jweLifetime = 300 * time.Second
	// ThumbprintHash is the hash behind the JWE kid header: jwcrypto's
	// JWK.thumbprint() default, RFC 7638 with SHA-256 (jwe_token.py:25).
	ThumbprintHash = crypto.SHA256
	// elementSize is the size of a ristretto255 element and of a scalar, the
	// only sizes pyoprf accepts (liboprf python/pyoprf/__init__.py).
	elementSize = 32
)

// hashToGroupDST is the domain separation tag liboprf uses to hash an input to
// a ristretto255 element (src/oprf.c:253 at stef/liboprf f925b87), which is
// RFC 9497 section 4 for OPRF(ristretto255, SHA-512) in base mode. It is
// written out here instead of borrowed from circl's oprf package, so the mock
// does not inherit the client's assumptions.
var hashToGroupDST = []byte("HashToGroup-OPRFV1-\x00-ristretto255-SHA512")

var ristretto = group.Ristretto255

// The recipient key pair is generated once per process: RSA key generation is
// the slow part of starting a mock, and tests gain nothing from a fresh key
// per instance.
var (
	recipientKeyOnce sync.Once
	recipientKey     *rsa.PrivateKey
	recipientKeyErr  error
)

func sharedRecipientKey() (*rsa.PrivateKey, error) {
	recipientKeyOnce.Do(func() {
		recipientKey, recipientKeyErr = rsa.GenerateKey(rand.Reader, 2048)
	})
	return recipientKey, recipientKeyErr
}

// Exchange is one captured call: the request as it arrived on the wire and the
// answer the mock gave.
type Exchange struct {
	Method   string
	Path     string
	Header   http.Header
	Body     []byte
	Status   int
	Response []byte
}

// Service is a mock PRS server. Recipients must be registered before a blind
// for them can be evaluated, as organizations and their public keys must be on
// the real service (POST /orgs and POST /register/certificate at v0.0.18).
type Service struct {
	server *http.Server
	url    string
	// oprfKey is the service's OPRF secret: a ristretto255 scalar, as
	// pyoprf.keygen() produces (oprf_service.py:33-38).
	oprfKey group.Scalar
	// recipientKey stands in for the key pair of every registered recipient.
	recipientKey   *rsa.PrivateKey
	recipientKeyID string

	mu         sync.Mutex
	exchanges  []Exchange
	recipients map[string]map[string]struct{} // URA -> scopes with a registered public key
}

// New creates and starts a mock PRS on listenAddr, which is anything
// net.Listen accepts ("localhost:0" for an ephemeral test port). Callers own
// the returned service and must call Stop when done; NewService is the
// testing.T-bound variant that does so for you.
func New(listenAddr string) (*Service, error) {
	key, err := sharedRecipientKey()
	if err != nil {
		return nil, fmt.Errorf("generating recipient key: %w", err)
	}
	keyID, err := thumbprint(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("computing recipient key thumbprint: %w", err)
	}
	prs := &Service{
		oprfKey:        ristretto.RandomNonZeroScalar(rand.Reader),
		recipientKey:   key,
		recipientKeyID: keyID,
		recipients:     map[string]map[string]struct{}{},
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}
	prs.url = "http://" + listener.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("POST "+evalPath, prs.handleEval)
	prs.server = &http.Server{Handler: mux}

	go func() {
		if err := prs.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("mock PRS server error", "error", err)
		}
	}()
	slog.Info("Mock PRS server started", "url", prs.url)

	return prs, nil
}

// NewService creates and starts a mock PRS on an ephemeral port, registering
// cleanup with t.
func NewService(t *testing.T) *Service {
	t.Helper()

	prs, err := New("localhost:0")
	require.NoError(t, err, "failed to start mock PRS server")

	t.Cleanup(func() {
		if err := prs.Stop(); err != nil {
			t.Logf("error stopping mock PRS server: %v", err)
		}
	})

	return prs
}

// GetURL returns the base URL of the mock PRS server.
func (s *Service) GetURL() string {
	return s.url
}

// Stop stops the mock PRS server.
func (s *Service) Stop() error {
	return s.server.Close()
}

// RegisterRecipient makes the organization with the given URA known and
// registers a public key for it under scope, the two onboarding steps the
// real service requires before it evaluates blinds for that recipient.
func (s *Service) RegisterRecipient(ura string, scope string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recipients[ura] == nil {
		s.recipients[ura] = map[string]struct{}{}
	}
	s.recipients[ura][scope] = struct{}{}
}

// GetExchanges returns every captured call, oldest first.
func (s *Service) GetExchanges() []Exchange {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Exchange(nil), s.exchanges...)
}

// GetLastExchange returns the most recent captured call, or nil.
func (s *Service) GetLastExchange() *Exchange {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.exchanges) == 0 {
		return nil
	}
	last := s.exchanges[len(s.exchanges)-1]
	return &last
}

// ExchangeCount returns the number of captured calls.
func (s *Service) ExchangeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.exchanges)
}

// RecipientPublicKey returns the public key the mock encrypts every JWE to.
func (s *Service) RecipientPublicKey() *rsa.PublicKey {
	return &s.recipientKey.PublicKey
}

// DecryptJWE opens a JWE the mock issued with the recipient's private key and
// returns the claims JSON, as the service's own test receiver does
// (app/routers/test_oprf.py:82-95).
func (s *Service) DecryptJWE(token string) ([]byte, error) {
	return jwe.Decrypt([]byte(token), jwe.WithKey(jwa.RSA_OAEP_256, s.recipientKey))
}

// Pseudonym returns the stable value a recipient ends up with for input after
// de-blinding the service's evaluation, computed here directly as
// k * HashToGroup(input) without any blinding. HashToGroup follows liboprf's
// voprf_hash_to_group (src/oprf.c:252-272): expand_message_xmd with SHA-512
// under hashToGroupDST, mapped to the group as in RFC 9380 appendix B.
func (s *Service) Pseudonym(input []byte) ([]byte, error) {
	point := ristretto.HashToElement(input, hashToGroupDST)
	return ristretto.NewElement().Mul(point, s.oprfKey).MarshalBinary()
}

// Finalize de-blinds an evaluation the way both the service's own test receiver
// (OprfService.finalize, oprf_service.py:78-86) and the NVI crypto service
// (minvws/gfmodules-nationale-verwijsindex-crypto-service-api,
// app/services/pseudonym_service.py:41-43) do: both values are read with
// Python's urlsafe_b64decode, N = (1/r) * Z is computed as liboprf's
// oprf_Unblind does (src/oprf.c:362-392) and N is returned as padded base64url.
func Finalize(blindFactor string, evaluated string) (string, error) {
	r, err := pythonURLSafeB64Decode(blindFactor)
	if err != nil {
		return "", fmt.Errorf("decoding blind factor: %w", err)
	}
	z, err := pythonURLSafeB64Decode(evaluated)
	if err != nil {
		return "", fmt.Errorf("decoding evaluation: %w", err)
	}
	if len(r) != elementSize || len(z) != elementSize {
		return "", errors.New("param has incorrect length")
	}
	blind := ristretto.NewScalar()
	if err := blind.UnmarshalBinary(r); err != nil {
		return "", fmt.Errorf("blind factor is not a scalar: %w", err)
	}
	element := ristretto.NewElement()
	if err := element.UnmarshalBinary(z); err != nil {
		return "", fmt.Errorf("evaluation is not a group element: %w", err)
	}
	unblinded, err := ristretto.NewElement().Mul(element, ristretto.NewScalar().Inv(blind)).MarshalBinary()
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(unblinded), nil
}

func (s *Service) handleEval(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	status, response := s.evaluate(body)

	s.mu.Lock()
	s.exchanges = append(s.exchanges, Exchange{
		Method:   r.Method,
		Path:     r.URL.Path,
		Header:   r.Header.Clone(),
		Body:     body,
		Status:   status,
		Response: []byte(response),
	})
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, response)
}

// evaluate is post_eval (app/routers/oprf.py:14-44) with the checks in the
// order the service performs them, so a client gets the same status and body
// for the same mistake. Response bodies are written out as literals on
// purpose: they are the contract, not a rendering of some Go struct.
func (s *Service) evaluate(body []byte) (int, string) {
	request, problems := parseBlindRequest(body)
	if len(problems) > 0 {
		return http.StatusUnprocessableEntity, `{"detail":[` + strings.Join(problems, ",") + `]}`
	}

	if !strings.HasPrefix(request.recipientOrganization, "ura:") {
		return http.StatusBadRequest, `{"error":"Invalid recipient organization. Format: ura:<ura_number>"}`
	}
	ura := strings.TrimPrefix(request.recipientOrganization, "ura:")

	s.mu.Lock()
	scopes, organizationKnown := s.recipients[ura]
	_, keyRegistered := scopes[request.recipientScope]
	s.mu.Unlock()
	if !organizationKnown {
		return http.StatusNotFound, `{"error":"No organization found for this ura"}`
	}
	if !keyRegistered {
		return http.StatusNotFound, `{"error":"No public key found for this organization and/or scope"}`
	}

	evaluated, err := s.evaluateBlind(request.encryptedPersonalID)
	if err != nil {
		return http.StatusBadRequest, `{"error":"Unable to evaluate blind"}`
	}
	token, err := s.buildJWE(request.recipientOrganization, request.recipientScope, evaluated)
	if err != nil {
		return http.StatusInternalServerError, `{"error":` + jsonString(err.Error()) + `}`
	}
	return http.StatusOK, `{"jwe":` + jsonString(token) + `}`
}

type blindRequest struct {
	encryptedPersonalID   string
	recipientOrganization string
	recipientScope        string
}

// parseBlindRequest validates the body like pydantic validates BlindRequest
// (oprf_service.py:12-26): three required strings of at least two characters,
// of which encryptedPersonalId must decode as base64url once padded. Unknown
// members are ignored, as pydantic ignores them by default. Problems are
// collected and reported together in FastAPI's 422 detail shape.
func parseBlindRequest(body []byte) (blindRequest, []string) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(body, &members); err != nil {
		return blindRequest{}, []string{`{"type":"json_invalid","loc":["body"],"msg":"JSON decode error"}`}
	}

	var request blindRequest
	var problems []string
	for _, member := range []struct {
		name  string
		value *string
	}{
		{"encryptedPersonalId", &request.encryptedPersonalID},
		{"recipientOrganization", &request.recipientOrganization},
		{"recipientScope", &request.recipientScope},
	} {
		raw, present := members[member.name]
		if !present {
			problems = append(problems, problem("missing", member.name, "Field required"))
			continue
		}
		if err := json.Unmarshal(raw, member.value); err != nil || string(raw) == "null" {
			problems = append(problems, problem("string_type", member.name, "Input should be a valid string"))
			continue
		}
		if len(*member.value) < 2 {
			problems = append(problems, problem("string_too_short", member.name, "String should have at least 2 characters"))
		}
	}

	if request.encryptedPersonalID != "" {
		// validate_base64 pads before decoding (oprf_service.py:17-26), so an
		// unpadded value passes here and only fails at evaluation.
		if _, err := pythonURLSafeB64Decode(padded(request.encryptedPersonalID)); err != nil {
			problems = append(problems, problem("value_error", "encryptedPersonalId", "Value error, must be base64url: "+err.Error()))
		}
	}
	return request, problems
}

func problem(kind string, field string, message string) string {
	return fmt.Sprintf(`{"type":%s,"loc":["body",%s],"msg":%s}`, jsonString(kind), jsonString(field), jsonString(message))
}

// evaluateBlind is OprfService.eval_blind (oprf_service.py:40-49): decode the
// blinded element and multiply it by the secret key, which is all liboprf's
// oprf_Evaluate does (src/oprf.c:344-348). eval_blind decodes without the
// padding the validator added, so a value that is not padded fails here.
func (s *Service) evaluateBlind(encoded string) ([]byte, error) {
	blinded, err := pythonURLSafeB64Decode(encoded)
	if err != nil {
		return nil, err
	}
	if len(blinded) != elementSize {
		return nil, fmt.Errorf("blinded param has incorrect length: %d", len(blinded))
	}
	element := ristretto.NewElement()
	if err := element.UnmarshalBinary(blinded); err != nil {
		return nil, err
	}
	evaluated := ristretto.NewElement().Mul(element, s.oprfKey)
	if evaluated.IsIdentity() {
		// libsodium's crypto_scalarmult_ristretto255 reports the identity as a failure.
		return nil, errors.New("evaluation is the identity element")
	}
	return evaluated.MarshalBinary()
}

// buildJWE is BlindJwe.build (jwe_token.py:6-36): the evaluation, audience and
// scope in a JSON payload, encrypted for the recipient with RSA-OAEP-256 and
// A256GCM under a protected header carrying the recipient key's thumbprint.
func (s *Service) buildJWE(audience string, scope string, evaluated []byte) (string, error) {
	now := time.Now()
	// Claims as jwe_token.py:14-22 writes them, json.dumps spacing included.
	claims := fmt.Sprintf(`{"subject": %s, "aud": %s, "scope": %s, "version": "1.1", "iat": %d, "exp": %d}`,
		jsonString(subjectPrefix+base64.URLEncoding.EncodeToString(evaluated)),
		jsonString(audience), jsonString(scope), now.Unix(), now.Add(jweLifetime).Unix())

	headers := jwe.NewHeaders()
	if err := headers.Set(jwe.KeyIDKey, s.recipientKeyID); err != nil {
		return "", err
	}
	if err := headers.Set(jwe.ContentTypeKey, "application/json"); err != nil {
		return "", err
	}
	token, err := jwe.Encrypt([]byte(claims),
		jwe.WithKey(jwa.RSA_OAEP_256, &s.recipientKey.PublicKey),
		jwe.WithContentEncryption(jwa.A256GCM),
		jwe.WithProtectedHeaders(headers))
	if err != nil {
		return "", err
	}
	return string(token), nil
}

func thumbprint(key *rsa.PublicKey) (string, error) {
	jwkKey, err := jwk.FromRaw(key)
	if err != nil {
		return "", err
	}
	digest, err := jwkKey.Thumbprint(ThumbprintHash)
	if err != nil {
		return "", err
	}
	// jwcrypto renders thumbprints as unpadded base64url.
	return base64.RawURLEncoding.EncodeToString(digest), nil
}

// pythonURLSafeB64Decode behaves like Python's base64.urlsafe_b64decode, which
// the service uses for every base64 value it reads: "-" and "_" are mapped to
// "+" and "/" and the result is decoded as standard base64, so the standard
// alphabet is accepted just the same. Padding must be correct. (Python also
// discards characters outside the alphabet where Go rejects them; that only
// matters for garbage input.)
func pythonURLSafeB64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(strings.NewReplacer("-", "+", "_", "/").Replace(s))
}

func padded(s string) string {
	return s + strings.Repeat("=", (4-len(s)%4)%4)
}

// jsonString renders s as a JSON string literal.
func jsonString(s string) string {
	encoded, _ := json.Marshal(s) // marshalling a string cannot fail
	return string(encoded)
}
