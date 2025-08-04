package updateclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// UpdateClient is a client for getting updates from an update server
type UpdateClient struct {
	client  *http.Client
	baseURL string
}

// NewUpdateClient creates a new UpdateClient
func NewUpdateClient(options ...func(*UpdateClient)) *UpdateClient {
	client := &UpdateClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "http://localhost:8080", // Default base URL
	}

	// Apply options
	for _, option := range options {
		option(client)
	}

	return client
}

// WithBaseURL sets the base URL for the client
func WithBaseURL(baseURL string) func(*UpdateClient) {
	return func(c *UpdateClient) {
		c.baseURL = baseURL
	}
}

// WithClient sets the HTTP client for the client
func WithClient(httpClient *http.Client) func(*UpdateClient) {
	return func(c *UpdateClient) {
		c.client = httpClient
	}
}

// GetUpdate gets an update for the given parameters
// If since is not nil, it will be used as the _since query parameter
// Returns the Bundle containing history data or an error
func (c *UpdateClient) GetUpdate(basePath string, since *time.Time) (*fhir.Bundle, error) {
	// Create a request with query parameters if needed
	req, err := http.NewRequest("GET", c.baseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	path := fmt.Sprintf("%s/_history", basePath)
	req.URL.Path = path

	// Add query parameters
	q := req.URL.Query()
	if since != nil {
		// Format time as ISO8601
		q.Add("_since", since.Format(time.RFC3339))
		req.URL.RawQuery = q.Encode()
	}

	fmt.Println("Request URL:", req.URL.String())

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse the response into our Bundle struct
	var bundle fhir.Bundle
	if err := json.NewDecoder(resp.Body).Decode(&bundle); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Validate the bundle
	if bundle.Type != fhir.BundleTypeHistory {
		return nil, fmt.Errorf("expected Bundle of 'history' type, got %s", bundle.Type)
	}

	return &bundle, nil
}
