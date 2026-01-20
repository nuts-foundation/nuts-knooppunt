package pdp

import (
	"context"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
)

func (c *Component) evalMitzPolicy(ctx context.Context, input PolicyInput) (PolicyInput, PolicyResult) {
	result := validateMitzInput(input)
	if !result.Allow {
		return input, result
	}

	consentReq := xacmlFromInput(input)
	consentResp, err := c.consentChecker.CheckConsent(ctx, consentReq)
	if err != nil {
		return input, Deny(ResultReason{
			Code:        TypeResultCodeInternalError,
			Description: "internal error, could not complete consent check with consentChecker",
		})
	}

	allow := false
	if consentResp.Decision == xacml.DecisionPermit {
		allow = true
	}

	if !allow {
		return input, Deny(ResultReason{
			Code:        TypeResultCodeInternalError,
			Description: "not allowed, denied by consentChecker",
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
	requiredData := []struct {
		Value   string
		Message string
	}{
		{
			Value:   input.Context.PatientBSN,
			Message: "Could not complete consentChecker consent check: Missing BSN",
		},
		{
			Value:   input.Context.DataHolderFacilityType,
			Message: "Could not complete consentChecker consent check: Missing data holder facility type",
		},
		{
			Value:   input.Context.DataHolderOrganizationId,
			Message: "Could not complete consentChecker consent check: Missing data holder organization ID",
		},
		{
			Value:   input.Subject.Properties.SubjectRole,
			Message: "Could not complete consentChecker consent check: Missing subject role",
		},
		{
			Value:   input.Subject.Properties.SubjectId,
			Message: "Could not complete consentChecker consent check: Missing subject id",
		},
		{
			Value:   input.Subject.Properties.SubjectOrganizationId,
			Message: "Could not complete consentChecker consent check: Missing subject organization ID",
		},
		{
			Value:   input.Subject.Properties.SubjectFacilityType,
			Message: "Could not complete consentChecker consent check: Missing subject facility type",
		},
	}

	errorReasons := make([]ResultReason, 0, len(requiredData))
	for _, def := range requiredData {
		if def.Value == "" {
			errorReasons = append(errorReasons, ResultReason{
				Code:        TypeResultCodeUnexpectedInput,
				Description: def.Message,
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
