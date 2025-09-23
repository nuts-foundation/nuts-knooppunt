package fhirutil

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// ResourceInfo contains common FHIR resource fields extracted from JSON
type ResourceInfo struct {
	ID           string
	ResourceType string
	LastUpdated  *time.Time
}

// ExtractResourceInfo extracts common FHIR resource fields from JSON bytes.
// This is a generic utility for parsing any FHIR resource to get basic metadata
// without requiring knowledge of the specific resource type.
func ExtractResourceInfo(resourceJSON []byte) (*ResourceInfo, error) {
	var resource map[string]any
	if err := json.Unmarshal(resourceJSON, &resource); err != nil {
		return nil, fmt.Errorf("failed to unmarshal resource: %w", err)
	}

	info := &ResourceInfo{}

	// Extract resource type
	if resourceType, ok := resource["resourceType"].(string); ok {
		info.ResourceType = resourceType
	}

	// Extract ID
	if id, ok := resource["id"].(string); ok {
		info.ID = id
	}

	// Extract meta.lastUpdated
	if meta, ok := resource["meta"].(map[string]any); ok {
		if lastUpdatedStr, ok := meta["lastUpdated"].(string); ok {
			if t, err := time.Parse(time.RFC3339, lastUpdatedStr); err == nil {
				info.LastUpdated = &t
			}
		}
	}

	return info, nil
}

// BuildSourceURL constructs a consistent FHIR source URL from a base URL and resource reference.
// This ensures all _source URL construction follows the same pattern throughout the application.
// Examples:
//   - BuildSourceURL("https://example.com/fhir", "Organization/123") -> "https://example.com/fhir/Organization/123"
//   - BuildSourceURL("https://example.com/fhir/", "Patient", "456") -> "https://example.com/fhir/Patient/456"
func BuildSourceURL(baseURL string, parts ...string) (string, error) {
	return url.JoinPath(baseURL, parts...)
}
