package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/seed"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
)

func main() {
	if len(os.Args) < 3 {
		panic("Usage: " + os.Args[0] + " <internal API base path> <HAPI FHIR multitenant base URL>")
	}
	internalAPI := strings.TrimRight(os.Args[1], "/")
	hapiBaseURL, err := url.Parse(os.Args[2])
	if err != nil {
		panic("invalid HAPI FHIR base URL: " + err.Error())
	}

	println("Loading FHIR testdata into HAPI...")
	if _, err := vectors.Load(hapiBaseURL); err != nil {
		panic("Unable to load testdata: " + err.Error())
	}

	// Create a did:web subject per demo organization and record its DID.
	//
	// Discovery registration deliberately does NOT happen here: registering
	// builds a Verifiable Presentation from the subject's wallet, which is still
	// empty at this point. The credential-issuer services mint an X509Credential
	// for each DID first, and the credentials seed (cmd/credentials) stores it and
	// registers afterwards.
	bootstrapNutsSubjects(internalAPI)

	// Register each pool patient's NVI localization Lists through the Knooppunt.
	// This is part of the core seed and must succeed.
	println("Registering NVI localization records...")
	internalBaseURL, err := url.Parse(internalAPI)
	if err != nil {
		panic("invalid internal API base URL: " + err.Error())
	}
	if err := vectors.SeedNVI(context.Background(), internalBaseURL); err != nil {
		panic("Unable to seed NVI: " + err.Error())
	}

	// Run the mCSD sync so the seeded admin directories flow into the query
	// directory. Without this the demo organizations exist but are not
	// resolvable to an address, and there is no timer that would eventually do
	// it — AC1 requires the deployment to be addressable with no manual step.
	println("Running mCSD update...")
	if err := invokeMCSDUpdate(internalAPI); err != nil {
		panic("Unable to run mCSD update: " + err.Error())
	}

	println("Seed complete.")
}

// bootstrapNutsSubjects ensures a did:web subject exists per demo organization
// and, when SEED_DID_DIR is set, records each subject's DID for the credential
// issuers.
//
// The seed is re-runnable against a node that already has the subjects, in which
// case creation fails and the existing DID is read back instead — the credential
// issuers need a DID either way.
func bootstrapNutsSubjects(internalAPI string) {
	didDir := os.Getenv(seed.DIDDirEnvVar)
	for _, org := range seed.Organizations() {
		did, err := createNutsSubject(internalAPI, org.Subject)
		if err != nil {
			println("Note: could not create Nuts subject " + org.Subject + " (" + err.Error() + "); looking up existing subject")
			did, err = resolveSubjectDID(internalAPI, org.Subject)
			if err != nil {
				println("Warn: no DID available for subject " + org.Subject + ": " + err.Error())
				continue
			}
			println("Using existing Nuts subject " + org.Subject + " (" + did + ")")
		} else {
			println("Created Nuts subject " + org.Subject + " (" + did + ")")
		}

		if didDir == "" {
			continue
		}
		// Named by organization key rather than URA, so the compose services that
		// read these files stay readable.
		path := filepath.Join(didDir, org.Key+".did")
		if err := os.WriteFile(path, []byte(did), 0o644); err != nil {
			panic("unable to write DID for " + org.Subject + ": " + err.Error())
		}
		println("Wrote " + path)
	}
}

// invokeMCSDUpdate triggers a synchronization of the configured mCSD
// administration directories into the query directory. The sync is
// request-driven (there is no background timer), so the seed has to ask for it.
func invokeMCSDUpdate(internalAPI string) error {
	httpResponse, err := http.Post(internalAPI+"/mcsd/update", "application/json", nil)
	if err != nil {
		return err
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseData, _ := io.ReadAll(httpResponse.Body)
		return fmt.Errorf("unexpected status code (status=%s, expected=200)\nResponse data:\n%s",
			httpResponse.Status, strings.TrimSpace(string(responseData)))
	}
	return nil
}

// resolveSubjectDID returns the preferred DID of an existing Nuts subject.
func resolveSubjectDID(internalAPI, subject string) (string, error) {
	httpResponse, err := http.Get(internalAPI + "/nuts/internal/vdr/v2/subject/" + subject)
	if err != nil {
		return "", err
	}
	dids, err := readJSONResponse[[]string](httpResponse, http.StatusOK)
	if err != nil {
		return "", fmt.Errorf("failed to look up Nuts subject: %w", err)
	}
	if len(dids) == 0 {
		return "", fmt.Errorf("subject %s has no DIDs", subject)
	}
	return dids[0], nil
}

// createNutsSubject creates a Nuts subject, returning its preferred DID.
func createNutsSubject(internalAPI string, subject string) (string, error) {
	httpResponse, err := http.Post(internalAPI+"/nuts/internal/vdr/v2/subject", "application/json", strings.NewReader(`{"subject":"`+subject+`"}`))
	if err != nil {
		return "", err
	}

	type ResultDocument struct {
		ID string `json:"id"`
	}
	type Result struct {
		Documents []ResultDocument `json:"documents"`
	}
	result, err := readJSONResponse[Result](httpResponse, http.StatusOK)
	if err != nil {
		return "", fmt.Errorf("failed to create Nuts subject: %w", err)
	}
	if len(result.Documents) == 0 {
		return "", fmt.Errorf("subject created but no documents returned")
	}
	return result.Documents[0].ID, err
}

func readJSONResponse[T any](httpResponse *http.Response, expectedStatus int) (T, error) {
	var result T
	if httpResponse.StatusCode != expectedStatus {
		responseData, _ := io.ReadAll(httpResponse.Body)
		return result, fmt.Errorf("unexpected status code (status=%s, expected=%d, url=%s)\nResponse data:\n----------------\n%s\n----------------", httpResponse.Status, expectedStatus, httpResponse.Request.URL, strings.TrimSpace(string(responseData)))
	}
	if err := json.NewDecoder(httpResponse.Body).Decode(&result); err != nil {
		return result, fmt.Errorf("failed to decode response body: %w", err)
	}
	return result, nil
}
