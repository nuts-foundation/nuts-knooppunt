package fhirutil

import (
	"encoding/json"
	"fmt"
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
