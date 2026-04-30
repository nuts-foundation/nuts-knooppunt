package pdp

import (
	"context"
	"errors"
	"log/slog"

	"github.com/nuts-foundation/nuts-knooppunt/component/mitz/xacml"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
)

func (c *Component) enrichPolicyInputWithMitz(ctx context.Context, input *PolicyInput) (*PolicyInput, []ResultReason) {
	input.Context.MitzConsent = false
	// If Mitz is not configured, skip consent check
	if c.consentChecker == nil {
		slog.DebugContext(ctx, "Mitz consent checker not configured, skipping consent check")
		return input, nil
	}
	// If this call doesn't relate to a BSN don't attempt Mitz
	if input.Context.PatientBSN == "" {
		return input, nil
	}

	resultReasons := validateMitzInput(*input)
	if len(resultReasons) > 0 {
		return input, resultReasons
	}

	consentReq, err := xacmlFromInput(*input)
	if err != nil {
		slog.WarnContext(ctx, "Mitz consent check failed", "error", err)
		return input, []ResultReason{
			{
				Code:        TypeResultCodeUnexpectedInput,
				Description: "could not complete consent check with Mitz: " + err.Error(),
			},
		}
	}
	consentResp, err := c.consentChecker.CheckConsent(ctx, consentReq)
	if err != nil {
		slog.WarnContext(ctx, "Mitz consent check failed", "error", err)
		return input, []ResultReason{
			{
				Code:        TypeResultCodeInternalError,
				Description: "could not complete consent check with Mitz: " + err.Error(),
			},
		}
	}
	input.Context.MitzConsent = consentResp.Decision == xacml.DecisionPermit
	return input, nil
}

func xacmlFromInput(input PolicyInput) (xacml.AuthzRequest, error) {
	delRegByProp, isDelegated := input.Subject.OtherProps["delegation_registered_by"]
	delRoleProp, hasDelegatedRole := input.Subject.OtherProps["delegation_role_code"]
	var responsiblePractitioner string
	var responsiblePractitionerRole string
	if isDelegated {
		// If the request is mandated, the practitioner who mandated is responsible
		s, _ := delRegByProp.(string)
		iden, err := fhirutil.TokenToIdentifier(s)
		if err != nil && iden.Value != nil {
			responsiblePractitioner = *iden.Value
		} else {
			return xacml.AuthzRequest{}, errors.New("invalid format for delegation_registered_by")
		}

		if hasDelegatedRole {
			s, _ = delRoleProp.(string)
			iden, err = fhirutil.TokenToIdentifier(s)
			if err != nil && iden.Value != nil {
				responsiblePractitionerRole = *iden.Value
			} else {
				return xacml.AuthzRequest{}, errors.New("missing delegation_role_code")
			}
		} else {
			// Subject role is a mandatory field is Mitz
			// If the responsibility is delegated but there is no role, we can only error out
			return xacml.AuthzRequest{}, errors.New("missing delegation_role_code")
		}

	} else {
		// If the request is not mandated the (Dezi) authenticated practitioner is responsible
		responsiblePractitioner = input.Subject.User.Id
		responsiblePractitionerRole = input.Subject.User.Role
	}

	req := xacml.AuthzRequest{
		PatientBSN:             input.Context.PatientBSN,
		HealthcareFacilityType: input.Context.DataHolderFacilityType,
		AuthorInstitutionID:    input.Context.DataHolderOrganizationId,
		// This code is always the same, it's the code for _de gesloten vraag_
		EventCode:              "GGC002",
		SubjectRole:            responsiblePractitionerRole,
		ProviderID:             responsiblePractitioner,
		ProviderInstitutionID:  input.Subject.Organization.Ura,
		ConsultingFacilityType: input.Subject.Organization.FacilityType,
		PurposeOfUse:           "TREAT",
	}

	if isDelegated {
		// If the request is mandated add the practitioner that has been delegated to
		req.MandatedID = to.Ptr(input.Subject.User.Id)
	}
	return req, nil
}

func validateMitzInput(input PolicyInput) []ResultReason {
	requiredData := []struct {
		Value   string
		Message string
	}{
		{
			Value:   input.Context.PatientBSN,
			Message: "Could not complete Mitz consent check: Missing BSN",
		},
		{
			Value:   input.Context.DataHolderFacilityType,
			Message: "Could not complete Mitz consent check: Missing data holder facility type",
		},
		{
			Value:   input.Context.DataHolderOrganizationId,
			Message: "Could not complete Mitz consent check: Missing data holder organization ID",
		},
		{
			Value:   input.Subject.User.Role,
			Message: "Could not complete Mitz consent check: Missing subject user role",
		},
		{
			Value:   input.Subject.User.Id,
			Message: "Could not complete Mitz consent check: Missing subject user id",
		},
		{
			Value:   input.Subject.Organization.Ura,
			Message: "Could not complete Mitz consent check: Missing subject organization URA",
		},
		{
			Value:   input.Subject.Organization.FacilityType,
			Message: "Could not complete Mitz consent check: Missing subject organization facility type",
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
	return errorReasons
}
