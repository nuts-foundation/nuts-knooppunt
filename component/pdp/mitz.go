package pdp

import (
	"context"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
)

func EvalMitzPolicy(c Component, ctx context.Context, input PolicyInput) (PolicyInput, PolicyResult) {
	result := validateMitzInput(input)
	if !result.Allow {
		return input, result
	}

	mitzComp := *c.Mitz
	consentReq := xacmlFromInput(input)
	consentResp, err := mitzComp.CheckConsent(ctx, consentReq)
	if err != nil {
		return input, Deny(ResultReason{
			Code:        TypeResultCodeInternalError,
			Description: "internal error, could not complete consent check with Mitz",
		})
	}

	allow := false
	if consentResp.Decision == xacml.DecisionPermit {
		allow = true
	}

	if !allow {
		return input, Deny(ResultReason{
			Code:        TypeResultCodeInternalError,
			Description: "not allowed, denied by Mitz",
		})
	}

	input.Context.MitzConsent = true
	return input, Allow()
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

func validateMitzInput(input PolicyInput) PolicyResult {
	requiredValues := []string{
		input.Context.PatientBSN,
		input.Context.DataHolderFacilityType,
		input.Context.DataHolderOrganizationId,
		input.Subject.Properties.SubjectRole,
		input.Subject.Properties.SubjectId,
		input.Subject.Properties.SubjectOrganizationId,
		input.Subject.Properties.SubjectFacilityType,
	}
	errorMessages := []string{
		"Could not complete Mitz consent check: Missing data holder facility type",
		"Could not complete Mitz consent check: Missing data holder organization ID",
		"Could not complete Mitz consent check: Missing subject role",
		"Could not complete Mitz consent check: Missing subject id",
		"Could not complete Mitz consent check: Missing subject organization ID",
		"Could not complete Mitz consent check: Missing subject facility type",
	}

	errorReasons := make([]ResultReason, 0, len(requiredValues))
	for idx, val := range requiredValues {
		if val == "" {
			errorReasons = append(errorReasons, ResultReason{
				Code:        TypeResultCodeUnexpectedInput,
				Description: errorMessages[idx],
			})
		}
	}

	if len(errorReasons) > 0 {
		return PolicyResult{
			Allow:   false,
			Reasons: errorReasons,
		}
	}

	return Allow()
}
