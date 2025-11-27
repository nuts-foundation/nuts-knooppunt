package pdp

import (
	"github.com/nuts-foundation/nuts-knooppunt/component/mitz"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type MainPolicyInput struct {
	Scope                            string                      `json:"scope"`
	Method                           string                      `json:"method"`
	Path                             []string                    `json:"path"`
	PatientBSN                       string                      `json:"patient_bsn"`
	PurposeOfUse                     string                      `json:"purpose_of_use"`
	DataHolderFacilityType           string                      `json:"data_holder_facility_type"`
	DataHolderOrganizationUra        string                      `json:"data_holder_organization_ura"`
	RequestingFacilityType           string                      `json:"requesting_facility_type"`
	RequestingOrganizationUra        string                      `json:"requesting_organization_ura"`
	RequestingPractitionerIdentifier string                      `json:"requesting_practitioner_identifier"`
	RequestingUziRoleCode            string                      `json:"requesting_uzi_role_code"`
	InteractionType                  fhir.TypeRestfulInteraction `json:"interaction_type"`
	ResourceType                     fhir.ResourceType           `json:"resource_type"`
	SearchParams                     []string                    `json:"search_params"`
	ResourceId                       string                      `json:"resource_id"`
}

type MainPolicyRequest struct {
	Input MainPolicyInput `json:"input"`
}

type MainPolicyResponse struct {
	Result PolicyResult `json:"result"`
}

type PolicyResult struct {
	Allow   bool           `json:"allow"`
	Reasons []ResultReason `json:"reasons"`
}

type ResultReason struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

// Allow helper for creating an allowed result without reasons
func Allow() PolicyResult {
	return PolicyResult{
		Allow: true,
	}
}

// Deny Helper for creating a result with a single deny reason
func Deny(reason ResultReason) PolicyResult {
	return PolicyResult{
		Allow: false,
		Reasons: []ResultReason{
			reason,
		},
	}
}

// TODO: Turn this into an enum
var resultCodes = []string{
	"missing_required_value",
	"unexpected_input",
	"not_allowed",
	"not_implemented",
	"internal_error",
}

type Config struct {
	Enabled bool
}

type Component struct {
	Config Config
	Mitz   *mitz.Component
}
