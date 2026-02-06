package pdp

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func (c *Component) enrichPolicyInputWithPIP(ctx context.Context, policyInput PolicyInput) (PolicyInput, []ResultReason) {
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
	return policyInput, nil
}
