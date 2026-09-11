// Package prsmock is an in-process stand-in for the national pseudonymization
// service (PRS, "Pseudoniemendienst") as documented for the Proeftuin
// acceptance environment (https://pseudoniemendienst.proeftuin.gf.irealisatie.nl).
//
// It is modelled on minvws/gfmodules-pseudoniemendienst at tag v0.0.18
// (commit 990cf81, 2026-02-24), the version data/services.yaml of
// minvws/gfmodules-technische-documentatie (commit 5a0027a) lists for that
// environment. The running binary has not been observed from this repository:
// its /version.json sits behind client-certificate TLS. File references in
// the comments below point into that tag.
//
// POST /oprf/eval is served over plain HTTP; any other path answers 404
// {"detail":"Not Found"} as FastAPI does. The acceptance environment is
// reached over mTLS (services.yaml:53), but that is terminated in front of
// the service: by the ingress where this mock is deployed, and by nothing at
// all in docker compose and in-process tests. The mock therefore assumes it
// has happened and checks what v0.0.18 itself checks, the bearer token. The
// route follows v0.0.18 in these respects, each covered by prsmock_test.go:
//
//   - a request without a bearer token is refused with 401
//     {"detail":"Missing bearer token"} (app/auth.py:59-61). FastAPI reads and
//     JSON-decodes the body before it runs that dependency and validates the
//     body after it (fastapi 0.129.2, routing.py:367-414), so malformed JSON
//     gets 422 without a token and every other body gets 401. The token must
//     be one /oauth/token issued for this service unless
//     Options.AcceptAnyBearer is set; its claims are not verified the way
//     v0.0.18 verifies them (see "Not modelled" below);
//   - the body is validated as FastAPI validates BlindRequest
//     (app/services/oprf/oprf_service.py:12-26) and refused with 422 in
//     FastAPI's {"detail":[...]} shape: an empty or null body, a body that is
//     not a JSON object, a Content-Type that names a non-JSON media type (no
//     Content-Type counts as JSON), missing members, members that are not
//     strings, strings shorter than two characters, and a blinded element
//     that Python's base64 decoder rejects once padded. For the inputs
//     in TestService_Validation that FastAPI 0.129.2 and pydantic 2.12.5
//     (the versions v0.0.18 locks) reject when running the v0.0.18
//     BlindRequest verbatim, status and body equal FastAPI's once pydantic's
//     "input" and "ctx" echoes and the JSON decode position are removed;
//     inputs they accept go on to the mock's real evaluation. Other inputs
//     are not claimed;
//   - the recipient must be "ura:<URA>" (400), known (404) and have a key
//     registered under the requested scope or under "*" (404), with the bodies
//     of app/routers/oprf.py:22-35 and the wildcard of
//     app/db/repositories/org_key_repository.py:18-30;
//   - base64 values are read as Python's base64.urlsafe_b64decode reads them,
//     characters outside the alphabet included (pythonURLSafeB64Decode), and
//     the blinded element is decoded once more without the validator's padding
//     at evaluation (oprf_service.py:45);
//   - the element is evaluated for real, Z = k * B on ristretto255, the
//     BlindEvaluate step of RFC 9497's OPRF(ristretto255, SHA-512) suite, and
//     400 {"error":"Unable to evaluate blind"} covers everything pyoprf raises
//     (oprf_service.py:40-49);
//   - the answer is {"jwe":"..."} with the JWE BlindJwe.build produces
//     (app/services/oprf/jwe_token.py), encrypted to a key pair the mock holds
//     for every recipient, so a test can also play the recipient and de-blind
//     the result.
//
// Next to the PRS route the same listener serves POST /oauth/token, a stand-in
// for the ministry's token endpoint as its documentation describes it
// (gfmodules-technische-documentatie, integration-oauth.html): the client
// credentials grant with scope and target_audience, authenticated by mTLS,
// which the proxy in front of this mock is assumed to have done. It issues an
// opaque token for the posted target audience, and /oprf/eval requires a
// token issued for this service unless Options.AcceptAnyBearer is set. The
// JWT-bearer client_assertion that component/authn adds to its request is
// not in that documentation and is ignored. "This service" is
// Options.PublicURL, a configured origin, never the request's Host header,
// which the caller controls.
//
// A second listener serves two routes the real PRS does not have, for a
// sandbox NVI that cannot de-blind itself: POST /sandbox/finalize turns a
// Knooppunt identifier value into the recipient's pseudonym, and POST
// /sandbox/tokenize turns a pseudonym back into a fresh identifier value for
// an audience. Neither route authenticates its caller: whoever reaches the
// listener can de-tokenize and create identifier values, so it belongs on a
// closed network only.
//
// Not modelled: transport security of any kind, so neither the TLS the PRS is
// reached over nor the UZI client certificate the acceptance ingress verifies;
// the bearer's signature and claims (app/services/client_oauth.py:87-140
// verifies issuer, audience and the cnf thumbprint binding against the
// ministry's keys); the other routes; and a key pair per recipient. Whether
// our own client presents a client certificate is therefore not observable
// here; component/pseudonymisation/integration_test.go exercises that against
// the real service. Nothing in the mock fails by itself; Fail injects the 5xx
// answers of Starlette's error middleware or of an ingress.
//
// It deliberately does not follow the service's main branch (v0.0.34 at the
// time of writing), which moved to "oin:" recipient identifiers and a prs:oprf
// scope, nor the draft implementation guide's /evaluate dialect.
// docs/prs-contract.md has the three side by side.
package prsmock

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

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

	jsonContentType = "application/json"
	textContentType = "text/plain; charset=utf-8"

	tokenPath    = "/oauth/token"
	finalizePath = "/sandbox/finalize"
	tokenizePath = "/sandbox/tokenize"
	// tokenLifetime is how long an issued access token is accepted.
	tokenLifetime = 300 * time.Second
	// defaultScope is the recipient scope the sandbox helper tokenizes for.
	defaultScope = "nationale-verwijsindex"
)

// hashToGroupDST is the domain separation tag liboprf uses to hash an input to
// a ristretto255 element (src/oprf.c:253 at stef/liboprf v0.9.4), which is
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

// Options configures a mock. NewService fills them in for a test; cmd fills
// them in from the environment.
type Options struct {
	// ListenAddr is where the PRS API listens, in plain HTTP; anything
	// net.Listen accepts ("localhost:0" for an ephemeral port).
	ListenAddr string
	// SandboxListenAddr is where the sandbox helper listens. Empty leaves the
	// helper off.
	SandboxListenAddr string
	// RecipientKey is the key pair every JWE is encrypted to; nil uses one
	// generated once per process.
	RecipientKey *rsa.PrivateKey
	// OPRFKey is the 32-byte ristretto255 scalar the service evaluates with;
	// nil draws a random one, so pseudonyms differ per instance.
	OPRFKey []byte
	// PublicURL is the origin clients address this service by, such as
	// "http://mock-prs:8080" in compose or the https URL of the ingress that
	// terminates TLS; tokens must have been requested for PublicURL for
	// /oprf/eval to accept them. Empty means the
	// listener's own address, which is what a test wants. It is a configured
	// value on purpose: taken from the request, the Host header would let the
	// caller choose what it is checked against.
	PublicURL string
	// AcceptAnyBearer makes /oprf/eval accept any bearer token, as the
	// component tests need; otherwise only tokens /oauth/token issued count.
	AcceptAnyBearer bool
}

// Service is a mock PRS server. Recipients must be registered before a blind
// for them can be evaluated, as organizations and their public keys must be on
// the real service (POST /orgs and POST /register/certificate at v0.0.18).
type Service struct {
	server          *http.Server
	sandboxServer   *http.Server
	url             string
	sandboxURL      string
	publicURL       string
	acceptAnyBearer bool
	// oprfKey is the service's OPRF secret: a ristretto255 scalar, as
	// pyoprf.keygen() produces (oprf_service.py:33-38).
	oprfKey group.Scalar
	// recipientKey stands in for the key pair of every registered recipient.
	recipientKey   *rsa.PrivateKey
	recipientKeyID string

	mu         sync.Mutex
	exchanges  []Exchange
	recipients map[string]map[string]struct{} // URA -> scopes with a registered public key
	failure    *failure
	tokens     map[string]issuedToken
}

// issuedToken is what the token endpoint remembers about a token: when it
// expires and the target audience it was obtained for, which must be this
// service for the PRS route to accept it, as the real verifier checks the
// token's audience (app/services/client_oauth.py:97-107).
type issuedToken struct {
	expiry         time.Time
	targetAudience string
}

// failure is an injected answer that replaces evaluation; see Fail.
type failure struct {
	status int
	body   string
}

// New creates and starts a mock PRS. Callers own the returned service and
// must call Stop when done; NewService is the testing.T-bound variant that
// does so for you.
func New(opts Options) (*Service, error) {
	key := opts.RecipientKey
	if key == nil {
		var err error
		if key, err = sharedRecipientKey(); err != nil {
			return nil, fmt.Errorf("generating recipient key: %w", err)
		}
	}
	keyID, err := thumbprint(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("computing recipient key thumbprint: %w", err)
	}
	oprfKey := ristretto.RandomNonZeroScalar(rand.Reader)
	if opts.OPRFKey != nil {
		oprfKey = ristretto.NewScalar()
		if err := oprfKey.UnmarshalBinary(opts.OPRFKey); err != nil {
			return nil, fmt.Errorf("OPRF key is not a ristretto255 scalar: %w", err)
		}
		// The decoder reduces a value at or above the group order rather
		// than rejecting it; a key that does not survive the round trip
		// would silently be a different key than the one on disk.
		if encoded, err := oprfKey.MarshalBinary(); err != nil || !bytes.Equal(encoded, opts.OPRFKey) {
			return nil, errors.New("OPRF key is not a canonical ristretto255 scalar (32 little-endian bytes below the group order)")
		}
		if oprfKey.IsZero() {
			return nil, errors.New("OPRF key is zero")
		}
	}
	prs := &Service{
		acceptAnyBearer: opts.AcceptAnyBearer,
		oprfKey:         oprfKey,
		recipientKey:    key,
		recipientKeyID:  keyID,
		recipients:      map[string]map[string]struct{}{},
		tokens:          map[string]issuedToken{},
	}

	listener, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", opts.ListenAddr, err)
	}
	prs.url = "http://" + listener.Addr().String()
	prs.publicURL = prs.url
	if opts.PublicURL != "" {
		// Validated as given, before any normalization: "/" is not "unset",
		// and "http://host//" or "http://host?" are not origins either.
		public, err := url.Parse(opts.PublicURL)
		if err != nil || (public.Scheme != "http" && public.Scheme != "https") ||
			public.Host == "" || public.User != nil ||
			(public.Path != "" && public.Path != "/") || public.RawQuery != "" || public.ForceQuery || public.Fragment != "" {
			_ = listener.Close()
			return nil, fmt.Errorf("PublicURL must be an http or https origin without path, query or user information: %q", opts.PublicURL)
		}
		prs.publicURL = public.Scheme + "://" + public.Host
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST "+evalPath, prs.handleEval)
	mux.HandleFunc("POST "+tokenPath, prs.handleToken)
	mux.HandleFunc("/", prs.handleUnknownPath)
	prs.server = &http.Server{Handler: mux}

	go func() {
		if err := prs.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("mock PRS server error", "error", err)
		}
	}()
	slog.Info("Mock PRS server started", "url", prs.url)

	if opts.SandboxListenAddr != "" {
		sandboxListener, err := net.Listen("tcp", opts.SandboxListenAddr)
		if err != nil {
			_ = prs.server.Close()
			return nil, fmt.Errorf("failed to listen on %s: %w", opts.SandboxListenAddr, err)
		}
		prs.sandboxURL = "http://" + sandboxListener.Addr().String()
		sandboxMux := http.NewServeMux()
		sandboxMux.HandleFunc("POST "+finalizePath, prs.handleSandboxFinalize)
		sandboxMux.HandleFunc("POST "+tokenizePath, prs.handleSandboxTokenize)
		prs.sandboxServer = &http.Server{Handler: sandboxMux}
		go func() {
			if err := prs.sandboxServer.Serve(sandboxListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("mock PRS sandbox helper error", "error", err)
			}
		}()
		slog.Info("Mock PRS sandbox helper started", "url", prs.sandboxURL)
	}

	return prs, nil
}

// NewOPRFKey draws a random OPRF key in the form Options.OPRFKey and
// OPRF_KEY_FILE take: a canonical non-zero ristretto255 scalar, 32 bytes.
func NewOPRFKey() ([]byte, error) {
	return ristretto.RandomNonZeroScalar(rand.Reader).MarshalBinary()
}

// NewService creates and starts a mock PRS on ephemeral ports, with the
// sandbox helper on and any bearer token accepted, registering cleanup with t.
func NewService(t *testing.T) *Service {
	t.Helper()

	prs, err := New(Options{ListenAddr: "localhost:0", SandboxListenAddr: "localhost:0", AcceptAnyBearer: true})
	require.NoError(t, err, "failed to start mock PRS server")

	t.Cleanup(func() {
		if err := prs.Stop(); err != nil {
			t.Logf("error stopping mock PRS server: %v", err)
		}
	})

	return prs
}

// GetURL returns the base URL of the mock PRS server, an http URL.
func (s *Service) GetURL() string {
	return s.url
}

// SandboxURL returns the base URL of the sandbox helper, or "" when it is off.
func (s *Service) SandboxURL() string {
	return s.sandboxURL
}

// Stop stops the mock PRS server and its sandbox helper.
func (s *Service) Stop() error {
	err := s.server.Close()
	if s.sandboxServer != nil {
		if sandboxErr := s.sandboxServer.Close(); err == nil {
			err = sandboxErr
		}
	}
	return err
}

// TokenIssued reports whether the token endpoint issued token and it has not
// expired.
func (s *Service) TokenIssued(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	issued, ok := s.tokens[token]
	return ok && time.Now().Before(issued.expiry)
}

// tokenIssuedTo reports whether token is valid and was requested for this
// service: the ministry's token is tied to one target_audience, and the
// verifier checks the token's audience
// (app/services/client_oauth.py:97-107). The scope value is not checked, as
// v0.0.18 does not check it either. The certificate binding the real verifier
// also checks is out of reach here; see the package comment.
func (s *Service) tokenIssuedTo(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	issued, ok := s.tokens[token]
	return ok && time.Now().Before(issued.expiry) &&
		targetsOrigin(issued.targetAudience, s.publicURL)
}

// targetsOrigin reports whether target, the target_audience a token was
// issued for, names this service: a URL whose scheme and authority are
// exactly the configured public origin, with no user information, no path
// beyond "/", no query (not even an empty one) and no fragment. A plain
// prefix comparison would accept "https://<host>@elsewhere", and the
// request's own Host header would let the caller choose the host.
func targetsOrigin(target string, publicURL string) bool {
	parsed, err := url.Parse(target)
	if err != nil {
		return false
	}
	return parsed.User == nil && parsed.Scheme+"://"+parsed.Host == publicURL &&
		(parsed.Path == "" || parsed.Path == "/") && parsed.RawQuery == "" && !parsed.ForceQuery && parsed.Fragment == ""
}

// RegisterRecipient makes the organization with the given URA known and
// registers a public key for it under scope, the two onboarding steps the
// real service requires before it evaluates blinds for that recipient. A key
// registered under "*" serves every scope, as on the real service.
func (s *Service) RegisterRecipient(ura string, scope string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.recipients[ura] == nil {
		s.recipients[ura] = map[string]struct{}{}
	}
	s.recipients[ura][scope] = struct{}{}
}

// Fail makes every following request answer with status and a text/plain body
// instead of being evaluated: 500 "Internal Server Error" as Starlette's error
// middleware answers an unhandled exception (starlette 0.52.1,
// middleware/errors.py:259), or 502 and 503 as an ingress answers when the
// service is down. A status of 0 restores normal operation.
func (s *Service) Fail(status int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if status == 0 {
		s.failure = nil
		return
	}
	s.failure = &failure{status: status, body: body}
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
// oprf_Unblind does (src/oprf.c:362-392 at v0.9.4) and N is returned as
// padded base64url. Like oprf_Unblind it fails on a zero blind, which has no
// inverse, and on an identity result, which libsodium's scalar multiplication
// reports as a failure. This is RFC 9497's Unblind step only; neither
// recipient applies Finalize.
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
	if blind.IsZero() {
		return "", errors.New("blind factor is zero")
	}
	element := ristretto.NewElement()
	if err := element.UnmarshalBinary(z); err != nil {
		return "", fmt.Errorf("evaluation is not a group element: %w", err)
	}
	unblinded := ristretto.NewElement().Mul(element, ristretto.NewScalar().Inv(blind))
	if unblinded.IsIdentity() {
		return "", errors.New("unblinded element is the identity")
	}
	encoded, err := unblinded.MarshalBinary()
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(encoded), nil
}

func (s *Service) handleEval(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	status, contentType, response := s.respond(r, body)
	s.record(r, body, status, response)

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = io.WriteString(w, response)
}

// handleUnknownPath answers every other path as FastAPI does, and records the
// call so a test can see where a request went.
func (s *Service) handleUnknownPath(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	defer r.Body.Close()
	const response = `{"detail":"Not Found"}`
	s.record(r, body, http.StatusNotFound, response)

	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, response)
}

func (s *Service) record(r *http.Request, body []byte, status int, response string) {
	exchange := Exchange{
		Method:   r.Method,
		Path:     r.URL.Path,
		Header:   r.Header.Clone(),
		Body:     body,
		Status:   status,
		Response: []byte(response),
	}
	s.mu.Lock()
	s.exchanges = append(s.exchanges, exchange)
	s.mu.Unlock()
}

// respond handles one request in the order the service checks it, so a client
// gets the same status and body for the same mistake: FastAPI reads and
// JSON-decodes the body, runs the bearer-token dependency, validates the body
// against BlindRequest, and only then enters post_eval. Response bodies are
// written out as literals on purpose: they are the contract, not a rendering
// of some Go struct.
func (s *Service) respond(r *http.Request, body []byte) (status int, contentType string, response string) {
	s.mu.Lock()
	injected := s.failure
	s.mu.Unlock()
	if injected != nil {
		return injected.status, textContentType, injected.body
	}

	parsed, problem := parseBody(r.Header.Get("Content-Type"), body)
	if problem != "" {
		return http.StatusUnprocessableEntity, jsonContentType, `{"detail":[` + problem + `]}`
	}
	token, ok := bearerToken(r)
	if !ok {
		return http.StatusUnauthorized, jsonContentType, `{"detail":"Missing bearer token"}`
	}
	if !s.acceptAnyBearer && !s.tokenIssuedTo(token) {
		// What the verifier answers for a token it cannot verify or whose
		// cnf.x5t#S256 is not the presented certificate: an HTTPException
		// that bypasses get_auth_ctx's OAuthError handling
		// (app/services/client_oauth.py:109-111,144-150).
		return http.StatusUnauthorized, jsonContentType, `{"detail":"Invalid token"}`
	}
	request, problems := validateBlindRequest(parsed)
	if len(problems) > 0 {
		return http.StatusUnprocessableEntity, jsonContentType, `{"detail":[` + strings.Join(problems, ",") + `]}`
	}
	status, response = s.evaluate(request)
	return status, jsonContentType, response
}

// parsedBody is the request body as FastAPI hands it to validation
// (routing.py:367-404 at 0.129.2): absent when empty, the JSON text when there
// is no Content-Type or the media type is JSON, and the raw bytes otherwise.
type parsedBody struct {
	absent bool
	raw    bool
	json   []byte
}

// parseBody returns the body as FastAPI sees it, or the json_invalid problem
// FastAPI raises before anything else when a JSON body does not decode.
func parseBody(contentType string, body []byte) (parsedBody, string) {
	if len(body) == 0 {
		return parsedBody{absent: true}, ""
	}
	if !isJSONMediaType(contentType) {
		return parsedBody{raw: true}, ""
	}
	if !json.Valid(body) {
		return parsedBody{}, `{"type":"json_invalid","loc":["body"],"msg":"JSON decode error"}`
	}
	return parsedBody{json: body}, ""
}

// isJSONMediaType is FastAPI's test (routing.py:369-382 at 0.129.2): no
// Content-Type means JSON; otherwise application/json or application/*+json.
func isJSONMediaType(contentType string) bool {
	if contentType == "" {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json" || (strings.HasPrefix(mediaType, "application/") && strings.HasSuffix(mediaType, "+json"))
}

// bearerToken reads an "Authorization: Bearer <token>" header as FastAPI's
// HTTPBearer does: the scheme is compared case-insensitively, and both scheme
// and token must be present.
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "bearer") || token == "" {
		return "", false
	}
	return token, true
}

// evaluate is post_eval (app/routers/oprf.py:14-44) with the checks in the
// order the service performs them.
func (s *Service) evaluate(request blindRequest) (int, string) {
	if !strings.HasPrefix(request.recipientOrganization, "ura:") {
		return http.StatusBadRequest, `{"error":"Invalid recipient organization. Format: ura:<ura_number>"}`
	}
	ura := strings.TrimPrefix(request.recipientOrganization, "ura:")

	s.mu.Lock()
	scopes, organizationKnown := s.recipients[ura]
	_, keyRegistered := scopes[request.recipientScope]
	if !keyRegistered {
		// A key entry with scope "*" matches everything (org_key_repository.py:18-30).
		_, keyRegistered = scopes["*"]
	}
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

// validateBlindRequest validates the body like pydantic validates BlindRequest
// (oprf_service.py:12-26): an object with three required strings of at least
// two characters, of which encryptedPersonalId must decode as base64url once
// padded. Unknown members are ignored, as pydantic ignores them by default.
// Problems are collected in field order and reported together in FastAPI's
// 422 detail shape.
func validateBlindRequest(parsed parsedBody) (blindRequest, []string) {
	missingBody := []string{`{"type":"missing","loc":["body"],"msg":"Field required"}`}
	notAnObject := []string{`{"type":"model_attributes_type","loc":["body"],"msg":"Input should be a valid dictionary or object to extract fields from"}`}
	if parsed.absent {
		return blindRequest{}, missingBody
	}
	if parsed.raw {
		return blindRequest{}, notAnObject
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(parsed.json, &members); err != nil {
		return blindRequest{}, notAnObject
	}
	if members == nil {
		// JSON null: FastAPI passes None on as an absent body.
		return blindRequest{}, missingBody
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
		if utf8.RuneCountInString(*member.value) < 2 {
			problems = append(problems, problem("string_too_short", member.name, "String should have at least 2 characters"))
			continue
		}
		if member.name == "encryptedPersonalId" {
			// validate_base64 runs only on a value that passed the field's own
			// checks, as pydantic runs an after-validator, and it pads before
			// decoding (oprf_service.py:17-26), so an unpadded value passes
			// here and only fails at evaluation.
			if _, err := pythonURLSafeB64Decode(padded(*member.value)); err != nil {
				problems = append(problems, problem("value_error", member.name, "Value error, must be base64url: "+err.Error()))
			}
		}
	}
	return request, problems
}

func problem(kind string, field string, message string) string {
	return fmt.Sprintf(`{"type":%s,"loc":["body",%s],"msg":%s}`, jsonString(kind), jsonString(field), jsonString(message))
}

// evaluateBlind is OprfService.eval_blind (oprf_service.py:40-49): decode the
// blinded element and multiply it by the secret key, which is all liboprf's
// oprf_Evaluate does (src/oprf.c). eval_blind decodes without the padding the
// validator added, so a value that is not padded fails here.
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

// handleToken is a stand-in for the ministry's token endpoint as its
// documentation describes it: a client_credentials grant carrying scope and
// target_audience, authenticated by mTLS, which is assumed to have happened
// in front of this mock. It issues an opaque token for that target audience.
// The JWT-bearer client_assertion component/authn adds (oauth2.go:101-147) is
// not in that documentation and is ignored. Which service a token is for is
// later compared with Options.PublicURL, a configured value, never the
// request's Host header.
func (s *Service) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		tokenError(w, "invalid_request", "cannot parse form")
		return
	}
	if r.PostForm.Get("grant_type") != "client_credentials" {
		tokenError(w, "unsupported_grant_type", "grant_type must be client_credentials")
		return
	}
	if r.PostForm.Get("scope") == "" {
		tokenError(w, "invalid_request", "scope is required")
		return
	}
	targetAudience := r.PostForm.Get("target_audience")
	if targetAudience == "" {
		tokenError(w, "invalid_request", "target_audience is required")
		return
	}
	// client_assertion and client_assertion_type, when posted, are ignored:
	// the ministry documents mTLS as the client authentication and no
	// assertion, and the certificate stays at the proxy that terminates TLS.

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	s.mu.Lock()
	s.tokens[token] = issuedToken{
		expiry:         time.Now().Add(tokenLifetime),
		targetAudience: targetAudience,
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", jsonContentType)
	_, _ = fmt.Fprintf(w, `{"access_token":%s,"token_type":"Bearer","expires_in":%d}`, jsonString(token), int(tokenLifetime.Seconds()))
}

// tokenError answers as RFC 6749 section 5.2 prescribes.
func tokenError(w http.ResponseWriter, code string, description string) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(http.StatusBadRequest)
	_, _ = fmt.Fprintf(w, `{"error":%s,"error_description":%s}`, jsonString(code), jsonString(description))
}

// identifierValue is the NVI identifier value the Knooppunt produces
// (component/pseudonymisation/component.go, marshalSubjectIdentifier): the
// blind factor as encoding/json writes a []byte, the JWE verbatim, the object
// as unpadded base64url.
type identifierValue struct {
	BlindFactor     []byte `json:"blind_factor"`
	EvaluatedOutput string `json:"evaluated_output"`
}

// handleSandboxFinalize plays the recipient for a sandbox NVI that cannot:
// {"token": <identifier value>} in, {"pseudonym": <hex of the unblinded
// element>} out. Hex, because the sandbox NVI uses the pseudonym as a FHIR id.
func (s *Service) handleSandboxFinalize(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Token == "" {
		sandboxError(w, "body must be {\"token\": <identifier value>}")
		return
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(request.Token, "="))
	if err != nil {
		sandboxError(w, "token is not base64url: "+err.Error())
		return
	}
	var value identifierValue
	if err := json.Unmarshal(decoded, &value); err != nil {
		sandboxError(w, "token is not the identifier JSON: "+err.Error())
		return
	}
	payload, err := s.DecryptJWE(value.EvaluatedOutput)
	if err != nil {
		sandboxError(w, "evaluated_output does not decrypt: "+err.Error())
		return
	}
	var claims struct {
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || !strings.HasPrefix(claims.Subject, subjectPrefix) {
		sandboxError(w, "JWE carries no evaluation")
		return
	}
	pseudonym, err := Finalize(base64.URLEncoding.EncodeToString(value.BlindFactor), strings.TrimPrefix(claims.Subject, subjectPrefix))
	if err != nil {
		sandboxError(w, "de-blinding failed: "+err.Error())
		return
	}
	element, err := base64.URLEncoding.DecodeString(pseudonym)
	if err != nil {
		sandboxError(w, err.Error())
		return
	}
	w.Header().Set("Content-Type", jsonContentType)
	_, _ = fmt.Fprintf(w, `{"pseudonym":%s}`, jsonString(hex.EncodeToString(element)))
}

// handleSandboxTokenize is the reverse for a sandbox NVI that hands tokens
// out on read: {"pseudonym": <hex>, "audience": <URA>, "scope": <optional>}
// in, {"token": <identifier value for that audience>} out. It blinds the
// pseudonym with a fresh scalar and encrypts the result for the audience, so
// the token de-blinds to the same pseudonym.
func (s *Service) handleSandboxTokenize(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Pseudonym string `json:"pseudonym"`
		Audience  string `json:"audience"`
		Scope     string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Pseudonym == "" || request.Audience == "" {
		sandboxError(w, "body must be {\"pseudonym\": <hex>, \"audience\": <URA>}")
		return
	}
	if request.Scope == "" {
		request.Scope = defaultScope
	}
	elementBytes, err := hex.DecodeString(request.Pseudonym)
	if err != nil || len(elementBytes) != elementSize {
		sandboxError(w, "pseudonym must be 32 bytes in hex")
		return
	}
	element := ristretto.NewElement()
	if err := element.UnmarshalBinary(elementBytes); err != nil {
		sandboxError(w, "pseudonym is not a ristretto255 element: "+err.Error())
		return
	}
	blind := ristretto.RandomNonZeroScalar(rand.Reader)
	blinded, err := ristretto.NewElement().Mul(element, blind).MarshalBinary()
	if err != nil {
		sandboxError(w, err.Error())
		return
	}
	token, err := s.buildJWE("ura:"+request.Audience, request.Scope, blinded)
	if err != nil {
		sandboxError(w, err.Error())
		return
	}
	blindBytes, err := blind.MarshalBinary()
	if err != nil {
		sandboxError(w, err.Error())
		return
	}
	value, err := json.Marshal(identifierValue{BlindFactor: blindBytes, EvaluatedOutput: token})
	if err != nil {
		sandboxError(w, err.Error())
		return
	}
	w.Header().Set("Content-Type", jsonContentType)
	_, _ = fmt.Fprintf(w, `{"token":%s}`, jsonString(base64.RawURLEncoding.EncodeToString(value)))
}

func sandboxError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(http.StatusBadRequest)
	_, _ = fmt.Fprintf(w, `{"error":%s}`, jsonString(message))
}

// pythonURLSafeB64Decode decodes s as Python's base64.urlsafe_b64decode does
// (Lib/base64.py and the non-strict mode of binascii.a2b_base64), which the
// service uses for every base64 value it reads: the string must be ASCII; "-"
// and "_" count as "+" and "/"; characters outside the alphabet are discarded,
// as is "=" in the first two positions of a quad; decoding stops at the first
// complete padding sequence and ignores what follows; and a final partial quad
// without complete padding is an error, with binascii's message. Checked
// against CPython 3.9, 3.11 and 3.13 on 60,000 random inputs;
// prsmock_internal_test.go keeps the edge cases. CPython 3.14.6 decodes
// malformed input differently: data after a complete padding sequence is an
// error there, and a "=" in the middle is skipped. The v0.0.18 image builds
// from python:3.14-slim (docker/Dockerfile:3) at an unobserved patch level,
// so for garbage input this model may be older than what acceptance runs;
// well-formed padded values decode the same on every interpreter.
func pythonURLSafeB64Decode(s string) ([]byte, error) {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7f {
			return nil, errors.New("string argument should contain only ASCII characters")
		}
	}
	data := make([]byte, 0, len(s))
	quadPos, pads := 0, 0
scan:
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '=':
			if quadPos >= 2 {
				pads++
				if quadPos+pads >= 4 {
					quadPos = 0
					break scan
				}
			}
			continue
		case '-':
			c = '+'
		case '_':
			c = '/'
		}
		if !inBase64Alphabet(c) {
			continue
		}
		pads = 0
		data = append(data, c)
		quadPos = (quadPos + 1) % 4
	}
	// The messages are binascii's, capitalization included: they end up in
	// the 422 body through the validator's "must be base64url: {e}".
	if quadPos == 1 {
		return nil, fmt.Errorf("Invalid base64-encoded string: number of data characters (%d) cannot be 1 more than a multiple of 4", len(data))
	}
	if quadPos != 0 {
		return nil, errors.New("Incorrect padding")
	}
	return base64.RawStdEncoding.DecodeString(string(data))
}

func inBase64Alphabet(c byte) bool {
	return ('A' <= c && c <= 'Z') || ('a' <= c && c <= 'z') || ('0' <= c && c <= '9') || c == '+' || c == '/'
}

func padded(s string) string {
	return s + strings.Repeat("=", (4-len(s)%4)%4)
}

// jsonString renders s as a JSON string literal.
func jsonString(s string) string {
	encoded, _ := json.Marshal(s) // marshalling a string cannot fail
	return string(encoded)
}
