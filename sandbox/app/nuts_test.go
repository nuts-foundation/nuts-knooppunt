package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/nuts-foundation/go-did/vc"
	"github.com/nuts-foundation/nuts-node/vcr/pe"
	"github.com/stretchr/testify/require"
)

func TestOrganizationContextCredentialShape(t *testing.T) {
	// Z9 must differ from nutsConfigFromEnv's default (Z3): otherwise a
	// hardcoded facilityType in the credential would pass this test too,
	// and nothing would prove the configured value is actually forwarded.
	cred := organizationContextCredential(nutsConfig{FacilityType: "Z9"})

	require.Equal(t, []string{"https://www.w3.org/2018/credentials/v1"}, cred["@context"])
	require.Equal(t, []string{"VerifiableCredential", "SandboxOrganizationContextCredential"}, cred["type"])
	require.Equal(t, map[string]any{"facilityType": "Z9"}, cred["credentialSubject"])
}

// The node rejects a request credential that carries either field and fills
// both in itself, so setting them turns a working request into a 400 whose
// message points at the credential rather than at us.
func TestOrganizationContextCredentialOmitsIssuerAndSubjectID(t *testing.T) {
	cred := organizationContextCredential(nutsConfig{FacilityType: "Z3"})

	require.NotContains(t, cred, "issuer", "the node rejects a caller-supplied issuer")
	subject, ok := cred["credentialSubject"].(map[string]any)
	require.True(t, ok)
	require.NotContains(t, subject, "id", "the node rejects a caller-supplied credentialSubject.id")
}

func TestNutsConfigDefaultsAreDevelopmentLocal(t *testing.T) {
	// Isolate from the ambient environment: without this, a developer who
	// happens to have e.g. SANDBOX_FACILITY_TYPE exported gets a spurious
	// failure here, since envOr would then read the real value instead of
	// falling back to the default this test asserts.
	for _, key := range []string{
		"KNOOPPUNT_INTERNAL_URL",
		"SANDBOX_NUTS_SUBJECT",
		"SANDBOX_BGZ_SCOPE",
		"SANDBOX_AUTH_SERVER",
		"SANDBOX_FACILITY_TYPE",
	} {
		t.Setenv(key, "")
	}

	cfg := nutsConfigFromEnv()

	require.Equal(t, "http://localhost:8081", cfg.InternalBaseURL)
	require.Equal(t, "plataan", cfg.Subject)
	require.Equal(t, "bgz", cfg.Scope)
	// The audience check compares this string to the node's advertised issuer
	// byte for byte, so a Docker hostname here fails even though it resolves.
	require.Equal(t, "http://localhost:8080/nuts/oauth2/plataan", cfg.AuthorizationServer)
	require.Equal(t, "Z3", cfg.FacilityType)
}

// The defaults test above never sets an environment variable, so it only
// exercises envOr's fallback branch. These five names are the contract with
// the compose file a later task writes: a renamed key, or a field reading
// another key while keeping its own default, would leave the default value
// in place and go uncaught there. Distinct sentinel values per key catch
// both.
func TestNutsConfigFromEnvReadsEachVariable(t *testing.T) {
	t.Setenv("KNOOPPUNT_INTERNAL_URL", "http://internal-base-url.example")
	t.Setenv("SANDBOX_NUTS_SUBJECT", "sentinel-subject")
	t.Setenv("SANDBOX_BGZ_SCOPE", "sentinel-scope")
	t.Setenv("SANDBOX_AUTH_SERVER", "http://auth-server.example")
	t.Setenv("SANDBOX_FACILITY_TYPE", "sentinel-facility-type")

	cfg := nutsConfigFromEnv()

	require.Equal(t, "http://internal-base-url.example", cfg.InternalBaseURL)
	require.Equal(t, "sentinel-subject", cfg.Subject)
	require.Equal(t, "sentinel-scope", cfg.Scope)
	require.Equal(t, "http://auth-server.example", cfg.AuthorizationServer)
	require.Equal(t, "sentinel-facility-type", cfg.FacilityType)
}

// sandbox/policy_test.go checks the id_sandbox_organization_context
// descriptor against its own hand-written credential fixture, not this
// function's output. Editing the definition and that fixture together — the
// obvious way to change what the descriptor selects — leaves both packages
// green while the credential this code actually produces no longer matches
// it. This test runs the real matcher (vcr/pe) over the real output of
// organizationContextCredential instead of a fixture.
func TestOrganizationContextCredentialMatchesBGZPolicy(t *testing.T) {
	policyRaw, err := os.ReadFile("../policy/bgz.json.template")
	require.NoError(t, err)

	var mapping map[string]pe.WalletOwnerMapping
	require.NoError(t, json.Unmarshal(policyRaw, &mapping))
	definition := mapping["bgz"]["organization"]

	var descriptor *pe.InputDescriptor
	for _, d := range definition.InputDescriptors {
		if d.Id == "id_sandbox_organization_context" {
			descriptor = d
		}
	}
	require.NotNil(t, descriptor, "id_sandbox_organization_context must still be defined in bgz.json")
	definition.InputDescriptors = []*pe.InputDescriptor{descriptor}

	const configuredFacilityType = "outpatient-clinic"
	cred := organizationContextCredential(nutsConfig{FacilityType: configuredFacilityType})
	credRaw, err := json.Marshal(cred)
	require.NoError(t, err)
	parsed, err := vc.ParseVerifiableCredential(string(credRaw))
	require.NoError(t, err)

	matched, descriptors, err := definition.Match([]vc.VerifiableCredential{*parsed})
	require.NoError(t, err, "the credential organizationContextCredential produces must satisfy the descriptor bgz.json selects it with")
	require.Len(t, matched, 1)

	credentialMap := map[string]vc.VerifiableCredential{descriptors[0].Id: matched[0]}
	fields, err := definition.ResolveConstraintsFields(credentialMap)
	require.NoError(t, err)
	require.Equal(t, configuredFacilityType, fields["organization_facility_type"])
}

// fakeNode stands in for the knooppunt-proxied Nuts internal mux. It asserts
// the request shape the real node requires, so a wrong path, a missing
// token_type or a malformed credentials array fails here rather than in a
// container log.
func fakeNode(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /nuts/internal/auth/v2/plataan/request-service-access-token",
		func(w http.ResponseWriter, r *http.Request) {
			// The real node answers 415 on anything else, and httptest does
			// not: json.Decoder below happily decodes a body sent as
			// text/plain, so without this the header is unverified. The
			// introspect handler gets this for free, because ParseForm only
			// populates the form when the type is form-encoded.
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			var body struct {
				AuthorizationServer string           `json:"authorization_server"`
				Scope               string           `json:"scope"`
				TokenType           string           `json:"token_type"`
				IdToken             string           `json:"id_token"`
				Credentials         []map[string]any `json:"credentials"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "http://localhost:8080/nuts/oauth2/plataan", body.AuthorizationServer)
			require.Equal(t, "bgz", body.Scope)
			require.Equal(t, "Bearer", body.TokenType, "omitting this yields DPoP")
			require.Equal(t, fixtureAttestation, body.IdToken)
			require.Len(t, body.Credentials, 1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "the-token", "token_type": "Bearer"})
		})
	mux.HandleFunc("POST /nuts/internal/auth/v2/accesstoken/introspect",
		func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			if r.PostFormValue("token") != "the-token" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"active": false})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"active":                     true,
				"user_id":                    "900001234",
				"user_role":                  "01.022",
				"organization_ura":           "00000010",
				"organization_name":          "Ziekenhuis De Plataan",
				"organization_facility_type": "Z3",
			})
		})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testNutsClient(t *testing.T, base string) *nutsClient {
	t.Helper()
	// Isolate from the ambient environment, for the same reason
	// TestNutsConfigDefaultsAreDevelopmentLocal does: fakeNode pins its route
	// and its assertions to the defaults, so a developer who has any of these
	// exported would otherwise see this helper hand back a client that
	// fakeNode rejects. The base URL is overridden below regardless.
	for _, key := range []string{
		"SANDBOX_NUTS_SUBJECT",
		"SANDBOX_BGZ_SCOPE",
		"SANDBOX_AUTH_SERVER",
		"SANDBOX_FACILITY_TYPE",
	} {
		t.Setenv(key, "")
	}

	cfg := nutsConfigFromEnv()
	cfg.InternalBaseURL = base
	return newNutsClient(cfg)
}

func TestRequestTokenSendsTheAttestationAsIdToken(t *testing.T) {
	node := fakeNode(t)
	session, err := sessionFromAttestation(fixtureAttestation)
	require.NoError(t, err)

	token, err := testNutsClient(t, node.URL).requestToken(context.Background(), session)
	require.NoError(t, err)
	require.Equal(t, "the-token", token)
}

func TestIntrospectReturnsTheMappedClaims(t *testing.T) {
	node := fakeNode(t)

	claims, err := testNutsClient(t, node.URL).introspect(context.Background(), "the-token")
	require.NoError(t, err)
	require.Equal(t, true, claims["active"])
	require.Equal(t, "900001234", claims["user_id"])
	require.Equal(t, "01.022", claims["user_role"])
	require.Equal(t, "00000010", claims["organization_ura"])
	require.Equal(t, "Z3", claims["organization_facility_type"])
}

// A failing step must name itself. During E4 these are the first things that
// go wrong, and the first slice already taught us what a generic "sign-in
// failed" costs in debugging time.
func TestNutsErrorsNameTheirStep(t *testing.T) {
	unreachable := testNutsClient(t, "http://127.0.0.1:1")
	session, err := sessionFromAttestation(fixtureAttestation)
	require.NoError(t, err)

	_, err = unreachable.requestToken(context.Background(), session)
	require.ErrorContains(t, err, "request service access token")

	_, err = unreachable.introspect(context.Background(), "the-token")
	require.ErrorContains(t, err, "introspect access token")
}

// A malformed KNOOPPUNT_INTERNAL_URL reaches http.NewRequestWithContext's
// error branch. That branch was previously untested and the label "unreachable
// in practice" was wrong: it is one bad environment variable away, and a
// version that swallowed the error would leave the request nil and panic
// inside http.Client.Do rather than reporting a misconfiguration.
func TestNutsRejectsAMalformedBaseURL(t *testing.T) {
	cfg := nutsConfigFromEnv()
	cfg.InternalBaseURL = "http://\x7f-control-char"
	client := newNutsClient(cfg)

	_, err := client.requestToken(context.Background(), authSession{Attestation: fixtureAttestation})
	require.ErrorContains(t, err, "request service access token")

	_, err = client.introspect(context.Background(), "the-token")
	require.ErrorContains(t, err, "introspect access token")
}

// recordedRequest is what actually left this process, as opposed to what
// fakeNode is willing to accept.
type recordedRequest struct {
	path string
	body map[string]any
}

// recordingNode answers every path with a canned status and body and records
// what it was sent. fakeNode cannot do either: its route and its assertions
// are pinned to nutsConfigFromEnv's defaults and it always answers 200, which
// is exactly the blind spot the tests below cover.
func recordingNode(t *testing.T, status int, response string) (*httptest.Server, *recordedRequest) {
	t.Helper()
	var recorded recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.path = r.URL.Path
		// Introspection sends a form, so this decode fails there; the tests
		// that use that endpoint read only the path and the response.
		_ = json.NewDecoder(r.Body).Decode(&recorded.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(srv.Close)
	return srv, &recorded
}

// fakeNode pins its route and its assertions to nutsConfigFromEnv's defaults,
// so a requestToken that hardcoded "plataan", "bgz", the default
// authorization server, or a default-valued config for the credential would
// satisfy it without ever reading nutsConfig. Every value here therefore
// differs from the corresponding default.
func TestRequestTokenSendsTheConfiguredValues(t *testing.T) {
	node, recorded := recordingNode(t, http.StatusOK, `{"access_token":"the-token"}`)
	session, err := sessionFromAttestation(fixtureAttestation)
	require.NoError(t, err)
	client := newNutsClient(nutsConfig{
		InternalBaseURL:     node.URL,
		Subject:             "sentinel-subject",
		Scope:               "sentinel-scope",
		AuthorizationServer: "http://sentinel-auth-server.example/oauth2",
		FacilityType:        "sentinel-facility-type",
	})

	_, err = client.requestToken(context.Background(), session)
	require.NoError(t, err)

	require.Equal(t, "/nuts/internal/auth/v2/sentinel-subject/request-service-access-token", recorded.path)
	require.Equal(t, "sentinel-scope", recorded.body["scope"])
	require.Equal(t, "http://sentinel-auth-server.example/oauth2", recorded.body["authorization_server"])

	credentials, ok := recorded.body["credentials"].([]any)
	require.True(t, ok)
	require.Len(t, credentials, 1)
	credential, ok := credentials[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"facilityType": "sentinel-facility-type"}, credential["credentialSubject"],
		"the credential must be built from this client's config, not from a default one")
}

// The node answers 412 when the wallet lacks a credential the authorizer
// requires, which is the failure most likely to greet the first real run. A
// client that ignored the status would decode that error body into an empty
// token struct and report a missing access_token instead of the reason the
// node gave.
func TestRequestTokenRejectsANonOKStatus(t *testing.T) {
	node, _ := recordingNode(t, http.StatusPreconditionFailed,
		`{"error":"invalid_request","error_description":"no matching credentials"}`)
	session, err := sessionFromAttestation(fixtureAttestation)
	require.NoError(t, err)

	_, err = testNutsClient(t, node.URL).requestToken(context.Background(), session)
	require.ErrorContains(t, err, "request service access token")
	require.ErrorContains(t, err, "status 412")
	require.ErrorContains(t, err, "no matching credentials",
		"the node's reason is the only thing naming which credential is missing")
}

// A 200 carrying no access_token would otherwise hand Task 5 an empty string
// to put in an Authorization header, and the failure would surface as an
// opaque 401 from the resource server one step later.
func TestRequestTokenRejectsAnEmptyAccessToken(t *testing.T) {
	node, _ := recordingNode(t, http.StatusOK, `{"token_type":"Bearer"}`)
	session, err := sessionFromAttestation(fixtureAttestation)
	require.NoError(t, err)

	_, err = testNutsClient(t, node.URL).requestToken(context.Background(), session)
	require.ErrorContains(t, err, "request service access token")
	require.ErrorContains(t, err, "no access_token")
}

// A 200 whose body is not JSON — a proxy error page, or a knooppunt route that
// answers HTML — must not be reported as a token response. Naming the step is
// not enough here: an ignored decode error leaves the token empty, which the
// check above already reports, so the message has to say the body was the
// problem rather than the node's answer.
func TestRequestTokenRejectsAMalformedResponseBody(t *testing.T) {
	node, _ := recordingNode(t, http.StatusOK, `<html>not json</html>`)
	session, err := sessionFromAttestation(fixtureAttestation)
	require.NoError(t, err)

	_, err = testNutsClient(t, node.URL).requestToken(context.Background(), session)
	require.ErrorContains(t, err, "request service access token")
	require.ErrorContains(t, err, "decode response")
}

// The node answers 401 only when the caller may not use the introspection
// endpoint at all; an invalid token is a 200 with active=false. Decoding a 401
// body yields claims that are merely absent, which reads downstream as a
// practitioner without a role rather than as a broken call.
func TestIntrospectRejectsANonOKStatus(t *testing.T) {
	node, _ := recordingNode(t, http.StatusUnauthorized, `{"error":"unauthorized"}`)

	_, err := testNutsClient(t, node.URL).introspect(context.Background(), "the-token")
	require.ErrorContains(t, err, "introspect access token")
	require.ErrorContains(t, err, "status 401")
}

// Worse than its requestToken counterpart: an ignored decode error here returns
// no claims and no error, so Task 5 renders an empty claims table and calls the
// flow a success. There is no later check to catch it.
func TestIntrospectRejectsAMalformedResponseBody(t *testing.T) {
	node, _ := recordingNode(t, http.StatusOK, `<html>not json</html>`)

	claims, err := testNutsClient(t, node.URL).introspect(context.Background(), "the-token")
	require.ErrorContains(t, err, "introspect access token")
	require.ErrorContains(t, err, "decode response")
	require.Nil(t, claims)
}
