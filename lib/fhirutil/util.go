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
	Resource     map[string]any
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

	// Add the complete resource unmarsheld for later use
	info.Resource = resource

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

// TypeAndIDFromReference extracts the resource type and logical id from a FHIR reference or URL,
// whether relative ("Organization/123"), absolute ("http://host/fhir/Organization/123"), or a
// versioned history URL ("Organization/123/_history/2"). ok is false when no type/id can be
// determined.
//
// Unlike IDFromReference/ReferencesType - which strictly validate a local literal reference to a
// known type - this is a lenient extractor for references whose form is not known up front.
func TypeAndIDFromReference(ref string) (resourceType, id string, ok bool) {
	// Drop any "/_history/{version}" suffix so a versioned URL resolves to the resource identity
	// rather than to the "_history/{version}" segments.
	if i := strings.Index(ref, "/_history/"); i != -1 {
		ref = ref[:i]
	}
	parts := strings.Split(strings.Trim(ref, "/"), "/")
	if len(parts) < 2 {
		return "", "", false
	}
	resourceType = parts[len(parts)-2]
	id = parts[len(parts)-1]
	if resourceType == "" || id == "" {
		return "", "", false
	}
	return resourceType, id, true
}

var localLiteralReferencePattern = regexp.MustCompile(`^[a-zA-Z]+/[A-Za-z0-9\-.]{1,64}$`)
