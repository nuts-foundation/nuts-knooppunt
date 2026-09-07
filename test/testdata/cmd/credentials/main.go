// Command credentials completes the local credential bootstrap: it stores the
// X509Credential minted for each demo organization in that organization's Nuts
// wallet, then registers the organization on the discovery service.
//
// This is the third phase of the compose seed pipeline:
//
//	init                  creates the Nuts subjects, writes <org>.did
//	credential-issuer-*   mints an X509Credential per DID, writes <org>.jwt
//	credentials (this)    stores the credentials and registers on discovery
//
// The split exists because did:web DIDs embed a fresh UUID per subject creation,
// so credentials cannot be pre-generated and committed — they must be issued
// against the DID that actually exists (see seed.DIDDirEnvVar). Registration has
// to come last: it builds a Verifiable Presentation from the subject's wallet,
// which is empty until the credential is stored.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/seed"
)

func main() {
	if len(os.Args) < 2 {
		panic("Usage: " + os.Args[0] + " <internal API base path>")
	}
	internalAPI := strings.TrimRight(os.Args[1], "/")

	sharedDir := os.Getenv(seed.DIDDirEnvVar)
	if sharedDir == "" {
		panic(seed.DIDDirEnvVar + " must be set to the directory holding the issued credentials")
	}

	for _, org := range seed.Organizations() {
		jwtPath := filepath.Join(sharedDir, org.Key+".jwt")
		credential, err := os.ReadFile(jwtPath)
		if err != nil {
			panic("unable to read credential for " + org.Key + ": " + err.Error())
		}

		if err := storeCredential(internalAPI, org.Subject, strings.TrimSpace(string(credential))); err != nil {
			panic("unable to store credential for " + org.Subject + ": " + err.Error())
		}
		println("Stored X509Credential in wallet of " + org.Subject)

		if err := registerOnDiscovery(internalAPI, org.Subject, org.FHIRBaseURL); err != nil {
			panic("unable to register subject " + org.Subject + " on discovery '" + seed.DiscoveryServiceID + "': " + err.Error())
		}
		println("Registered " + org.Subject + " on discovery service " + seed.DiscoveryServiceID + " at " + org.FHIRBaseURL)
	}

	println("Credential seed complete.")
}

// storeCredential loads a Verifiable Credential into a subject's wallet. The
// credential is a JWT, and the endpoint takes it as a bare JSON string.
func storeCredential(internalAPI, subject, credential string) error {
	body := []byte(`"` + credential + `"`)

	httpResponse, err := http.Post(
		internalAPI+"/nuts/internal/vcr/v2/holder/"+subject+"/vc",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()

	// Storing an already-stored credential is not an error: the seed is
	// re-runnable, and a conflict means the wallet already holds it.
	if httpResponse.StatusCode == http.StatusConflict {
		return nil
	}
	return expectStatus(httpResponse, http.StatusNoContent)
}

// registerOnDiscovery activates the discovery service for the subject, so it is
// findable by other participants. registrationParameters carries the subject's
// FHIR base URL.
func registerOnDiscovery(internalAPI, subject, fhirBaseURL string) error {
	body, _ := json.Marshal(map[string]any{
		"registrationParameters": map[string]string{
			"fhirBaseURL": fhirBaseURL,
		},
	})

	httpResponse, err := http.Post(
		internalAPI+"/nuts/internal/discovery/v1/"+seed.DiscoveryServiceID+"/"+subject,
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()

	return expectStatus(httpResponse, http.StatusOK)
}

func expectStatus(httpResponse *http.Response, expectedStatus int) error {
	if httpResponse.StatusCode != expectedStatus {
		responseData, _ := io.ReadAll(httpResponse.Body)
		return fmt.Errorf("unexpected status code (status=%s, expected=%d, url=%s)\nResponse data:\n----------------\n%s\n----------------",
			httpResponse.Status, expectedStatus, httpResponse.Request.URL, strings.TrimSpace(string(responseData)))
	}
	return nil
}
