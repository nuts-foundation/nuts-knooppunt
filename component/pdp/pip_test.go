package pdp

import (
	"context"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/test"
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestEnrichResourceContent(t *testing.T) {
	t.Run("fetches resource content via full PIP pipeline", func(t *testing.T) {
		task := fhir.Task{
			Id:     to.Ptr("task-1"),
			Status: fhir.TaskStatusRequested,
			Owner: &fhir.Reference{
				Identifier: &fhir.Identifier{
					System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
					Value:  to.Ptr("00000001"),
				},
			},
		}

		component := &Component{
			pipClient: &test.StubFHIRClient{
				Resources: []any{task},
			},
		}

		resourceType := fhir.ResourceTypeTask
		input := &PolicyInput{
			Resource: PolicyResource{
				Type: &resourceType,
				Id:   "task-1",
			},
		}

		result, _ := component.enrichPolicyInputWithPIP(context.Background(), input)

		require.NotNil(t, result.Resource.Content, "resource content should be populated after PIP enrichment")
		assert.Equal(t, "task-1", result.Resource.Content["id"])
		assert.Equal(t, "requested", result.Resource.Content["status"])
		assert.Equal(t, "Task", result.Resource.Content["resourceType"])

		owner, ok := result.Resource.Content["owner"].(map[string]any)
		require.True(t, ok)
		identifier, ok := owner["identifier"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "http://fhir.nl/fhir/NamingSystem/ura", identifier["system"])
		assert.Equal(t, "00000001", identifier["value"])
	})

	t.Run("skips when no resource type", func(t *testing.T) {
		component := &Component{
			pipClient: &test.StubFHIRClient{},
		}
		input := &PolicyInput{
			Resource: PolicyResource{Id: "task-1"},
		}

		result, reasons := component.enrichResourceContent(context.Background(), input)
		assert.Empty(t, reasons)
		assert.Nil(t, result.Resource.Content)
	})

	t.Run("skips when no resource id", func(t *testing.T) {
		component := &Component{
			pipClient: &test.StubFHIRClient{},
		}
		resourceType := fhir.ResourceTypeTask
		input := &PolicyInput{
			Resource: PolicyResource{Type: &resourceType},
		}

		result, reasons := component.enrichResourceContent(context.Background(), input)
		assert.Empty(t, reasons)
		assert.Nil(t, result.Resource.Content)
	})

	t.Run("returns pip_error when resource not found", func(t *testing.T) {
		component := &Component{
			pipClient: &test.StubFHIRClient{},
		}
		resourceType := fhir.ResourceTypeTask
		input := &PolicyInput{
			Resource: PolicyResource{
				Type: &resourceType,
				Id:   "nonexistent",
			},
		}

		result, reasons := component.enrichResourceContent(context.Background(), input)
		require.Len(t, reasons, 1)
		assert.Equal(t, TypeResultCodePIPError, reasons[0].Code)
		assert.Nil(t, result.Resource.Content)
	})
}
