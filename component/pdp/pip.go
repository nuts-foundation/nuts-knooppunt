package pdp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func (c *Component) enrichPolicyInputWithPIP(ctx context.Context, policyInput *PolicyInput) (*PolicyInput, []ResultReason) {
	if c.pipClient == nil {
		slog.WarnContext(ctx, "PIP client not configured")
		return policyInput, []ResultReason{
			{
				Code:        TypeResultCodePIPError,
				Description: "PIP client not configured, policy input might not be complete",
			},
		}
	}

	// If we have a patientId try and fetch the BSN
	if policyInput.Context.PatientID != "" && policyInput.Context.PatientBSN == "" {
		client := c.pipClient

		var patient fhir.Patient
		path := fmt.Sprintf("Patient/%s", policyInput.Context.PatientID)
		err := client.Read(path, &patient)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get patient record from PIP", logging.Error(err))
			return policyInput, []ResultReason{
				{
					Code:        TypeResultCodePIPError,
					Description: fmt.Sprintf("failed to get patient record from PIP, policy input might not be complete: %v", err),
				},
			}
		}

		bsns := fhirutil.FilterIdentifiersBySystem(patient.Identifier, coding.BSNNamingSystem)
		if len(bsns) == 0 {
			return policyInput, []ResultReason{
				{
					Code:        TypeResultCodePIPError,
					Description: "could not find BSN for patient record from PIP, policy input might not be complete",
				},
			}
		}

		if len(bsns) > 1 {
			return policyInput, []ResultReason{
				{
					Code:        TypeResultCodePIPError,
					Description: "could not determine BSN, patient record has multiple BSN's",
				},
			}
		}
		bsn := bsns[0]

		if bsn.Value == nil {
			return policyInput, []ResultReason{
				{
					Code:        TypeResultCodePIPError,
					Description: "BSN identifier is missing value",
				},
			}
		}
		policyInput.Context.PatientBSN = *bsn.Value
	}

	// Check for local consent resources
	if policyInput.Action.FHIRRest.InteractionType == fhir.TypeRestfulInteractionRead {
		client := c.pipClient

		//	GET http://localhost:7050/fhir/policy-information-point/Consent?
		//	data=Patient/3E439979-017F-40AA-594D-EBCF880FFD97&
		//		organization:identifier=http://fhir.nl/fhir/NamingSystem/ura|00000030&
		//      actor:identifier=http://fhir.nl/fhir/NamingSystem/ura|00000040

		var searchResult fhir.Bundle
		client.SearchWithContext(ctx, "Consent", url.Values{
			"data": []string{policyInput.Resource.Id},
		}, &searchResult)

		type Ruling struct {
			Scope         string
			ProvisionType fhir.ConsentProvisionType
			// Not in use until we implement specificity rules
			// Specificity   int
		}
		var rulings []Ruling

		err := fhirutil.VisitBundleResources[fhir.Consent](&searchResult, func(consent *fhir.Consent) error {
			if len(consent.Scope.Coding) != 1 && consent.Scope.Coding[0].Code != nil {
				// Only continue if there's a single simple code
				// Complex coding scheme's not supported for now
				return nil
			}
			scope := *consent.Scope.Coding[0].Code

			var applyRuling bool

			applyRuling = consent.Status == fhir.ConsentStateActive

			applyRuling = applyRuling && coding.CodablesIncludesCode(consent.Provision.Action, fhir.Coding{
				System: to.Ptr("http://terminology.hl7.org/CodeSystem/consentaction"),
				Code:   to.Ptr("access"),
			})

			applyRuling = applyRuling && consent.Provision.Type != nil

			rulings = append(rulings, Ruling{
				Scope:         scope,
				ProvisionType: *consent.Provision.Type,
			})

			return nil
		})
		if err != nil {
			return policyInput, []ResultReason{
				{
					Code:        TypeResultCodePIPError,
					Description: "Error occurred while parsing consent resources in bundle",
				},
			}
		}

		finalRulings := map[string]bool{}
		for _, ruling := range rulings {
			// In case of an objection and a consent with equal specificness: Objections supersede consents
			// Therefore if we recorded an objection we will stop processing further rulings
			if finalRulings[ruling.Scope] == false {
				continue
			}

			if ruling.ProvisionType == fhir.ConsentProvisionTypeDeny {
				finalRulings[ruling.Scope] = false
				continue
			}

			if ruling.ProvisionType == fhir.ConsentProvisionTypePermit {
				finalRulings[ruling.Scope] = true
				continue
			}
		}

		for scope, consentGranted := range finalRulings {
			if consentGranted {
				policyInput.Resource.Consents = append(policyInput.Resource.Consents, PolicyConsent{
					Scope: scope,
				})
			}
		}
	}

	return policyInput, nil
}
