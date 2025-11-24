package pdp

import (
	"context"
	"net/http"
	"slices"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
)

func EvalMitzPolicy(c Component, input MainPolicyInput) PolicyResult {
	// TODO: make this return more detailed information for what fields are missing
	ok := validateMitzInput(input)
	if !ok {
		return Deny(ResultReason{
			Code:        "input_not_valid",
			Description: "input not valid, missing required fields",
		})
	}

	ctx := context.Background()
	mitzComp := *c.Mitz
	consentReq := xacmlFromInput(input)
	consentResp, err := mitzComp.CheckConsent(ctx, consentReq)
	if err != nil {
		return Deny(ResultReason{
			Code:        "internal_error",
			Description: "internal error, could not complete consent check with Mitz",
		})
	}

	allow := false
	if consentResp.Decision == xacml.DecisionPermit {
		allow = true
	}

	if !allow {
		return Deny(ResultReason{
			Code:        "not_allowed",
			Description: "not allowed, denied by Mitz",
		})
	}

	return Allow()
}

func xacmlFromInput(input MainPolicyInput) xacml.AuthzRequest {
	var purpose string
	switch input.PurposeOfUse {
	case "treatment":
		purpose = "TREAT"
	case "secondary":
		purpose = "COC"
	default:
		purpose = "TREAT"
	}

	return xacml.AuthzRequest{
		PatientBSN:             input.PatientBSN,
		HealthcareFacilityType: input.DataHolderFacilityType,
		AuthorInstitutionID:    input.DataHolderOrganizationUra,
		// This code is always the same, it's the code for _de gesloten vraag_
		EventCode:              "GGC002",
		SubjectRole:            input.RequestingUziRoleCode,
		ProviderID:             input.RequestingPractitionerIdentifier,
		ProviderInstitutionID:  input.RequestingOrganizationUra,
		ConsultingFacilityType: input.RequestingFacilityType,
		PurposeOfUse:           purpose,
	}
}

func validateMitzInput(input MainPolicyInput) bool {
	requiredValues := []string{
		input.Scope,
		input.Method,
		input.PatientBSN,
		input.RequestingUziRoleCode,
		input.RequestingPractitionerIdentifier,
		input.RequestingOrganizationUra,
		input.RequestingFacilityType,
		input.DataHolderOrganizationUra,
		input.DataHolderFacilityType,
		input.PurposeOfUse,
	}
	if slices.Contains(requiredValues, "") {
		return false
	}

	validMethods := []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodPatch}
	if !slices.Contains(validMethods, input.Method) {
		return false
	}

	return true
}
