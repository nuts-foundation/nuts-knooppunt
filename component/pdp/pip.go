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
	if c.pipClient == nil {
		slog.Warn("PIP client not configured")
		return policyInput
	}

	for idx, patient := range policyInput.Context.Patients {
		// If we have a patientId try and fetch the BSN
		if patient.PatientID != "" {
			client := c.pipClient

			var fhirPatient fhir.Patient
			path := fmt.Sprintf("Patient/%s", patient.PatientID)
			err := client.Read(path, &fhirPatient)
			if err != nil {
				slog.Warn("Failed to get patient record from PIP", logging.Error(err))
				continue
			}

			bsns := fhirutil.FilterIdentifiersBySystem(fhirPatient.Identifier, coding.BSNNamingSystem)
			if len(bsns) == 0 {
				slog.Warn("Could not find BSN for patient record")
				continue
			}

			if len(bsns) > 1 {
				slog.Warn("Could not determine BSN, patient record has multiple BSN's")
				continue
			}
			bsn := bsns[0]

			if bsn.Value == nil {
				slog.Warn("BSN identifier is missing value")
				continue
			}
			policyInput.Context.Patients[idx].PatientBSN = *bsn.Value
		}
	}
	return policyInput
}
