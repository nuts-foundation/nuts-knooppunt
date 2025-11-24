package pdp

import (
	"embed"
	"encoding/json"
	"fmt"

	"github.com/nuts-foundation/nuts-knooppunt/component/pdp"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

//go:embed *.json
var FS embed.FS

func readCapability(fileName string) (fhir.CapabilityStatement, error) {
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
	case "mcsd-update-client":
		cap, err := readCapability("nl-gf-admin-directory-update-client.json")
		return cap, err == nil
	default:
		return fhir.CapabilityStatement{}, false
	}
}

func EvalCapabilityPolicy(input pdp.MainPolicyInput) pdp.PolicyResult {
	out := pdp.PolicyResult{
		Allow: false,
	}

	if input.Scope == "" {
		reason := pdp.ResultReason{
			Code:        "missing_required_value",
			Description: "missing required value for scope field",
		}
		out.Reasons = []pdp.ResultReason{reason}
		return out
	}

	statement, ok := capabilityForScope(input.Scope)
	if !ok {
		reason := pdp.ResultReason{
			Code:        "unexpected_input",
			Description: "unexpected input, no capability statement known for scope",
		}
		out.Reasons = []pdp.ResultReason{reason}
		return out
	}

	return evalInteraction(statement, input.ResourceType, input.InteractionType)
}

func evalInteraction(
	statement fhir.CapabilityStatement,
	resourceType fhir.ResourceType,
	interaction fhir.TypeRestfulInteraction,
) pdp.PolicyResult {
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
		return pdp.PolicyResult{
			Allow: false,
			Reasons: []pdp.ResultReason{
				{
					Code:        "not_allowed",
					Description: "not allowed, capability statement does not allow interaction",
				},
			},
		}
	}

	return pdp.PolicyResult{
		Allow: true,
	}
}
