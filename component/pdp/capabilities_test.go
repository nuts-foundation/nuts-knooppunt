package pdp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_reject_interaction(t *testing.T) {
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

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
	assert.False(t, inp.Context.FHIRCapabilityChecked)
}

func TestComponent_allow_interaction(t *testing.T) {
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

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
	assert.True(t, inp.Context.FHIRCapabilityChecked)
}

func TestComponent_allow_search_param(t *testing.T) {
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

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
	assert.True(t, inp.Context.FHIRCapabilityChecked)
}

func TestComponent_reject_search_param(t *testing.T) {
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

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
	assert.False(t, inp.Context.FHIRCapabilityChecked)
}

func TestComponent_reject_interaction_type(t *testing.T) {
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

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
	assert.False(t, inp.Context.FHIRCapabilityChecked)
}

func TestComponent_allow_include(t *testing.T) {
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

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
	assert.True(t, inp.Context.FHIRCapabilityChecked)
}

func TestComponent_reject_include(t *testing.T) {
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

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
	assert.False(t, inp.Context.FHIRCapabilityChecked)
}

func TestComponent_reject_revinclude(t *testing.T) {
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

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
	assert.False(t, inp.Context.FHIRCapabilityChecked)
}

func TestComponent_allow_revinclude(t *testing.T) {
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

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
	assert.True(t, inp.Context.FHIRCapabilityChecked)
}
