package pdp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_reject_interaction(t *testing.T) {
	input = PolicyInput{
		Scope:           "mcsd_update",
		InteractionType: fhir.TypeRestfulInteractionUpdate,
		ResourceId:      "118876",
		ResourceType:    fhir.ResourceTypeOrganization,
		RequestingUra:   "00000666",
		DataHolderUra:   "00000659",
	}

	resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
}

func TestComponent_allow_interaction(t *testing.T) {
	input := PolicyInput{
		Scope:                     "mcsd_update",
		InteractionType:           fhir.TypeRestfulInteractionHistoryType,
		ResourceId:                "118876",
		ResourceType:              fhir.ResourceTypeOrganization,
		RequestingOrganizationUra: "00000666",
		DataHolderOrganizationUra: "00000659",
	}

	resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
}

func TestComponent_allow_search_param(t *testing.T) {
	input := PolicyInput{
		Scope:                     "mcsd_update",
		InteractionType:           fhir.TypeRestfulInteractionSearchType,
		SearchParams:              []string{"_since"},
		ResourceId:                "118876",
		ResourceType:              fhir.ResourceTypeOrganization,
		RequestingOrganizationUra: "00000666",
		DataHolderOrganizationUra: "00000659",
	}

	resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
}

func TestComponent_reject_search_param(t *testing.T) {
	input := PolicyInput{
		Scope:                     "mcsd_update",
		InteractionType:           fhir.TypeRestfulInteractionSearchType,
		SearchParams:              []string{"_foo", "_since"},
		ResourceId:                "118876",
		ResourceType:              fhir.ResourceTypeOrganization,
		RequestingOrganizationUra: "00000666",
		DataHolderOrganizationUra: "00000659",
	}

	resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
}

func TestComponent_reject_interaction_type(t *testing.T) {
	input := PolicyInput{
		Scope:                     "mcsd_update",
		InteractionType:           fhir.TypeRestfulInteractionSearchSystem,
		RequestingOrganizationUra: "00000666",
		DataHolderOrganizationUra: "00000659",
	}

	resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
}

func TestComponent_allow_include(t *testing.T) {
	input := PolicyInput{
		Scope:                     "mcsd_query",
		InteractionType:           fhir.TypeRestfulInteractionRead,
		ResourceId:                "88716123",
		ResourceType:              fhir.ResourceTypeLocation,
		Include:                   []string{"Location:organization"},
		RequestingOrganizationUra: "00000666",
		DataHolderOrganizationUra: "00000659",
	}

	resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
}

func TestComponent_reject_include(t *testing.T) {
	input := PolicyInput{
		Scope:                     "mcsd_query",
		InteractionType:           fhir.TypeRestfulInteractionRead,
		ResourceId:                "88716123",
		ResourceType:              fhir.ResourceTypeEndpoint,
		Include:                   []string{"Endpoint:organization"},
		RequestingOrganizationUra: "00000666",
		DataHolderOrganizationUra: "00000659",
	}

	resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
}

func TestComponent_reject_revinclude(t *testing.T) {
	input := PolicyInput{
		Scope:                     "mcsd_query",
		InteractionType:           fhir.TypeRestfulInteractionRead,
		ResourceId:                "88716123",
		ResourceType:              fhir.ResourceTypePractitioner,
		Revinclude:                []string{"Location:organization"},
		RequestingOrganizationUra: "00000666",
		DataHolderOrganizationUra: "00000659",
	}

	resp := evalCapabilityPolicy(context.Background(), input)
	assert.False(t, resp.Allow)
}

func TestComponent_allow_revinclude(t *testing.T) {
	input := PolicyInput{
		Scope:                     "mcsd_query",
		InteractionType:           fhir.TypeRestfulInteractionRead,
		ResourceId:                "88716123",
		ResourceType:              fhir.ResourceTypeOrganization,
		Revinclude:                []string{"Location:organization"},
		RequestingOrganizationUra: "00000666",
		DataHolderOrganizationUra: "00000659",
	}

	resp := evalCapabilityPolicy(context.Background(), input)
	assert.True(t, resp.Allow)
}
