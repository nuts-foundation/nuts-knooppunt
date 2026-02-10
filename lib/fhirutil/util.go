package fhirutil

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func FilterIdentifiersBySystem(identifiers []fhir.Identifier, system string) []fhir.Identifier {
	var filtered []fhir.Identifier
	for _, id := range identifiers {
		if (id.System == nil && system == "") || (id.System != nil && *id.System == system) {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

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

// TokenToIdentifier converts a FHIR search token ("system|value") to a FHIR Identifier.
// If the token is empty or not in the correct format, an error is returned.
func TokenToIdentifier(token string) (*fhir.Identifier, error) {
	if token == "" {
		return nil, errors.New("empty token")
	}
	parts := strings.Split(token, "|")
	if len(parts) != 2 {
		return nil, errors.New("invalid token format")
	}
	result := fhir.Identifier{}
	if parts[0] == "" && parts[1] == "" {
		return nil, errors.New("invalid token: both system and value are empty")
	}
	if parts[0] != "" {
		result.System = &parts[0]
	}
	if parts[1] != "" {
		result.Value = &parts[1]
	}
	return &result, nil
}

func ReferencesType(ref string, resourceType string) bool {
	if !localLiteralReferencePattern.MatchString(ref) {
		// not allowed
		return false
	}
	return strings.HasPrefix(ref, resourceType+"/")
}

func IDFromReference(ref string, resourceType string) string {
	if !ReferencesType(ref, resourceType) {
		return ""
	}
	return strings.TrimPrefix(ref, resourceType+"/")
}

var localLiteralReferencePattern = regexp.MustCompile(`^[a-zA-Z]+/[A-Za-z0-9\-.]{1,64}$`)
