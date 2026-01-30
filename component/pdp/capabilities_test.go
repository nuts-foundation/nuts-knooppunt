package pdp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_enrichPolicyInputWithCapabilityStatement(t *testing.T) {
	t.Run("reject interaction", func(t *testing.T) {
		input := PolicyInput{
			Subject: Subject{
				Properties: SubjectProperties{
					ClientQualifications:  []string{"mcsd_update"},
					SubjectOrganizationId: "00000666",
				},
			},
			Resource: PolicyResource{
				Type: fhir.ResourceTypeOrganization,
				Properties: PolicyResourceProperties{
					ResourceId: "118876",
				},
			},
			Action: PolicyAction{
				Properties: PolicyActionProperties{
					ContentType:     "application/fhir+json",
					InteractionType: fhir.TypeRestfulInteractionUpdate,
				},
			},
			Context: PolicyContext{
				DataHolderOrganizationId: "00000659",
			},
		}

		inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_update")
		assert.NotEmpty(t, resp)
		assert.False(t, inp.Context.FHIRCapabilityChecked)
	})
	t.Run("allow interaction", func(t *testing.T) {
		input := PolicyInput{
			Subject: Subject{
				Properties: SubjectProperties{
					ClientQualifications:  []string{"mcsd_update"},
					SubjectOrganizationId: "00000666",
				},
			},
			Resource: PolicyResource{
				Type: fhir.ResourceTypeOrganization,
				Properties: PolicyResourceProperties{
					ResourceId: "118876",
				},
			},
			Action: PolicyAction{
				Properties: PolicyActionProperties{
					ContentType:     "application/fhir+json",
					InteractionType: fhir.TypeRestfulInteractionHistoryType,
				},
			},
			Context: PolicyContext{
				DataHolderOrganizationId: "00000659",
			},
		}

		inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_update")
		assert.Empty(t, resp)
		assert.True(t, inp.Context.FHIRCapabilityChecked)
	})
	t.Run("allow search param", func(t *testing.T) {
		input := PolicyInput{
			Subject: Subject{
				Properties: SubjectProperties{
					ClientQualifications:  []string{"mcsd_update"},
					SubjectOrganizationId: "00000666",
				},
			},
			Resource: PolicyResource{
				Type: fhir.ResourceTypeOrganization,
				Properties: PolicyResourceProperties{
					ResourceId: "118876",
				},
			},
			Action: PolicyAction{
				Properties: PolicyActionProperties{
					ContentType:     "application/fhir+json",
					InteractionType: fhir.TypeRestfulInteractionSearchType,
					SearchParams:    map[string]string{"_since": "2024-01-01"},
				},
			},
			Context: PolicyContext{
				DataHolderOrganizationId: "00000659",
			},
		}

		inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_update")
		assert.Empty(t, resp)
		assert.True(t, inp.Context.FHIRCapabilityChecked)
	})
	t.Run("reject search param", func(t *testing.T) {
		input := PolicyInput{
			Subject: Subject{
				Properties: SubjectProperties{
					ClientQualifications:  []string{"mcsd_update"},
					SubjectOrganizationId: "00000666",
				},
			},
			Resource: PolicyResource{
				Type: fhir.ResourceTypeOrganization,
				Properties: PolicyResourceProperties{
					ResourceId: "118876",
				},
			},
			Action: PolicyAction{
				Properties: PolicyActionProperties{
					ContentType:     "application/fhir+json",
					InteractionType: fhir.TypeRestfulInteractionSearchType,
					SearchParams:    map[string]string{"_foo": "bar", "_since": "2024-01-01"},
				},
			},
			Context: PolicyContext{
				DataHolderOrganizationId: "00000659",
			},
		}

		inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_update")
		assert.NotEmpty(t, resp)
		assert.False(t, inp.Context.FHIRCapabilityChecked)
	})
	t.Run("reject interaction type", func(t *testing.T) {
		input := PolicyInput{
			Subject: Subject{
				Properties: SubjectProperties{
					ClientQualifications:  []string{"mcsd_update"},
					SubjectOrganizationId: "00000666",
				},
			},
			Action: PolicyAction{
				Properties: PolicyActionProperties{
					ContentType:     "application/fhir+json",
					InteractionType: fhir.TypeRestfulInteractionSearchSystem,
				},
			},
			Context: PolicyContext{
				DataHolderOrganizationId: "00000659",
			},
		}

		inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_update")
		assert.NotEmpty(t, resp)
		assert.False(t, inp.Context.FHIRCapabilityChecked)
	})
	t.Run("allow include", func(t *testing.T) {
		input := PolicyInput{
			Subject: Subject{
				Properties: SubjectProperties{
					ClientQualifications:  []string{"mcsd_query"},
					SubjectOrganizationId: "00000666",
				},
			},
			Resource: PolicyResource{
				Type: fhir.ResourceTypeLocation,
				Properties: PolicyResourceProperties{
					ResourceId: "88716123",
				},
			},
			Action: PolicyAction{
				Properties: PolicyActionProperties{
					ContentType:     "application/fhir+json",
					InteractionType: fhir.TypeRestfulInteractionRead,
					Include:         []string{"Location:organization"},
				},
			},
			Context: PolicyContext{
				DataHolderOrganizationId: "00000659",
			},
		}

		inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_query")
		assert.Empty(t, resp)
		assert.True(t, inp.Context.FHIRCapabilityChecked)
	})
	t.Run("reject include", func(t *testing.T) {
		input := PolicyInput{
			Subject: Subject{
				Properties: SubjectProperties{
					ClientQualifications:  []string{"mcsd_query"},
					SubjectOrganizationId: "00000666",
				},
			},
			Resource: PolicyResource{
				Type: fhir.ResourceTypeEndpoint,
				Properties: PolicyResourceProperties{
					ResourceId: "88716123",
				},
			},
			Action: PolicyAction{
				Properties: PolicyActionProperties{
					ContentType:     "application/fhir+json",
					InteractionType: fhir.TypeRestfulInteractionRead,
					Include:         []string{"Endpoint:organization"},
				},
			},
			Context: PolicyContext{
				DataHolderOrganizationId: "00000659",
			},
		}

		inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_query")
		assert.NotEmpty(t, resp)
		assert.False(t, inp.Context.FHIRCapabilityChecked)
	})
	t.Run("reject revinclude", func(t *testing.T) {
		input := PolicyInput{
			Subject: Subject{
				Properties: SubjectProperties{
					ClientQualifications:  []string{"mcsd_query"},
					SubjectOrganizationId: "00000666",
				},
			},
			Resource: PolicyResource{
				Type: fhir.ResourceTypePractitioner,
				Properties: PolicyResourceProperties{
					ResourceId: "88716123",
				},
			},
			Action: PolicyAction{
				Properties: PolicyActionProperties{
					ContentType:     "application/fhir+json",
					InteractionType: fhir.TypeRestfulInteractionRead,
					Include:         []string{"Location:organization"},
				},
			},
			Context: PolicyContext{
				DataHolderOrganizationId: "00000659",
			},
		}

		inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_query")
		assert.NotEmpty(t, resp)
		assert.False(t, inp.Context.FHIRCapabilityChecked)
	})
	t.Run("allow revinclude", func(t *testing.T) {
		input := PolicyInput{
			Subject: Subject{
				Properties: SubjectProperties{
					ClientQualifications:  []string{"mcsd_query"},
					SubjectOrganizationId: "00000666",
				},
			},
			Resource: PolicyResource{
				Type: fhir.ResourceTypeOrganization,
				Properties: PolicyResourceProperties{
					ResourceId: "88716123",
				},
			},
			Action: PolicyAction{
				Properties: PolicyActionProperties{
					ContentType:     "application/fhir+json",
					InteractionType: fhir.TypeRestfulInteractionRead,
					Revinclude:      []string{"Location:organization"},
				},
			},
			Context: PolicyContext{
				DataHolderOrganizationId: "00000659",
			},
		}

		inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_query")
		assert.Empty(t, resp)
		assert.True(t, inp.Context.FHIRCapabilityChecked)
	})
}
