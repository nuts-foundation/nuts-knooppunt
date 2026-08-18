package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
)

// discoveryServiceID is the Nuts discovery service the demo organizations
// register on. Registration is best-effort: locally the compose Nuts node has
// no discovery definitions configured, so it is expected to fail there and only
// matters once the service is defined (hosted / E4 token flow).
const discoveryServiceID = "bgz-test"

// bootstrapSubjects are the demo organizations that get a did:web subject plus a
// (best-effort) discovery registration. The subject name is the organization's
// URA so it is stable and recognizable.
func bootstrapSubjects() []struct {
	Subject     string
	FHIRBaseURL string
} {
	return []struct {
		Subject     string
		FHIRBaseURL string
	}{
		// Ziekenhuis De Plataan (consumer/requester) — revived per E5.
		{Subject: plataan.URA, FHIRBaseURL: "http://localhost:7050/fhir/plataan-patients"},
		// Zorgcentrum De Zonnebloem (source / data holder).
		{Subject: *sunflower.Organization().Identifier[0].Value, FHIRBaseURL: "http://localhost:7050/fhir/sunflower-patients"},
	}
}

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

	// Revive the subject bootstrap: create a did:web subject per demo
	// organization and register it on the discovery service. Best-effort — see
	// discoveryServiceID. The URA credential is E2's responsibility (TODO(E2)).
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
	println("Seed complete.")
}

// bootstrapNutsSubjects creates a did:web subject per demo organization and, for
// each, attempts a discovery registration. Failures are logged as warnings and
// do not abort the seed (the local Nuts node has no discovery service defined;
// TODO(E2): issue the URA credential once the mock VC issuer is wired).
func bootstrapNutsSubjects(internalAPI string) {
	for _, org := range bootstrapSubjects() {
		did, err := createNutsSubject(internalAPI, org.Subject)
		if err != nil {
			println("Warn: unable to create Nuts subject " + org.Subject + ": " + err.Error())
			continue
		}
		println("Created Nuts subject " + org.Subject + " (" + did + ")")

		if err := registerOnDiscovery(internalAPI, org.Subject, org.FHIRBaseURL); err != nil {
			println("Warn: unable to register subject " + org.Subject + " on discovery '" + discoveryServiceID + "': " + err.Error())
		}
	}
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
		internalAPI+"/nuts/internal/discovery/v1/"+discoveryServiceID+"/"+subject,
		"application/json",
		strings.NewReader(string(body)),
	)
	if err != nil {
		return err
	}
	if _, err := readJSONResponse[map[string]any](httpResponse, http.StatusOK); err != nil {
		return err
	}
	return nil
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
