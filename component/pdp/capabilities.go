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
	case "mcsd_query":
		capa, err := readCapability(ctx, "nl-gf-query-directory-query-client")
		return capa, err == nil
	case "bgz_patient":
		capa, err := readCapability(ctx, "bgz-patient")
		return capa, err == nil
	case "bgz_professional":
		capa, err := readCapability(ctx, "bgz-professional")
		return capa, err == nil
	default:
		return fhir.CapabilityStatement{}, false
	}
}

func evalCapabilityPolicy(ctx context.Context, input PolicyInput) PolicyResult {
	out := PolicyResult{
		Allow: false,
	}

	scope := input.Subject.Properties.ClientQualifications[0]

	statement, ok := capabilityForScope(ctx, scope)
	if !ok {
		reason := ResultReason{
			Code:        TypeResultCodeUnexpectedInput,
			Description: "unexpected input, no capability statement known for scope",
		}
		out.Reasons = []ResultReason{reason}
		return out
	}

	return evalInteraction(statement, input)
}

func evalInteraction(
	statement fhir.CapabilityStatement,
	input PolicyInput,
) PolicyResult {
	policyResult := PolicyResult{
		Allow: false,
	}

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

	props := input.Action.Properties

	if !slices.Contains(supported, props.InteractionType) {
		return Deny(
			ResultReason{
				Code:        TypeResultCodeNotImplemented,
				Description: "restful interaction type not supported",
			})
	}

	var resourceDescriptions []fhir.CapabilityStatementRestResource
	for _, rest := range statement.Rest {
		for _, res := range rest.Resource {
			if res.Type == *input.Resource.Properties.ResourceType {
				resourceDescriptions = append(resourceDescriptions, res)
			}
		}
	}

	allowInteraction := false
	for _, des := range resourceDescriptions {
		for _, inter := range des.Interaction {
			if inter.Code == props.InteractionType {
				allowInteraction = true
			}
		}
	}

	if !allowInteraction {
		return Deny(
			ResultReason{
				Code:        TypeResultCodeNotAllowed,
				Description: "capability statement does not allow interaction",
			})
	}

	allowParams := false
	rejectedSearchParams := make([]string, 0, 10)
	if props.InteractionType == fhir.TypeRestfulInteractionSearchType {
		allowedParams := make([]string, 0, 10)
		for _, des := range resourceDescriptions {
			for _, param := range des.SearchParam {
				allowedParams = append(allowedParams, param.Name)
			}
		}

		for _, param := range props.SearchParams {
			if !slices.Contains(allowedParams, param) {
				rejectedSearchParams = append(rejectedSearchParams, param)
			}
		}
	}

	allowParams = len(rejectedSearchParams) == 0
	if !allowParams {
		policyResult.AddReasons(rejectedSearchParams, "search parameter %s is not allowed", TypeResultCodeNotAllowed)
		return policyResult
	}

	allowIncludes := false
	allowedIncludes := make([]string, 0, 10)
	for _, des := range resourceDescriptions {
		for _, include := range des.SearchInclude {
			allowedIncludes = append(allowedIncludes, include)
		}
	}

	rejectedIncludes := make([]string, 0, len(allowedIncludes))
	for _, inc := range props.Include {
		if !slices.Contains(allowedIncludes, inc) {
			rejectedIncludes = append(rejectedIncludes, inc)
		}
	}

	allowIncludes = len(rejectedIncludes) == 0
	if !allowIncludes {
		policyResult.AddReasons(rejectedIncludes, "include %s is not allowed", TypeResultCodeNotAllowed)
		return policyResult
	}

	allowRevincludes := false
	allowedRevincludes := make([]string, 0, 10)
	for _, des := range resourceDescriptions {
		for _, revinclude := range des.SearchRevInclude {
			allowedRevincludes = append(allowedRevincludes, revinclude)
		}
	}

	rejectedRevincludes := make([]string, 0, len(allowedRevincludes))
	for _, inc := range props.Revinclude {
		if !slices.Contains(allowedRevincludes, inc) {
			rejectedRevincludes = append(rejectedRevincludes, inc)
		}
	}

	allowRevincludes = len(rejectedRevincludes) == 0
	if !allowRevincludes {
		policyResult.AddReasons(rejectedRevincludes, "Revinclude %s is not allowed", TypeResultCodeNotAllowed)
		return policyResult
	}

	return Allow()
}
