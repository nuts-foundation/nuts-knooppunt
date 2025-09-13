package fhir

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