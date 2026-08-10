package sandbox

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeNode mirrors the parts of the Nuts internal API the bootstrap script
// touches, so the script can be driven without containers, and counts the
// mutating calls so a second run can be proven to make none.
//
// Two behaviours matter as much as the counters. A create without a subject in
// the body generates a name, as the node does (nuts-node
// docs/_static/vdr/v2.yaml: "If not given, a uuid is generated and returned"),
// and the wallet hands credentials back in the format they were stored in,
// which for a did:x509 credential is a JWT compact string
// (docs/_static/common/ssi_types.yaml JWTCompactVerifiableCredential).
type fakeNode struct {
	mu       sync.Mutex
	subjects map[string][]string
	wallets  map[string][]json.RawMessage
	creates  int
	stores   int
	// rejectStores makes the node refuse credentials, as it does for one it
	// cannot verify.
	rejectStores bool
	// failSubjectList, failSubjectCreate and failWallet make a call fail the
	// way the node does: an error status carrying a problem document, which is
	// itself valid JSON.
	failSubjectList   bool
	failSubjectCreate bool
	failWallet        bool
}

func newFakeNode(t *testing.T) (*fakeNode, string) {
	t.Helper()
	n := &fakeNode{
		subjects: map[string][]string{},
		wallets:  map[string][]json.RawMessage{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /nuts/internal/vdr/v2/subject", n.listSubjects)
	mux.HandleFunc("POST /nuts/internal/vdr/v2/subject", n.createSubject)
	mux.HandleFunc("GET /nuts/internal/vcr/v2/holder/{subject}/vc", n.listWallet)
	mux.HandleFunc("POST /nuts/internal/vcr/v2/holder/{subject}/vc", n.storeCredential)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return n, srv.URL
}

func (n *fakeNode) listSubjects(w http.ResponseWriter, _ *http.Request) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.failSubjectList {
		problem(w, "could not list subjects")
		return
	}
	_ = json.NewEncoder(w).Encode(n.subjects)
}

// problem answers the way the node does on an error: RFC 7807, and so a body
// that parses as JSON even though it holds no answer
// (nuts-node docs/_static/common/error_response.yaml).
func problem(w http.ResponseWriter, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title":  "Internal Server Error",
		"status": http.StatusInternalServerError,
		"detail": detail,
	})
}

func (n *fakeNode) createSubject(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.creates++

	if !jsonBody(w, r) {
		return
	}
	if n.failSubjectCreate {
		problem(w, "could not create the subject")
		return
	}
	var request struct {
		Subject string `json:"subject"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &request); err != nil {
		http.Error(w, "malformed body", http.StatusBadRequest)
		return
	}
	if request.Subject == "" {
		request.Subject = fmt.Sprintf("generated-uuid-%d", n.creates)
	}

	did := "did:web:example.com:iam:" + request.Subject
	n.subjects[request.Subject] = []string{did}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"subject":   request.Subject,
		"documents": []map[string]string{{"id": did}},
	})
}

func (n *fakeNode) listWallet(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.failWallet {
		problem(w, "could not read the wallet")
		return
	}
	subject := r.PathValue("subject")
	if _, ok := n.subjects[subject]; !ok {
		http.Error(w, "no such subject", http.StatusNotFound)
		return
	}
	wallet := n.wallets[subject]
	if wallet == nil {
		wallet = []json.RawMessage{}
	}
	_ = json.NewEncoder(w).Encode(wallet)
}

func (n *fakeNode) storeCredential(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stores++

	if !jsonBody(w, r) {
		return
	}
	subject := r.PathValue("subject")
	if _, ok := n.subjects[subject]; !ok {
		http.Error(w, "no such subject", http.StatusNotFound)
		return
	}
	if n.rejectStores {
		http.Error(w, `{"title":"invalid credential"}`, http.StatusBadRequest)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var credential json.RawMessage
	if err := json.Unmarshal(body, &credential); err != nil {
		http.Error(w, "credential is not JSON", http.StatusBadRequest)
		return
	}
	n.wallets[subject] = append(n.wallets[subject], credential)
	w.WriteHeader(http.StatusNoContent)
}

// jsonBody rejects a request the node's binder would reject, so a body sent
// with curl's default form content type does not silently pass.
func jsonBody(w http.ResponseWriter, r *http.Request) bool {
	if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		http.Error(w, "unsupported media type "+contentType, http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

func (n *fakeNode) counts() (creates int, stores int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.creates, n.stores
}

func (n *fakeNode) subjectNames() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	names := make([]string, 0, len(n.subjects))
	for name := range n.subjects {
		names = append(names, name)
	}
	return names
}

// walletRaw returns the credentials as stored, so a test can assert the exact
// JSON the script posted.
func (n *fakeNode) walletRaw(subject string) []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	raw := make([]string, 0, len(n.wallets[subject]))
	for _, credential := range n.wallets[subject] {
		raw = append(raw, string(credential))
	}
	return raw
}

func (n *fakeNode) seedSubject(subject string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.subjects[subject] = []string{"did:web:example.com:iam:" + subject}
}

func (n *fakeNode) seedWallet(subject string, credentials ...string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, credential := range credentials {
		n.wallets[subject] = append(n.wallets[subject], json.RawMessage(credential))
	}
}

// jwtCredential builds a credential in the JWT compact serialization, carrying
// its types in the "vc" claim where go-did reads them
// (go-did vc/vc.go parseJWTCredential).
func jwtCredential(types ...string) string {
	return jwtCredentialWithClaims(nil, types...)
}

func jwtCredentialWithClaims(extra map[string]any, types ...string) string {
	segment := func(claims any) string {
		encoded, err := json.Marshal(claims)
		if err != nil {
			panic(err)
		}
		return base64.RawURLEncoding.EncodeToString(encoded)
	}
	claims := map[string]any{"vc": map[string]any{"type": types}}
	for name, value := range extra {
		claims[name] = value
	}
	return strings.Join([]string{
		segment(map[string]string{"alg": "none"}),
		segment(claims),
		"signature",
	}, ".")
}

// x509CredentialJWT is what the toolkit hands back: an X509Credential as a JWT,
// with the claims that come with it. Those claims are what makes the payload
// not a multiple of four base64 characters, so reading it back at all depends
// on restoring the padding a JWT leaves off.
func x509CredentialJWT(t *testing.T) string {
	t.Helper()
	credential := jwtCredentialWithClaims(map[string]any{
		"iss": "did:x509:0:sha256:0Nfv4ZSDLbBqLXvVBPcjITMWFYm4dsK5wIeCDcbmxi0::subject:serialNumber:00000010",
		"sub": "did:web:example.com:iam:plataan",
		"jti": "urn:uuid:5f0ff0a1-2eb3-4f30-b3a1-6f2a54a5c7",
	}, "VerifiableCredential", "X509Credential")
	payload := strings.Split(credential, ".")[1]
	require.NotZero(t, len(payload)%4, "this credential no longer exercises base64 padding")
	return credential
}

func (n *fakeNode) refuseCredentials() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.rejectStores = true
}

func (n *fakeNode) breakSubjectList() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failSubjectList = true
}

func (n *fakeNode) breakSubjectCreate() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failSubjectCreate = true
}

func (n *fakeNode) breakWallet() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failWallet = true
}

func bootstrap(nodeURL string, env ...string) *exec.Cmd {
	cmd := exec.Command("bash", "bootstrap-nuts.sh")
	cmd.Env = append(cmd.Environ(), "NUTS_INTERNAL_BASE_URL="+nodeURL)
	cmd.Env = append(cmd.Env, env...)
	return cmd
}

// skipDidx509 turns off the toolkit call and puts a docker that refuses to run
// on PATH, so a run that reaches for a container fails instead of quietly
// making the suite depend on one.
func skipDidx509(t *testing.T) []string {
	t.Helper()
	stubDir := t.TempDir()
	stub := "#!/usr/bin/env bash\necho 'docker must not be called with SANDBOX_SKIP_DIDX509=1' >&2\nexit 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(stubDir, "docker"), []byte(stub), 0o755))
	return []string{
		"SANDBOX_SKIP_DIDX509=1",
		"PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
}

// bootstrapWithoutBaseURL leaves NUTS_INTERNAL_BASE_URL out of the
// environment, which bootstrap always sets, so that the default an operator
// actually runs with can be exercised at all.
func bootstrapWithoutBaseURL(env ...string) *exec.Cmd {
	cmd := exec.Command("bash", "bootstrap-nuts.sh")
	for _, entry := range cmd.Environ() {
		if !strings.HasPrefix(entry, "NUTS_INTERNAL_BASE_URL=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, env...)
	return cmd
}

// runBootstrapIn runs the script from a working directory of the test's
// choosing. $CERTS is derived from the script's own location, so a test that
// needs to control what sits beside the certificates runs a copy from
// scriptCopy rather than the checkout. The script is invoked by a relative
// path either way, which keeps the absolute path it builds under test.
func runBootstrapIn(t *testing.T, dir, nodeURL string, env ...string) (string, error) {
	t.Helper()
	cmd := bootstrap(nodeURL, env...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runBootstrap(t *testing.T, nodeURL string, env ...string) string {
	t.Helper()
	out, err := runBootstrapIn(t, ".", nodeURL, env...)
	require.NoError(t, err, "bootstrap failed: %s", out)
	return out
}

func runBootstrapExpectingFailure(t *testing.T, nodeURL string, env ...string) string {
	t.Helper()
	out, err := runBootstrapIn(t, ".", nodeURL, env...)
	require.Error(t, err, "bootstrap reported success: %s", out)
	return out
}

// scriptCopy puts the script alone in a temporary directory, so a test decides
// whether the certificates it looks for are there. The real .certs is
// gitignored, so a test reading it would pass or fail on whether the operator
// had generated the demo material.
func scriptCopy(t *testing.T) string {
	t.Helper()
	// Resolved, because the script reports its own location with pwd, and on
	// macOS the temporary directory is reached through a symlink.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	script, err := os.ReadFile("bootstrap-nuts.sh")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bootstrap-nuts.sh"), script, 0o755))
	return dir
}

// seedCertificates puts the two files generate-demo-certs.sh produces beside a
// script copy. Only a stubbed toolkit ever opens them, so their contents are
// irrelevant and their presence is the whole point.
func seedCertificates(t *testing.T, dir string) {
	t.Helper()
	certs := filepath.Join(dir, ".certs")
	require.NoError(t, os.MkdirAll(certs, 0o755))
	for _, name := range []string{"plataan-uzi-chain.pem", "plataan-uzi.key"} {
		require.NoError(t, os.WriteFile(filepath.Join(certs, name), []byte("demo material\n"), 0o600))
	}
}

// didx509Stub is a docker on PATH that records how the toolkit was called and
// answers with a credential, so the real path can be driven without a
// container.
type didx509Stub struct {
	env           []string
	argumentsFile string
}

func stubDidx509(t *testing.T, credential string) didx509Stub {
	t.Helper()
	stubDir := t.TempDir()
	argumentsFile := filepath.Join(stubDir, "arguments")
	stub := fmt.Sprintf("#!/usr/bin/env bash\nprintf '%%s\\n' \"$@\" > %q\nprintf '%%s\\n' %q\n",
		argumentsFile, credential)
	require.NoError(t, os.WriteFile(filepath.Join(stubDir, "docker"), []byte(stub), 0o755))
	return didx509Stub{
		env:           []string{"PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH")},
		argumentsFile: argumentsFile,
	}
}

func (s didx509Stub) arguments(t *testing.T) []string {
	t.Helper()
	recorded, err := os.ReadFile(s.argumentsFile)
	require.NoError(t, err, "the script must invoke the toolkit")
	return strings.Split(strings.TrimSuffix(string(recorded), "\n"), "\n")
}

func (s didx509Stub) requireNotCalled(t *testing.T) {
	t.Helper()
	_, err := os.Stat(s.argumentsFile)
	require.ErrorIs(t, err, os.ErrNotExist, "the toolkit must not run before its inputs are checked")
}

func TestBootstrapIsIdempotent(t *testing.T) {
	node, nodeURL := newFakeNode(t)

	runBootstrap(t, nodeURL, skipDidx509(t)...)
	creates, stores := node.counts()
	require.Equal(t, 1, creates)
	require.Equal(t, 1, stores)
	require.Equal(t, []string{"plataan"}, node.subjectNames(),
		"the create must name the subject; without a name the node generates a uuid and the wallet calls address nothing")
	require.Len(t, node.walletRaw("plataan"), 1)

	// The reference helpers in test/e2e/pep/authorization_test.go always create
	// and always store. Re-running this script must do neither, or the compose
	// init step fails on every restart.
	runBootstrap(t, nodeURL, skipDidx509(t)...)
	creates, stores = node.counts()
	require.Equal(t, 1, creates, "a second run must not create the subject again")
	require.Equal(t, 1, stores, "a second run must not store the credential again")
}

func TestBootstrapReadsEveryCredentialFormatTheWalletReturns(t *testing.T) {
	node, nodeURL := newFakeNode(t)
	node.seedSubject("plataan")
	node.seedWallet("plataan",
		// A JSON-LD credential with one type marshals that type as a bare
		// string (go-did vc/vc.go, marshal.Unplural).
		`{"type":"OrganizationCredential"}`,
		`{"type":["VerifiableCredential","NutsAuthorizationCredential"]}`,
		// Anything stored as a JWT comes back as a JWT.
		`"`+jwtCredential("VerifiableCredential", "NutsAuthorizationCredential")+`"`,
		// Strings that are not credentials at all must be skipped rather than
		// abort the run: too few segments, undecodable, and decodable but not
		// JSON.
		`"not-a-jwt"`,
		`"not.a.jwt"`,
		`"not.aGVsbG8.jwt"`,
		// Decodable and valid JSON, but not an object: a payload of 1 and a
		// payload of ["x"]. A wallet entry that is not a credential at all is
		// the same case one level up.
		`"a.MQ.b"`,
		`"a.WyJ4Il0.b"`,
		`null`,
		`42`,
		// A bare-string type has to be compared whole, not by substring.
		`{"type":"X509CredentialArchive"}`,
	)

	runBootstrap(t, nodeURL, skipDidx509(t)...)
	creates, stores := node.counts()
	require.Equal(t, 0, creates, "the subject already exists")
	require.Equal(t, 1, stores, "none of the seeded credentials is an X509Credential")

	runBootstrap(t, nodeURL, skipDidx509(t)...)
	_, stores = node.counts()
	require.Equal(t, 1, stores, "the X509Credential must be found among unrelated credentials")
}

func TestBootstrapDoesNothingWhenTheWalletAlreadyHoldsTheCredential(t *testing.T) {
	node, nodeURL := newFakeNode(t)
	node.seedSubject("plataan")
	node.seedWallet("plataan", `"`+x509CredentialJWT(t)+`"`)

	runBootstrap(t, nodeURL, skipDidx509(t)...)
	creates, stores := node.counts()
	require.Zero(t, creates)
	require.Zero(t, stores, "a credential issued by an earlier run must be recognised")
}

// A problem document parses as JSON, so nothing but the status says that the
// answer is not an answer.
//
// What each case additionally requires is that the operator can act on the
// failure. The node explains itself in an RFC 7807 body, and that body is the
// only account of why it refused; the exit status alone leaves the operator
// with a half-created subject and nothing that says which step made it or what
// went wrong. So every case asserts the step, the status and the node's own
// detail reach the output, not merely that the run failed.
func TestBootstrapFailsWhenTheNodeReturnsAnError(t *testing.T) {
	t.Run("listing subjects", func(t *testing.T) {
		node, nodeURL := newFakeNode(t)
		node.breakSubjectList()

		out := runBootstrapExpectingFailure(t, nodeURL, skipDidx509(t)...)
		requireDiagnosis(t, out, "Listing subjects", "500", "could not list subjects")
		creates, stores := node.counts()
		require.Zero(t, creates, "an unreadable subject list must not be read as an absent subject")
		require.Zero(t, stores)
	})

	t.Run("creating the subject", func(t *testing.T) {
		node, nodeURL := newFakeNode(t)
		node.breakSubjectCreate()

		out := runBootstrapExpectingFailure(t, nodeURL, skipDidx509(t)...)
		require.NotContains(t, out, "Stored the X509Credential")
		requireDiagnosis(t, out, "Creating the subject", "500", "could not create the subject")
		creates, stores := node.counts()
		require.Equal(t, 1, creates, "the create was attempted and refused")
		require.Zero(t, stores, "a subject that does not exist must not be given a credential")
	})

	t.Run("reading the wallet", func(t *testing.T) {
		node, nodeURL := newFakeNode(t)
		node.seedSubject("plataan")
		node.breakWallet()

		out := runBootstrapExpectingFailure(t, nodeURL, skipDidx509(t)...)
		requireDiagnosis(t, out, "Reading the wallet", "500", "could not read the wallet")
		_, stores := node.counts()
		require.Zero(t, stores, "an unreadable wallet must not be read as an empty one")
	})
}

// requireDiagnosis is the shape every node failure has to arrive in: which step
// was being attempted, what the node answered, and what it said about why.
func requireDiagnosis(t *testing.T, out, step, status, detail string) {
	t.Helper()
	require.Contains(t, out, step, "the failure must name the step, or the operator cannot tell which call failed")
	require.Contains(t, out, status, "the HTTP status is what distinguishes a refusal from a problem document that parses")
	require.Contains(t, out, detail, "the node's own problem detail is the only account of why it refused")
}

func TestBootstrapHonoursTheConfiguredSubject(t *testing.T) {
	node, nodeURL := newFakeNode(t)
	subject := "de-plataan"

	runBootstrap(t, nodeURL, append(skipDidx509(t), "SANDBOX_NUTS_SUBJECT="+subject)...)
	require.Equal(t, []string{subject}, node.subjectNames())
	require.Len(t, node.walletRaw(subject), 1, "the credential belongs in the configured subject's wallet")

	runBootstrap(t, nodeURL, append(skipDidx509(t), "SANDBOX_NUTS_SUBJECT="+subject)...)
	creates, stores := node.counts()
	require.Equal(t, 1, creates)
	require.Equal(t, 1, stores)
}

func TestBootstrapDefaultsToTheNodesPublishedInternalAPI(t *testing.T) {
	// A curl that records how it was called and then fails makes the default
	// observable without opening a socket. Letting the script reach 8081 for
	// real would mean creating subjects in whatever node the operator has
	// running, which is exactly the node this default points at.
	stubDir := t.TempDir()
	argumentsFile := filepath.Join(stubDir, "arguments")
	stub := fmt.Sprintf("#!/usr/bin/env bash\nprintf '%%s\\n' \"$@\" > %q\nexit 1\n", argumentsFile)
	require.NoError(t, os.WriteFile(filepath.Join(stubDir, "curl"), []byte(stub), 0o755))

	cmd := bootstrapWithoutBaseURL("PATH=" + stubDir + string(os.PathListSeparator) + os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "a curl that fails must fail the run: %s", out)

	recorded, err := os.ReadFile(argumentsFile)
	require.NoError(t, err, "the script must have called curl")
	arguments := strings.Split(strings.TrimSuffix(string(recorded), "\n"), "\n")
	require.Equal(t, "http://localhost:8081/nuts/internal/vdr/v2/subject", arguments[len(arguments)-1],
		"the default has to reach the port docker-compose.yml publishes for the node")
}

func TestBootstrapFailsWhenTheNodeIsUnreachable(t *testing.T) {
	// Nothing listens on port 1. A compose init step that exits 0 here would
	// leave the demo running against an empty wallet.
	out := runBootstrapExpectingFailure(t, "http://127.0.0.1:1", skipDidx509(t)...)
	require.NotContains(t, out, "Stored the X509Credential")

	// A transport failure has no problem document to report, so the address is
	// the whole of the diagnosis: "the node refused" and "there is no node
	// there" are the same exit status and want different actions from the
	// operator.
	require.Contains(t, out, "Listing subjects", "the failure must name the step")
	require.Contains(t, out, "http://127.0.0.1:1",
		"an unreachable node must be reported with the address that was tried")
}

func TestBootstrapFailsWhenTheNodeRejectsTheCredential(t *testing.T) {
	node, nodeURL := newFakeNode(t)
	node.refuseCredentials()

	out := runBootstrapExpectingFailure(t, nodeURL, skipDidx509(t)...)
	require.NotContains(t, out, "Stored the X509Credential")

	// The most expensive failure to diagnose without this: the subject exists
	// by now, so a rerun reports nothing to do while the wallet stays empty.
	requireDiagnosis(t, out, "Storing the X509Credential", "400", "invalid credential")

	_, stores := node.counts()
	require.Equal(t, 1, stores, "the credential was offered and refused")
	require.Empty(t, node.walletRaw("plataan"))
}

func TestBootstrapIssuesTheCredentialFromTheDemoCertificates(t *testing.T) {
	node, nodeURL := newFakeNode(t)
	dir := scriptCopy(t)
	seedCertificates(t, dir)

	// The real path shells out to the didx509 toolkit, so the arguments that
	// have to match the demo certificates are checked against a stub.
	credential := jwtCredential("VerifiableCredential", "X509Credential")
	toolkit := stubDidx509(t, credential)

	out, err := runBootstrapIn(t, dir, nodeURL, toolkit.env...)
	require.NoError(t, err, "bootstrap failed: %s", out)

	certs := filepath.Join(dir, ".certs")
	require.Equal(t, []string{
		"run", "--rm",
		// Absolute, because a bind mount is resolved against the daemon's
		// notion of the path, not the shell's.
		"-v", certs + "/plataan-uzi-chain.pem:/cert-chain.pem:ro",
		"-v", certs + "/plataan-uzi.key:/cert-key.key:ro",
		"nutsfoundation/go-didx509-toolkit:1.2.0",
		"vc", "/cert-chain.pem", "/cert-key.key",
		// The issuer of the demo chain, from generate-demo-certs.sh.
		"CN=GF Sandbox Demo CA",
		"did:web:example.com:iam:plataan",
	}, toolkit.arguments(t))

	require.Equal(t, []string{`"` + credential + `"`}, node.walletRaw("plataan"),
		"the credential must be stored as a JSON string, as the node parses the body as JSON")
}

// Handing a missing path to docker run -v has the daemon create a directory
// there, and these two paths sit inside generate-demo-certs.sh's own output
// directory: that script would then read the material as absent, rewrite the
// CA and die on the directory, leaving half a rotated PKI and no documented
// way back.
func TestBootstrapRefusesToIssueWithoutTheDemoCertificates(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		setUp   func(t *testing.T, dir string)
		missing string
	}{
		{
			name:    "a checkout where the generator has not been run",
			setUp:   func(*testing.T, string) {},
			missing: "plataan-uzi-chain.pem",
		},
		{
			// Both are mounted, so checking only the first still leaves one
			// path for the daemon to create.
			name: "half the material",
			setUp: func(t *testing.T, dir string) {
				seedCertificates(t, dir)
				require.NoError(t, os.Remove(filepath.Join(dir, ".certs", "plataan-uzi.key")))
			},
			missing: "plataan-uzi.key",
		},
		{
			// What a run without this guard leaves behind, and the reason the
			// check is -f: a directory is perfectly readable.
			name: "a directory where an earlier run wedged the bind mount",
			setUp: func(t *testing.T, dir string) {
				seedCertificates(t, dir)
				chain := filepath.Join(dir, ".certs", "plataan-uzi-chain.pem")
				require.NoError(t, os.Remove(chain))
				require.NoError(t, os.Mkdir(chain, 0o755))
			},
			missing: "plataan-uzi-chain.pem",
		},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			node, nodeURL := newFakeNode(t)
			dir := scriptCopy(t)
			scenario.setUp(t, dir)
			toolkit := stubDidx509(t, jwtCredential("VerifiableCredential", "X509Credential"))

			out, err := runBootstrapIn(t, dir, nodeURL, toolkit.env...)
			require.Error(t, err, "bootstrap reported success: %s", out)
			require.Contains(t, out, scenario.missing, "the message must name the file it needs")
			require.Contains(t, out, "generate-demo-certs.sh", "and the script that produces it")
			toolkit.requireNotCalled(t)

			_, stores := node.counts()
			require.Zero(t, stores, "nothing was issued, so nothing can be stored")
		})
	}
}
