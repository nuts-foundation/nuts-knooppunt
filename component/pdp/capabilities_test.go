package pdp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_disallow_interaction(t *testing.T) {
	input := MainPolicyInput{
		Scope:                     "mcsd_update",
		InteractionType:           fhir.TypeRestfulInteractionRead,
		ResourceId:                "118876",
		ResourceType:              fhir.ResourceTypeOrganization,
		RequestingOrganizationUra: "00000666",
		DataHolderOrganizationUra: "00000659",
	}

	resp := EvalCapabilityPolicy(input)
	assert.False(t, resp.Allow)
}

func TestComponent_allow_interaction(t *testing.T) {
	input := MainPolicyInput{
		Scope:                     "mcsd_update",
		InteractionType:           fhir.TypeRestfulInteractionHistoryType,
		ResourceId:                "118876",
		ResourceType:              fhir.ResourceTypeOrganization,
		RequestingOrganizationUra: "00000666",
		DataHolderOrganizationUra: "00000659",
	}

	resp := EvalCapabilityPolicy(input)
	assert.True(t, resp.Allow)
}
