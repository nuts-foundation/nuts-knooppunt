package pdp

import (
	"context"
	"slices"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
)

func EvalMitzPolicy(c Component, ctx context.Context, input PolicyInput) PolicyResult {
	// TODO: make this return more detailed information for what fields are missing
	ok := validateMitzInput(input)
	if !ok {
		return Deny(ResultReason{
			Code:        TypeResultCodeUnexpectedInput,
			Description: "input not valid, missing required fields",
		})
	}

	mitzComp := *c.Mitz
	consentReq := xacmlFromInput(input)
	consentResp, err := mitzComp.CheckConsent(ctx, consentReq)
	if err != nil {
		return Deny(ResultReason{
			Code:        TypeResultCodeInternalError,
			Description: "internal error, could not complete consent check with Mitz",
		})
	}

	allow := false
	if consentResp.Decision == xacml.DecisionPermit {
		allow = true
	}

	if !allow {
		return Deny(ResultReason{
			Code:        TypeResultCodeInternalError,
			Description: "not allowed, denied by Mitz",
		})
	}

	return Allow()
}

func xacmlFromInput(input PolicyInput) xacml.AuthzRequest {
	return xacml.AuthzRequest{
		PatientBSN:             input.PatientBSN,
		HealthcareFacilityType: input.DataHolderFacilityType,
		AuthorInstitutionID:    input.DataHolderUra,
		// This code is always the same, it's the code for _de gesloten vraag_
		EventCode:              "GGC002",
		SubjectRole:            input.RequestingUziRoleCode,
		ProviderID:             input.RequestingPractitionerIdentifier,
		ProviderInstitutionID:  input.RequestingUra,
		ConsultingFacilityType: input.RequestingFacilityType,
		PurposeOfUse:           "TREAT",
	}
}

func validateMitzInput(input PolicyInput) bool {
	requiredValues := []string{
		input.Scope,
		input.PatientBSN,
		input.RequestingUziRoleCode,
		input.RequestingPractitionerIdentifier,
		input.RequestingUra,
		input.RequestingFacilityType,
		input.DataHolderUra,
		input.DataHolderFacilityType,
	}
	if slices.Contains(requiredValues, "") {
		return false
	}

	return true
}
