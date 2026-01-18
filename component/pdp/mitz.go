package pdp

import (
	"context"
	"fmt"
	"slices"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
)

func EvalMitzPolicy(c Component, ctx context.Context, input PolicyInput) (PolicyInput, PolicyResult) {
	result := validateMitzInput(input)
	if !result.Allow {
		return input, result
	}

	mitzComp := *c.Mitz
	var errorReasons []ResultReason

	for _, patient := range input.Context.Patients {
		consentReq := xacmlFromInput(input, patient)
		consentResp, err := mitzComp.CheckConsent(ctx, consentReq)
		if err != nil {
			errorReasons = append(errorReasons, ResultReason{
				Code:        TypeResultCodeInternalError,
				Description: fmt.Sprintf("could not complete consent check with Mitz for patient %s", patient.PatientID),
			})
			continue
		}

		allow := false
		if consentResp.Decision == xacml.DecisionPermit {
			allow = true
		}

		if !allow {
			errorReasons = append(errorReasons, ResultReason{
				Code:        TypeResultCodeNotAllowed,
				Description: fmt.Sprintf("not allowed, consent denied for %s by Mitz", patient.PatientID),
			})
			continue
		}

		patient.MitzConsent = true
	}

	if len(errorReasons) > 0 {
		return input, PolicyResult{
			Allow:   false,
			Reasons: errorReasons,
		}
	}

	return input, Allow()
}

func xacmlFromInput(input PolicyInput, patient PolicyPatient) xacml.AuthzRequest {
	return xacml.AuthzRequest{
		PatientBSN:             patient.PatientBSN,
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
			Value:   input.Context.DataHolderFacilityType,
			Message: "Could not complete Mitz consent check: Missing data holder facility type",
		},
		{
			Value:   input.Context.DataHolderOrganizationId,
			Message: "Could not complete Mitz consent check: Missing data holder organization ID",
		},
		{
			Value:   input.Subject.Properties.SubjectRole,
			Message: "Could not complete Mitz consent check: Missing subject role",
		},
		{
			Value:   input.Subject.Properties.SubjectId,
			Message: "Could not complete Mitz consent check: Missing subject id",
		},
		{
			Value:   input.Subject.Properties.SubjectOrganizationId,
			Message: "Could not complete Mitz consent check: Missing subject organization ID",
		},
		{
			Value:   input.Subject.Properties.SubjectFacilityType,
			Message: "Could not complete Mitz consent check: Missing subject facility type",
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

	if len(input.Context.Patients) < 0 {
		errorReasons = append(errorReasons, ResultReason{
			Code:        TypeResultCodeUnexpectedInput,
			Description: "Could not complete Mitz consent check: Missing BSN",
		})
	} else {
		anyBsn := slices.ContainsFunc(input.Context.Patients, func(p PolicyPatient) bool {
			return p.PatientBSN != ""
		})
		if !anyBsn {
			errorReasons = append(errorReasons, ResultReason{
				Code:        TypeResultCodeUnexpectedInput,
				Description: "Could not complete Mitz consent check: Missing BSN",
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
