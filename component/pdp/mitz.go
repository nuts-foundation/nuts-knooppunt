package pdp

import (
	"context"
	"slices"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
)

func EvalMitzPolicy(c Component, ctx context.Context, input PolicyInput) PolicyResult {
	ok := validateMitzInput(input)
	if !ok {
		return Deny(ResultReason{
			Code:        TypeResultCodeInternalError,
			Description: "internal error, could not complete consent check with Mitz",
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
		PatientBSN:             input.Context.PatientBSN,
		HealthcareFacilityType: input.Context.DataHolderFacilityType,
		AuthorInstitutionID:    input.Context.DataHolderOrganizationId,
		// This code is always the same, it's the code for _de gesloten vraag_
		EventCode:              "GGC002",
		SubjectRole:            input.Subject.Properties.SubjectRole,
		ProviderID:             input.Subject.Properties.SubjectId,
		ProviderInstitutionID:  input.Subject.Properties.SubjectOrganizationId,
		ConsultingFacilityType: input.Subject.Properties.SubjectFacilityType,
		PurposeOfUse:           "TREAT",
	}
}

func validateMitzInput(input PolicyInput) bool {
	requiredValues := []string{
		input.Context.PatientBSN,
		input.Context.DataHolderFacilityType,
		input.Context.DataHolderOrganizationId,
		input.Subject.Properties.SubjectRole,
		input.Subject.Properties.SubjectId,
		input.Subject.Properties.SubjectOrganizationId,
		input.Subject.Properties.SubjectFacilityType,
	}
	if slices.Contains(requiredValues, "") {
		return false
	}

	return true
}
