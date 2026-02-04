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

func (c *Component) enrichPolicyInputWithPIP(ctx context.Context, policyInput PolicyInput) PolicyInput {
	if c.pipClient == nil {
		slog.WarnContext(ctx, "PIP client not configured")
		return policyInput
	}

	// If we have a patientId try and fetch the BSN
	if policyInput.Action.Properties.ConnectionData.FHIRRest.PatientID != "" && policyInput.Context.PatientBSN == "" {
		client := c.pipClient

		var patient fhir.Patient
		path := fmt.Sprintf("Patient/%s", policyInput.Action.Properties.ConnectionData.FHIRRest.PatientID)
		err := client.Read(path, &patient)
		if err != nil {
			slog.WarnContext(ctx, "Failed to get patient record from PIP, policy input might not be complete", logging.Error(err))
			return policyInput
		}

		bsns := fhirutil.FilterIdentifiersBySystem(patient.Identifier, coding.BSNNamingSystem)
		if len(bsns) == 0 {
			slog.WarnContext(ctx, "Could not find BSN for patient record")
			return policyInput
		}

		if len(bsns) > 1 {
			slog.WarnContext(ctx, "Could not determine BSN, patient record has multiple BSN's")
			return policyInput
		}
		bsn := bsns[0]

		if bsn.Value == nil {
			slog.WarnContext(ctx, "BSN identifier is missing value")
			return policyInput
		}
		policyInput.Context.PatientBSN = *bsn.Value
	}
	return policyInput
}
