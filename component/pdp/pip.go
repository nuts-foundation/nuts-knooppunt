package pdp

import (
	"fmt"
	"log/slog"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func PipPolicyInput(c Component, policyInput PolicyInput) PolicyInput {
	if c.PIPClient == nil {
		slog.Warn("PIP client not configured")
		return policyInput
	}

	// If we have a patientId try and fetch the BSN
	patientId := policyInput.Context.PatientId
	if patientId != "" {
		client := c.PIPClient

		var patient fhir.Patient
		path := fmt.Sprintf("Patient/%s", patientId)
		err := client.Read(path, &patient)
		if err != nil {
			slog.Warn("Failed to get patient record from PIP, policy input might not be complete", logging.Error(err))
			return policyInput
		}

		bsns := fhirutil.FilterIdentifiersBySystem(patient.Identifier, coding.BSNNamingSystem)
		if len(bsns) == 0 {
			slog.Warn("Could not find BSN for patient record")
			return policyInput
		}

		if len(bsns) > 1 {
			slog.Warn("Patient record has multiple BSN's, defaulting to the first one")
		}
		bsn := bsns[0]

		if bsn.Value == nil {
			slog.Warn("BSN identifier is missing value")
			return policyInput
		}
		policyInput.Context.PatientBSN = *bsn.Value
	}
	return policyInput
}
