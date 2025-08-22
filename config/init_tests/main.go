package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		panic("Usage: " + os.Args[0] + " <internal API base path>")
	}
	internalAPI := os.Args[1]
	tenantName := "orgA"
	if err := createTenant(internalAPI, tenantName); err != nil {
		panic("Unable to create tenant: " + err.Error())
	}
	println("Created tenant:", tenantName)
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
