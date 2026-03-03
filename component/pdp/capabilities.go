package pdp

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

//go:embed policies/*/fhir_capabilitystatement.json
var FS embed.FS

func readCapability(name string) (fhir.CapabilityStatement, error) {
	fileName := fmt.Sprintf("policies/%s/fhir_capabilitystatement.json", name)
	data, err := FS.ReadFile(fileName)
	if err != nil {
		return fhir.CapabilityStatement{}, fmt.Errorf("file read: %w", err)
	}

	var capability fhir.CapabilityStatement
	if err := json.Unmarshal(data, &capability); err != nil {
		return fhir.CapabilityStatement{}, fmt.Errorf("JSON unmarshal: %w", err)
	}

	return capability, nil
}

func enrichPolicyInputWithCapabilityStatement(ctx context.Context, input PolicyInput, policy string) (PolicyInput, []ResultReason) {
	statement, err := readCapability(policy)
	if err != nil {
		return input, []ResultReason{
			{
				Code:        TypeResultCodeUnexpectedInput,
				Description: fmt.Sprintf("missing FHIR CapabilityStatement '%s': %s", policy, err.Error()),
			},
		}
	}
	// Add the capability statement to the input so OPA can evaluate it
	input.CapabilityStatement = &statement
	return input, nil
}
