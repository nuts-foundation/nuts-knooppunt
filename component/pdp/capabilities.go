package pdp

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

//go:embed capabilities/*.json
var FS embed.FS

func readCapability(ctx context.Context, name string) (fhir.CapabilityStatement, error) {
	fileName := fmt.Sprintf("capabilities/%s.json", name)
	data, err := FS.ReadFile(fileName)
	if err != nil {
		return fhir.CapabilityStatement{}, err
	}

	var capability fhir.CapabilityStatement
	if err := json.Unmarshal(data, &capability); err != nil {
		slog.WarnContext(ctx, "unable to read JSON", slog.String("file", fileName), logging.Error(err))
		return fhir.CapabilityStatement{}, err
	}

	return capability, nil
}

func capabilityForScope(ctx context.Context, scope string) (fhir.CapabilityStatement, bool) {
	switch scope {
	// FUTURE: Should be made configurable or packaged up with some policy
	case "mcsd_update":
		capa, err := readCapability(ctx, "nl-gf-admin-directory-update-client")
		return capa, err == nil
	case "patient_example":
		capa, err := readCapability(ctx, "patient-example")
		return capa, err == nil
	default:
		return fhir.CapabilityStatement{}, false
	}
}

func evalCapabilityPolicy(ctx context.Context, input MainPolicyInput) PolicyResult {
	out := PolicyResult{
		Allow: false,
	}

	statement, ok := capabilityForScope(ctx, input.Scope)
	if !ok {
		reason := ResultReason{
			Code:        "unexpected_input",
			Description: "unexpected input, no capability statement known for scope",
		}
		out.Reasons = []ResultReason{reason}
		return out
	}

	return evalInteraction(statement, input)
}

func evalInteraction(
	statement fhir.CapabilityStatement,
	input MainPolicyInput,
) PolicyResult {
	// FUTURE: This is a pretty naive implementation - we can make it more efficient at a later point.
	var supported = []fhir.TypeRestfulInteraction{
		fhir.TypeRestfulInteractionRead,
		fhir.TypeRestfulInteractionVread,
		fhir.TypeRestfulInteractionUpdate,
		fhir.TypeRestfulInteractionPatch,
		fhir.TypeRestfulInteractionDelete,
		fhir.TypeRestfulInteractionHistoryInstance,
		fhir.TypeRestfulInteractionHistoryType,
		fhir.TypeRestfulInteractionCreate,
		fhir.TypeRestfulInteractionSearchType,
	}
	if !slices.Contains(supported, input.InteractionType) {
		return PolicyResult{
			Allow: false,
			Reasons: []ResultReason{
				{
					Code:        "not_implemented",
					Description: "restful interaction type not supported",
				},
			},
		}
	}

	var resourceDescriptions []fhir.CapabilityStatementRestResource
	for _, rest := range statement.Rest {
		for _, res := range rest.Resource {
			if res.Type == input.ResourceType {
				resourceDescriptions = append(resourceDescriptions, res)
			}
		}
	}

	allowInteraction := false
	for _, des := range resourceDescriptions {
		for _, inter := range des.Interaction {
			if inter.Code == input.InteractionType {
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
					Description: "capability statement does not allow interaction",
				},
			},
		}
	}

	allowParams := false
	rejectedSearchParams := make([]string, 0, 10)
	if input.InteractionType == fhir.TypeRestfulInteractionSearchType {
		allowedParams := make([]string, 0, 10)
		for _, des := range resourceDescriptions {
			for _, param := range des.SearchParam {
				allowedParams = append(allowedParams, param.Name)
			}
		}

		for _, param := range input.SearchParams {
			if !slices.Contains(allowedParams, param) {
				rejectedSearchParams = append(rejectedSearchParams, param)
			}
		}
	}
	if len(rejectedSearchParams) == 0 {
		allowParams = true
	}

	if !allowParams {
		reasons := make([]ResultReason, 0, 10)
		for _, param := range rejectedSearchParams {
			reason := ResultReason{
				Code:        "not_allowed",
				Description: fmt.Sprintf("search parameter %s is not allowed", param),
			}
			reasons = append(reasons, reason)
		}
		return PolicyResult{
			Allow:   false,
			Reasons: reasons,
		}
	}

	return PolicyResult{
		Allow: true,
	}
}
