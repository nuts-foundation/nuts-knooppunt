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
		Subject: Subject{
			Properties: SubjectProperties{
				ClientQualifications:  []string{"mcsd_update"},
				SubjectOrganizationId: "00000666",
			},
		},
		Resource: PolicyResource{
			Type: to.Ptr(fhir.ResourceTypeOrganization),
			Properties: PolicyResourceProperties{
				ResourceId: "118876",
			},
		},
		Action: PolicyAction{
			Properties: PolicyActionProperties{
				ConnectionData: PolicyConnectionData{
					FHIRRest: FhirConnectionData{
						isFHIRRest:      true,
						InteractionType: fhir.TypeRestfulInteractionUpdate,
					},
				},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
	assert.False(t, inp.Action.Properties.ConnectionData.FHIRRest.CapabilityChecked)
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
			Type: to.Ptr(fhir.ResourceTypeOrganization),
			Properties: PolicyResourceProperties{
				ResourceId: "118876",
			},
		},
		Action: PolicyAction{
			Properties: PolicyActionProperties{
				ConnectionData: PolicyConnectionData{
					FHIRRest: FhirConnectionData{
						isFHIRRest:      true,
						InteractionType: fhir.TypeRestfulInteractionHistoryType,
					},
				},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
	assert.True(t, inp.Action.Properties.ConnectionData.FHIRRest.CapabilityChecked)
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
			Type: to.Ptr(fhir.ResourceTypeOrganization),
			Properties: PolicyResourceProperties{
				ResourceId: "118876",
			},
		},
		Action: PolicyAction{
			Properties: PolicyActionProperties{
				ConnectionData: PolicyConnectionData{
					FHIRRest: FhirConnectionData{
						isFHIRRest:      true,
						InteractionType: fhir.TypeRestfulInteractionSearchType,
						SearchParams:    map[string]string{"_since": "2024-01-01"},
					},
				},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
	assert.True(t, inp.Action.Properties.ConnectionData.FHIRRest.CapabilityChecked)
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
			Type: to.Ptr(fhir.ResourceTypeOrganization),
			Properties: PolicyResourceProperties{
				ResourceId: "118876",
			},
		},
		Action: PolicyAction{
			Properties: PolicyActionProperties{
				ConnectionData: PolicyConnectionData{
					FHIRRest: FhirConnectionData{
						isFHIRRest:      true,
						InteractionType: fhir.TypeRestfulInteractionSearchType,
						SearchParams:    map[string]string{"_foo": "bar", "_since": "2024-01-01"},
					},
				},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
	assert.False(t, inp.Action.Properties.ConnectionData.FHIRRest.CapabilityChecked)
}

func TestComponent_reject_interaction_type(t *testing.T) {
	input := PolicyInput{
		Subject: Subject{
			Properties: SubjectProperties{
				ClientQualifications:  []string{"mcsd_update"},
				SubjectOrganizationId: "00000666",
			},
		},
		Resource: PolicyResource{
			Type: to.Ptr(fhir.ResourceTypeOrganization),
		},
		Action: PolicyAction{
			Properties: PolicyActionProperties{
				ConnectionData: PolicyConnectionData{
					FHIRRest: FhirConnectionData{
						isFHIRRest:      true,
						InteractionType: fhir.TypeRestfulInteractionSearchSystem,
					},
				},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
	assert.False(t, inp.Action.Properties.ConnectionData.FHIRRest.CapabilityChecked)
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
			Type: to.Ptr(fhir.ResourceTypeLocation),
			Properties: PolicyResourceProperties{
				ResourceId: "88716123",
			},
		},
		Action: PolicyAction{
			Properties: PolicyActionProperties{
				ConnectionData: PolicyConnectionData{
					FHIRRest: FhirConnectionData{
						isFHIRRest:      true,
						InteractionType: fhir.TypeRestfulInteractionRead,
						Include:         []string{"Location:organization"},
					},
				},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
	assert.True(t, inp.Action.Properties.ConnectionData.FHIRRest.CapabilityChecked)
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
			Type: to.Ptr(fhir.ResourceTypeEndpoint),
			Properties: PolicyResourceProperties{
				ResourceId: "88716123",
			},
		},
		Action: PolicyAction{
			Properties: PolicyActionProperties{
				ConnectionData: PolicyConnectionData{
					FHIRRest: FhirConnectionData{
						isFHIRRest:      true,
						InteractionType: fhir.TypeRestfulInteractionRead,
						Include:         []string{"Endpoint:organization"},
					},
				},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
	assert.False(t, inp.Action.Properties.ConnectionData.FHIRRest.CapabilityChecked)
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
			Type: to.Ptr(fhir.ResourceTypePractitioner),
			Properties: PolicyResourceProperties{
				ResourceId: "88716123",
			},
		},
		Action: PolicyAction{
			Properties: PolicyActionProperties{
				ConnectionData: PolicyConnectionData{
					FHIRRest: FhirConnectionData{
						isFHIRRest:      true,
						InteractionType: fhir.TypeRestfulInteractionRead,
						Include:         []string{"Location:organization"},
					},
				},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
	assert.False(t, inp.Action.Properties.ConnectionData.FHIRRest.CapabilityChecked)
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
			Type: to.Ptr(fhir.ResourceTypeOrganization),
			Properties: PolicyResourceProperties{
				ResourceId: "88716123",
			},
		},
		Action: PolicyAction{
			Properties: PolicyActionProperties{
				ConnectionData: PolicyConnectionData{
					FHIRRest: FhirConnectionData{
						isFHIRRest:      true,
						InteractionType: fhir.TypeRestfulInteractionRead,
						Revinclude:      []string{"Location:organization"},
					},
				},
			},
		},
		Context: PolicyContext{
			DataHolderOrganizationId: "00000659",
		},
	}

	inp, resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
	assert.True(t, inp.Action.Properties.ConnectionData.FHIRRest.CapabilityChecked)
}
