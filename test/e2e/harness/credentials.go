package harness

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The requester side of an exchange: a Nuts subject, an X509 credential in its
// wallet proving which organization it is, and a registration on the discovery
// service the scope's presentation definition reads from. A token request fails
// closed without all three, and the failures name the token endpoint rather than
// the step that was skipped, so these live together.

// CreateSubject creates a Nuts subject and returns the DID of its first
// document.
func CreateSubject(t *testing.T, nutsAPI func(string) string, subject string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"subject": subject})
	require.NoError(t, err)

	res, err := http.Post(nutsAPI("/internal/vdr/v2/subject"), "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()

	responseBody, _ := io.ReadAll(res.Body)
	require.Equal(t, http.StatusOK, res.StatusCode, "create subject %s: %s", subject, responseBody)

	var result struct {
		Documents []struct {
			ID string `json:"id"`
		} `json:"documents"`
	}
	require.NoError(t, json.Unmarshal(responseBody, &result))
	require.NotEmpty(t, result.Documents, "a created subject has at least one DID document")
	return result.Documents[0].ID
}

// IssueX509Credential runs the didx509 toolkit over a certificate chain to
// produce the credential that proves the holder's URA. The toolkit runs in
// Docker rather than as a dependency, which is also how the repository issues
// these outside tests.
func IssueX509Credential(t *testing.T, chainPath, keyPath, subjectDID string) string {
	t.Helper()
	command := exec.Command("docker", "run", "--rm",
		"-v", chainPath+":/cert-chain.pem:ro",
		"-v", keyPath+":/cert-key.key:ro",
		"nutsfoundation/go-didx509-toolkit:main",
		"vc", "/cert-chain.pem", "/cert-key.key", "CN=Fake UZI Root CA", subjectDID,
	)
	output, err := command.Output()
	if exitErr, ok := err.(*exec.ExitError); ok {
		t.Fatalf("go-didx509-toolkit failed: %s\nstderr: %s", err, exitErr.Stderr)
	}
	require.NoError(t, err, "go-didx509-toolkit failed")
	return strings.TrimSpace(string(output))
}

// StoreCredential puts a credential in a subject's wallet.
func StoreCredential(t *testing.T, nutsAPI func(string) string, holder, credential string) {
	t.Helper()
	res, err := http.Post(nutsAPI("/internal/vcr/v2/holder/"+holder+"/vc"),
		"application/json", bytes.NewReader([]byte(`"`+credential+`"`)))
	require.NoError(t, err)
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	require.Equal(t, http.StatusNoContent, res.StatusCode, "store credential for %s: %s", holder, body)
}

// RegisterOnDiscovery registers a subject on the bgz-test discovery service,
// which is the one the scopes in config/policy resolve their presentation
// definitions against.
func RegisterOnDiscovery(t *testing.T, nutsAPI func(string) string, subject string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"registrationParameters": map[string]string{"fhirBaseURL": "http://example.com/fhir"},
	})
	require.NoError(t, err)

	res, err := http.Post(nutsAPI("/internal/discovery/v1/bgz-test/"+subject),
		"application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()

	responseBody, _ := io.ReadAll(res.Body)
	require.Equal(t, http.StatusOK, res.StatusCode, "register %s on discovery: %s", subject, responseBody)
}

// RegisterRequester does the three steps a party needs before it can ask for a
// token, and returns the subject's DID.
func RegisterRequester(t *testing.T, nutsAPI func(string) string, subject, chainPath, keyPath string) string {
	t.Helper()
	did := CreateSubject(t, nutsAPI, subject)
	StoreCredential(t, nutsAPI, subject, IssueX509Credential(t, chainPath, keyPath, did))
	RegisterOnDiscovery(t, nutsAPI, subject)
	return did
}
