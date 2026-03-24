package pdp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"slices"

	fhirclient "github.com/SanteonNL/go-fhir-client"
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

		var patient struct {
			Identifier []fhir.Identifier `json:"identifier"`
		}
		path := fmt.Sprintf("Patient/%s", policyInput.Context.PatientID)
		err := client.ReadWithContext(ctx, path, &patient, fhirclient.QueryParam("_elements", "identifier"))
		if err != nil {
			slog.WarnContext(ctx, "Failed to get patient identifiers from PIP", logging.Error(err))
			return policyInput, []ResultReason{
				{
					Code:        TypeResultCodePIPError,
					Description: fmt.Sprintf("failed to get patient identifiers from PIP, policy input might not be complete: %v", err),
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
	if policyInput.Action.ConnectionTypeCode == "hl7-fhir-rest" &&
		policyInput.Action.FHIRRest.InteractionType == fhir.TypeRestfulInteractionRead {
		client := c.pipClient

		//	GET http://0.0.0.0:7050/fhir/policy-information-point/Consent?
		//	data=Patient/3E439979-017F-40AA-594D-EBCF880FFD97&
		var searchResult fhir.Bundle
		err := client.SearchWithContext(ctx, "Consent", url.Values{
			"data": []string{policyInput.Resource.Id},
		}, &searchResult)
		if err != nil {
			slog.ErrorContext(ctx, "PIP consent retrieval failed", logging.Error(err))
			return policyInput, []ResultReason{
				{
					Code:        TypeResultCodePIPError,
					Description: "Error occurred while retrieving consents",
				},
			}
		}

		var entries []fhir.BundleEntry
		err = fhirclient.Paginate(ctx, client, searchResult, func(searchSet *fhir.Bundle) (bool, error) {
			entries = append(entries, searchSet.Entry...)
			return true, nil
		})
		if err != nil {
			slog.ErrorContext(ctx, "PIP consent paginated retrieval failed", logging.Error(err))
			return policyInput, []ResultReason{
				{
					Code:        TypeResultCodePIPError,
					Description: "Error occurred while retrieving paginated consents",
				},
			}
		}

		type Ruling struct {
			Scope         string
			ProvisionType fhir.ConsentProvisionType
			// Not in use until we implement specificity rules
			// Specificity   int
		}
		var rulings []Ruling

		err = fhirutil.VisitBundleResources[fhir.Consent](&searchResult, func(consent *fhir.Consent) error {
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

			var orgUras []string
			for _, orgRef := range consent.Organization {
				if orgRef.Identifier != nil {
					ident := *orgRef.Identifier
					if ident.System != nil && *ident.System == "http://fhir.nl/fhir/NamingSystem/ura" {
						orgUras = append(orgUras, *ident.Value)
					}
				}
			}
			applyRuling = applyRuling && slices.Contains(orgUras, policyInput.Context.DataHolderOrganizationId)

			var actorUras []string
			for _, actor := range consent.Provision.Actor {
				if actor.Reference.Identifier != nil {
					ident := *actor.Reference.Identifier
					if ident.System != nil && *ident.System == "http://fhir.nl/fhir/NamingSystem/ura" {
						actorUras = append(actorUras, *ident.Value)
					}
				}
			}
			applyRuling = applyRuling && slices.Contains(actorUras, policyInput.Subject.Organization.Ura)

			applyRuling = applyRuling && consent.Provision.Type != nil

			if applyRuling {
				rulings = append(rulings, Ruling{
					Scope:         scope,
					ProvisionType: *consent.Provision.Type,
				})
			}

			return nil
		})
		if err != nil {
			slog.ErrorContext(ctx, "Failed to parse consent resources in bundle", logging.Error(err))
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
			current, ok := finalRulings[ruling.Scope]
			if ok && current == false {
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
