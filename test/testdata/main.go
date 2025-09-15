package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors"
)

func main() {
	if len(os.Args) < 3 {
		panic("Usage: " + os.Args[0] + " <internal API base path> <HAPI FHIR multitenant base URL>")
	}
	internalAPI := os.Args[1]
	hapiBaseURL, _ := url.Parse(os.Args[2])
	tenantName := "orgA"
	if err := createTenant(internalAPI, tenantName); err != nil {
		println("Warn: Unable to create tenant: " + err.Error())
	} else {
		println("Created tenant:", tenantName)
	}

	println("Loading FHIR testdata into HAPI...")
	if _, err := vectors.Load(hapiBaseURL); err != nil {
		panic("Unable to load testdata: " + err.Error())
	}

}

func createTenant(internalAPI string, subjectID string) error {
	_, err := createNutsSubject(internalAPI, subjectID)
	return err
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
