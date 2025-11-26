package pdp

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

//go:embed capabilities/*.json
var FS embed.FS

func readCapability(name string) (fhir.CapabilityStatement, error) {
	fileName := fmt.Sprintf("capabilities/%s.json", name)
	data, err := FS.ReadFile(fileName)
	if err != nil {
		return fhir.CapabilityStatement{}, err
	}

	var capability fhir.CapabilityStatement
	if err := json.Unmarshal(data, &capability); err != nil {
		log.Warn().Err(err).Msg(fmt.Sprintf("unable to read JSON in %s", fileName))
		return fhir.CapabilityStatement{}, err
	}

	return capability, nil
}

func capabilityForScope(scope string) (fhir.CapabilityStatement, bool) {
	switch scope {
	// FUTURE: Should be made configurable or packaged up with some policy
	case "mcsd_update":
		capa, err := readCapability("nl-gf-admin-directory-update-client")
		return capa, err == nil
	case "patient_example":
		capa, err := readCapability("patient-example")
		return capa, err == nil
	default:
		return fhir.CapabilityStatement{}, false
	}
}

func EvalCapabilityPolicy(input MainPolicyInput) PolicyResult {
	out := PolicyResult{
		Allow: false,
	}

	if input.Scope == "" {
		reason := ResultReason{
			Code:        "missing_required_value",
			Description: "missing required value for scope field",
		}
		out.Reasons = []ResultReason{reason}
		return out
	}

	statement, ok := capabilityForScope(input.Scope)
	if !ok {
		reason := ResultReason{
			Code:        "unexpected_input",
			Description: "unexpected input, no capability statement known for scope",
		}
		out.Reasons = []ResultReason{reason}
		return out
	}

	return evalInteraction(statement, input.ResourceType, input.InteractionType)
}

func evalInteraction(
	statement fhir.CapabilityStatement,
	resourceType fhir.ResourceType,
	interaction fhir.TypeRestfulInteraction,
) PolicyResult {
	var resourceDescriptions []fhir.CapabilityStatementRestResource
	for _, rest := range statement.Rest {
		for _, res := range rest.Resource {
			if res.Type == resourceType {
				resourceDescriptions = append(resourceDescriptions, res)
			}
		}
	}

	allowInteraction := false
	for _, des := range resourceDescriptions {
		for _, inter := range des.Interaction {
			if inter.Code == interaction {
				allowInteraction = true
			}
		}
	}

	if !allowInteraction {
		return PolicyResult{
			Allow: false,
			Reasons: []ResultReason{
				{
					Code:        "not_allowed",
					Description: "not allowed, capability statement does not allow interaction",
				},
			},
		}
	}

	return PolicyResult{
		Allow: true,
	}
}
