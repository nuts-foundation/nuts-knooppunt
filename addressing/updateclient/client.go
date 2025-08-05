package updateclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
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

// GetHistoryBundle gets an update for the given parameters
// If since is not nil, it will be used as the _since query parameter
// Returns the Bundle containing history data or an error
func (c *UpdateClient) GetHistoryBundle(basePath string, since *time.Time) (*fhir.Bundle, error) {
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

// orgsPerDirectory is a map that associates authoritative directories (URLs) with a list of their organization IDs.
type orgsPerDirectory map[url.URL][]string

// GetOrganisationsPerDirectory extracts the authoritative directories and their associated organization IDs from a FHIR Bundle.
func (c *UpdateClient) GetOrganisationsPerDirectory(historyBundle *fhir.Bundle) (orgsPerDirectory, error) {

	dirOrgMap := make(orgsPerDirectory)

	// loop twice over the entries, first to map the org references to their identifiers
	// refIdMap is a map that associates organization IDs with their identifiers.
	refIdMap := make(map[string]string)
	for _, entry := range historyBundle.Entry {
		if entry.Resource == nil {
			continue
		}

		resourceType, err := extractResourceType(entry.Resource)
		if err != nil {
			fmt.Printf("Failed to extract resource type: %v\n", err)
			continue // Skip if resourceType cannot be determined
		}

		if resourceType == "Organization" {
			org := &fhir.Organization{}
			if err := json.Unmarshal(entry.Resource, org); err != nil {
				fmt.Printf("Failed to unmarshal Organization resource: %v\n", err)
				continue
			}

			if org.Id == nil || *org.Id == "" {
				continue // Skip if no organization ID
			}
			if org.Identifier == nil || len(org.Identifier) == 0 {
				continue // Skip if no identifier
			}

			refIdMap[*org.Id] = *org.Identifier[0].Value // Assuming the first identifier is the authoritative one
		}
	}

	for _, entry := range historyBundle.Entry {
		if entry.Resource == nil {
			continue
		}

		resourceType, err := extractResourceType(entry.Resource)
		if err != nil {
			fmt.Printf("Failed to extract resource type: %v\n", err)
			continue // Skip if resourceType cannot be determined
		}

		if resourceType == "Endpoint" {
			endpoint := &fhir.Endpoint{}
			if err := json.Unmarshal(entry.Resource, endpoint); err != nil {
				fmt.Printf("Failed to unmarshal Endpoint resource: %v\n", err)
				continue
			}

			if endpoint.ConnectionType.Code == nil || *endpoint.ConnectionType.Code != "mcsd-directory" {
				continue // Skip if not a directory endpoint
			}

			if endpoint.ManagingOrganization == nil || endpoint.ManagingOrganization.Reference == nil {
				continue // Skip if no organization reference
			}
			orgRef := *endpoint.ManagingOrganization.Reference
			orgId := strings.TrimPrefix(orgRef, "Organization/") // Assuming the reference is in the format "Organization/{id}"

			orgIdentifier, ok := refIdMap[orgId]
			if !ok || orgIdentifier == "" {
				fmt.Printf("No matching organization ID found for Endpoint: %s\n", *endpoint.ManagingOrganization.Reference)
				continue // Skip if no matching organization ID
			}

			dirUrl, err := url.Parse(endpoint.Address)
			if err != nil {
				fmt.Printf("Failed to parse Endpoint address '%s': %v\n", endpoint.Address, err)
				continue // Skip if address is not a valid baseURL
			}
			dirOrgs, ok := dirOrgMap[*dirUrl]
			if !ok {
				dirOrgMap[*dirUrl] = []string{}
			}

			dirOrgMap[*dirUrl] = append(dirOrgs, orgIdentifier)
		}
	}
	return dirOrgMap, nil
}

func (c *UpdateClient) GetHistoryBundleForAuthoritativeDirectories(dirOrgMap orgsPerDirectory, since *time.Time) (map[url.URL]*fhir.Bundle, error) {
	historyBundles := make(map[url.URL]*fhir.Bundle)

	for dirUrl, orgIds := range dirOrgMap {
		dirBundle, err := c.GetHistoryBundle(dirUrl.Path, since)
		if err != nil {
			return nil, fmt.Errorf("failed to create request for %s: %w", dirUrl.String(), err)
		}

		dirBundle, err = excludeUnauthorizedEntries(dirBundle, orgIds)
		if err != nil {
			return nil, fmt.Errorf("failed to exclude unauthorized entries for %s: %w", dirUrl.String(), err)
		}
		// dirBundle.Entry = authorizedEntries.Entry
		//
		// // only keep entries from organizations that the directory is authoritative for
		// dirBundle.Entry = slices.DeleteFunc(dirBundle.Entry, func(entry fhir.BundleEntry) bool {
		// 	if entry.Resource == nil {
		// 		return true
		// 	}
		// 	resourceType, err := extractResourceType(entry.Resource)
		// 	if err != nil {
		// 		fmt.Printf("Failed to extract resource type: %v\n", err)
		// 		return true
		// 	}
		// 	if resourceType == "Organization" {
		// 		org := &fhir.Organization{}
		// 		if err := json.Unmarshal(entry.Resource, org); err != nil {
		// 			fmt.Printf("Failed to unmarshal Organization resource: %v\n", err)
		// 			return true // Skip if unmarshalling fails
		// 		}
		// 		orgIdentifier := extractIdentifier(org.Identifier, "http://fhir.nl/fhir/NamingSystem/ura")
		// 		if orgIdentifier == "" {
		// 			fmt.Printf("No identifier found for Organization resource: %s\n", *org.Id)
		// 			return true // Skip if no identifier found
		// 		}
		// 		// Check if the organization ID is in the list of orgIds
		// 		return !slices.Contains(orgIds, orgIdentifier)
		// 	}
		// 	// todo: handle other resource types
		// 	return true // remove all unknown resource types
		// })

		historyBundles[dirUrl] = dirBundle
	}

	return historyBundles, nil
}

func excludeUnauthorizedEntries(bundle *fhir.Bundle, orgIds []string) (*fhir.Bundle, error) {
	if bundle == nil {
		return nil, fmt.Errorf("bundle is nil")
	}

	// Filter entries based on the organization IDs
	bundle.Entry = slices.DeleteFunc(bundle.Entry, func(entry fhir.BundleEntry) bool {
		if entry.Resource == nil {
			return true // Remove entries with no resource
		}

		resourceType, err := extractResourceType(entry.Resource)
		if err != nil {
			fmt.Printf("Failed to extract resource type: %v\n", err)
			return true // Remove entries with unknown resource type
		}

		switch resourceType {
		// TODO: Handle other resource types
		case "Organization":
			org := &fhir.Organization{}
			if err := json.Unmarshal(entry.Resource, org); err != nil {
				fmt.Printf("Failed to unmarshal Organization resource: %v\n", err)
				return true // Remove entries with unmarshalling errors
			}

			orgIdentifier := extractIdentifier(org.Identifier, "http://fhir.nl/fhir/NamingSystem/ura")
			if orgIdentifier == "" {
				fmt.Printf("No identifier found for Organization resource: %s\n", *org.Id)
				return true // Remove entries with no identifier
			}

			return !slices.Contains(orgIds, orgIdentifier) // Keep only authorized organizations
		}

		return true // Remove all other resource types
	})

	return bundle, nil
}

func extractIdentifier(identifier []fhir.Identifier, system string) string {
	for _, c := range identifier {
		if c.System != nil && *c.System == system {
			if c.Value != nil {
				return *c.Value
			}
			return ""
		}
	}
	return ""
}

// extractResourceType is a helper function to extract the resourceType from a FHIR resource since the zorgbijjou/fhir-models package does not provide a direct way to access it.
func extractResourceType(r json.RawMessage) (string, error) {
	resource := struct {
		ResourceType string `json:"resourceType"`
	}{}

	if err := json.Unmarshal(r, &resource); err != nil {
		return "", fmt.Errorf("failed to unmarshal resource: %w", err)
	}
	if resource.ResourceType == "" {
		return "", fmt.Errorf("resourceType is empty in resource")
	}
	return resource.ResourceType, nil
}
