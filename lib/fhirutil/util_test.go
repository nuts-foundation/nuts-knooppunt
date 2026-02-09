package fhirutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExtractResourceInfo(t *testing.T) {
	tests := []struct {
		name               string
		resourceJSON       []byte
		expectedID         string
		expectedType       string
		expectedLastUpdate *time.Time
		expectError        bool
	}{
		{
			name:               "complete resource with all fields",
			resourceJSON:       []byte(`{"id":"123","resourceType":"Organization","meta":{"lastUpdated":"2025-08-01T10:00:00.000+00:00"}}`),
			expectedID:         "123",
			expectedType:       "Organization",
			expectedLastUpdate: parseTime("2025-08-01T10:00:00.000+00:00"),
		},
		{
			name:               "resource without meta",
			resourceJSON:       []byte(`{"id":"456","resourceType":"Endpoint"}`),
			expectedID:         "456",
			expectedType:       "Endpoint",
			expectedLastUpdate: nil,
		},
		{
			name:               "resource without ID",
			resourceJSON:       []byte(`{"resourceType":"Location","meta":{"lastUpdated":"2025-08-01T12:00:00.000+00:00"}}`),
			expectedID:         "",
			expectedType:       "Location",
			expectedLastUpdate: parseTime("2025-08-01T12:00:00.000+00:00"),
		},
		{
			name:               "invalid timestamp format",
			resourceJSON:       []byte(`{"id":"789","resourceType":"HealthcareService","meta":{"lastUpdated":"invalid-date"}}`),
			expectedID:         "789",
			expectedType:       "HealthcareService",
			expectedLastUpdate: nil,
		},
		{
			name:         "invalid JSON",
			resourceJSON: []byte(`{invalid json}`),
			expectError:  true,
		},
		{
			name:               "empty resource",
			resourceJSON:       []byte(`{}`),
			expectedID:         "",
			expectedType:       "",
			expectedLastUpdate: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractResourceInfo(tt.resourceJSON)
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectedID, result.ID)
			require.Equal(t, tt.expectedType, result.ResourceType)

			if tt.expectedLastUpdate == nil {
				require.Nil(t, result.LastUpdated)
			} else {
				require.NotNil(t, result.LastUpdated)
				require.Equal(t, *tt.expectedLastUpdate, *result.LastUpdated)
			}
		})
	}
}

// Helper function to parse time for test expectations
func parseTime(timeStr string) *time.Time {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestBuildSourceURL(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		parts    []string
		expected string
	}{
		{
			name:     "simple resource reference",
			baseURL:  "https://example.com/fhir",
			parts:    []string{"Organization/123"},
			expected: "https://example.com/fhir/Organization/123",
		},
		{
			name:     "base URL with trailing slash",
			baseURL:  "https://example.com/fhir/",
			parts:    []string{"Organization/123"},
			expected: "https://example.com/fhir/Organization/123",
		},
		{
			name:     "separate resource type and ID",
			baseURL:  "https://example.com/fhir",
			parts:    []string{"Patient", "456"},
			expected: "https://example.com/fhir/Patient/456",
		},
		{
			name:     "multiple parts",
			baseURL:  "https://example.com/fhir",
			parts:    []string{"Organization", "123", "_history", "2"},
			expected: "https://example.com/fhir/Organization/123/_history/2",
		},
		{
			name:     "empty parts are skipped",
			baseURL:  "https://example.com/fhir",
			parts:    []string{"Organization", "", "123"},
			expected: "https://example.com/fhir/Organization/123",
		},
		{
			name:     "no parts",
			baseURL:  "https://example.com/fhir",
			parts:    []string{},
			expected: "https://example.com/fhir",
		},
		{
			name:     "base URL without path",
			baseURL:  "https://example.com",
			parts:    []string{"fhir", "Organization", "123"},
			expected: "https://example.com/fhir/Organization/123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := BuildSourceURL(tt.baseURL, tt.parts...)
			require.NoError(t, err)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestReferencesType(t *testing.T) {
	tests := []struct {
		name         string
		ref          string
		resourceType string
		expected     bool
	}{
		{
			name:         "valid reference",
			ref:          "Organization/123",
			resourceType: "Organization",
			expected:     true,
		},
		{
			name:         "valid reference with UUID",
			ref:          "Patient/550e8400-e29b-41d4-a716-446655440000",
			resourceType: "Patient",
			expected:     true,
		},
		{
			name:         "wrong resource type",
			ref:          "Organization/123",
			resourceType: "Patient",
			expected:     false,
		},
		{
			name:         "empty reference",
			ref:          "",
			resourceType: "Organization",
			expected:     false,
		},
		{
			name:         "reference without ID",
			ref:          "Organization/",
			resourceType: "Organization",
			expected:     false,
		},
		{
			name:         "type only without slash",
			ref:          "Organization",
			resourceType: "Organization",
			expected:     false,
		},
		{
			name:         "partial type match",
			ref:          "Org/123",
			resourceType: "Organization",
			expected:     false,
		},
		{
			name:         "type is prefix of reference type",
			ref:          "OrganizationAffiliation/123",
			resourceType: "Organization",
			expected:     false,
		},
		{
			name:         "case sensitive mismatch",
			ref:          "organization/123",
			resourceType: "Organization",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReferencesType(tt.ref, tt.resourceType)
			require.Equal(t, tt.expected, result)
		})
	}
}
