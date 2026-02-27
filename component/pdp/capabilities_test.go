package pdp

import (
	"context"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_reject_interaction(t *testing.T) {
	input := PolicyInput{
		Subject: PolicySubject{
			Client: PolicySubjectClient{
				Scopes: []string{"mcsd_query"},
			},
			Organization: PolicySubjectOrganization{
				Ura: "00000666",
			},
		},
		Resource: PolicyResource{
			Type: to.Ptr(fhir.ResourceTypeOrganization),
			Properties: PolicyResourceProperties{
				ResourceId: "118876",
			},
		},
		Action: PolicyAction{
			ConnectionTypeCode: "hl7-fhir-rest",
			FHIRRest: FHIRRestData{
				InteractionType: fhir.TypeRestfulInteractionUpdate,
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_update")
	assert.NotEmpty(t, resp)
	assert.False(t, inp.Action.FHIRRest.CapabilityChecked)
}

func TestComponent_allow_interaction(t *testing.T) {
	input := PolicyInput{
		Subject: PolicySubject{
			Client: PolicySubjectClient{
				Scopes: []string{"mcsd_query"},
			},
			Organization: PolicySubjectOrganization{
				Ura: "00000666",
			},
		},
		Resource: PolicyResource{
			Type: to.Ptr(fhir.ResourceTypeOrganization),
			Properties: PolicyResourceProperties{
				ResourceId: "118876",
			},
		},
		Action: PolicyAction{
			ConnectionTypeCode: "hl7-fhir-rest",
			FHIRRest: FHIRRestData{
				InteractionType: fhir.TypeRestfulInteractionHistoryType,
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_update")
	assert.Empty(t, resp)
	assert.True(t, inp.Action.FHIRRest.CapabilityChecked)
}

func TestComponent_allow_search_param(t *testing.T) {
	input := PolicyInput{
		Subject: PolicySubject{
			Client: PolicySubjectClient{
				Scopes: []string{"mcsd_query"},
			},
			Organization: PolicySubjectOrganization{
				Ura: "00000666",
			},
		},
		Resource: PolicyResource{
			Type: to.Ptr(fhir.ResourceTypeOrganization),
			Properties: PolicyResourceProperties{
				ResourceId: "118876",
			},
		},
		Action: PolicyAction{
			ConnectionTypeCode: "hl7-fhir-rest",
			FHIRRest: FHIRRestData{
				InteractionType: fhir.TypeRestfulInteractionSearchType,
				SearchParams:    map[string][]string{"_since": []string{"2024-01-01"}},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_update")
	assert.Empty(t, resp)
	assert.True(t, inp.Action.FHIRRest.CapabilityChecked)
}

func TestComponent_reject_search_param(t *testing.T) {
	input := PolicyInput{
		Subject: PolicySubject{
			Client: PolicySubjectClient{
				Scopes: []string{"mcsd_query"},
			},
			Organization: PolicySubjectOrganization{
				Ura: "00000666",
			},
		},
		Resource: PolicyResource{
			Type: to.Ptr(fhir.ResourceTypeOrganization),
			Properties: PolicyResourceProperties{
				ResourceId: "118876",
			},
		},
		Action: PolicyAction{
			ConnectionTypeCode: "hl7-fhir-rest",
			FHIRRest: FHIRRestData{
				InteractionType: fhir.TypeRestfulInteractionSearchType,
				SearchParams:    map[string][]string{"_foo": []string{"bar"}, "_since": {"2024-01-01"}},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_update")
	assert.NotEmpty(t, resp)
	assert.False(t, inp.Action.FHIRRest.CapabilityChecked)
}

func TestComponent_reject_interaction_type(t *testing.T) {
	input := PolicyInput{
		Subject: PolicySubject{
			Client: PolicySubjectClient{
				Scopes: []string{"mcsd_query"},
			},
			Organization: PolicySubjectOrganization{
				Ura: "00000666",
			},
		},
		Action: PolicyAction{
			ConnectionTypeCode: "hl7-fhir-rest",
			FHIRRest: FHIRRestData{
				InteractionType: fhir.TypeRestfulInteractionSearchSystem,
				SearchParams:    map[string][]string{"_foo": []string{"bar"}, "_since": []string{"2024-01-01"}},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := enrichPolicyInputWithCapabilityStatement(context.Background(), input, "mcsd_update")
	assert.Empty(t, resp)
	assert.False(t, inp.Action.FHIRRest.CapabilityChecked)
}

func TestComponent_allow_include(t *testing.T) {
	input := PolicyInput{
		Subject: PolicySubject{
			Client: PolicySubjectClient{
				Scopes: []string{"mcsd_query"},
			},
			Organization: PolicySubjectOrganization{
				Ura: "00000666",
			},
		},
		Resource: PolicyResource{
			Type: to.Ptr(fhir.ResourceTypeLocation),
			Properties: PolicyResourceProperties{
				ResourceId: "88716123",
			},
		},
		Action: PolicyAction{
			ConnectionTypeCode: "hl7-fhir-rest",
			FHIRRest: FHIRRestData{
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
	assert.True(t, inp.Action.FHIRRest.CapabilityChecked)
}

func TestComponent_reject_include(t *testing.T) {
	input := PolicyInput{
		Subject: PolicySubject{
			Client: PolicySubjectClient{
				Scopes: []string{"mcsd_query"},
			},
			Organization: PolicySubjectOrganization{
				Ura: "00000666",
			},
		},
		Resource: PolicyResource{
			Type: to.Ptr(fhir.ResourceTypeEndpoint),
			Properties: PolicyResourceProperties{
				ResourceId: "88716123",
			},
		},
		Action: PolicyAction{
			ConnectionTypeCode: "hl7-fhir-rest",
			FHIRRest: FHIRRestData{
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
	assert.False(t, inp.Action.FHIRRest.CapabilityChecked)
}

func TestComponent_reject_revinclude(t *testing.T) {
	input := PolicyInput{
		Subject: PolicySubject{
			Client: PolicySubjectClient{
				Scopes: []string{"mcsd_query"},
			},
			Organization: PolicySubjectOrganization{
				Ura: "00000666",
			},
		},
		Resource: PolicyResource{
			Type: to.Ptr(fhir.ResourceTypePractitioner),
			Properties: PolicyResourceProperties{
				ResourceId: "88716123",
			},
		},
		Action: PolicyAction{
			ConnectionTypeCode: "hl7-fhir-rest",
			FHIRRest: FHIRRestData{
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
	assert.False(t, inp.Action.FHIRRest.CapabilityChecked)
}

func TestComponent_allow_revinclude(t *testing.T) {
	input := PolicyInput{
		Subject: PolicySubject{
			Client: PolicySubjectClient{
				Scopes: []string{"mcsd_query"},
			},
			Organization: PolicySubjectOrganization{
				Ura: "00000666",
			},
		},
		Resource: PolicyResource{
			Type: to.Ptr(fhir.ResourceTypeOrganization),
			Properties: PolicyResourceProperties{
				ResourceId: "88716123",
			},
		},
		Action: PolicyAction{
			ConnectionTypeCode: "hl7-fhir-rest",
			FHIRRest: FHIRRestData{
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
	assert.True(t, inp.Action.FHIRRest.CapabilityChecked)
}
