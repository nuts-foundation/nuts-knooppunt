package pdp

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

//go:embed policies/*/fhir_capabilitystatement.json
var FS embed.FS

func readCapability(ctx context.Context, name string) (fhir.CapabilityStatement, error) {
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
	// Skip capability checking for requests that don't target a specific resource type (e.g., /metadata, /)
	if input.Resource.Type == nil {
		return input, nil
	}

	statement, err := readCapability(ctx, policy)
	if err != nil {
		return input, []ResultReason{
			{
				Code:        TypeResultCodeUnexpectedInput,
				Description: "FHIR CapabilityStatement check failed: " + err.Error(),
			},
		}
	}

	resultReasons := evalInteraction(statement, input)
	input.Action.FHIRRest.CapabilityChecked = len(resultReasons) == 0
	return input, resultReasons
}

// evalInteraction checks whether the requested interaction is allowed by the capability statement.
// If not, it returns a list of reasons why not.
func evalInteraction(
	statement fhir.CapabilityStatement,
	input PolicyInput,
) []ResultReason {
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

	props := input.Action.FHIRRest

	if !slices.Contains(supported, props.InteractionType) {
		return []ResultReason{
			{
				Code:        TypeResultCodeNotImplemented,
				Description: "restful interaction type not supported",
			},
		}
	}

	var resourceDescriptions []fhir.CapabilityStatementRestResource
	for _, rest := range statement.Rest {
		for _, res := range rest.Resource {
			if res.Type == *input.Resource.Type {
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
		return []ResultReason{
			{
				Code:        TypeResultCodeNotAllowed,
				Description: "capability statement does not allow interaction",
			},
		}
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

		for paramName := range props.SearchParams {
			if !slices.Contains(allowedParams, paramName) {
				rejectedSearchParams = append(rejectedSearchParams, paramName)
			}
		}
	}

	allowParams = len(rejectedSearchParams) == 0
	if !allowParams {
		return []ResultReason{
			{
				Code:        TypeResultCodeNotAllowed,
				Description: fmt.Sprintf("search parameter %s not allowed", rejectedSearchParams),
			},
		}
	}

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

	allowIncludes := len(rejectedIncludes) == 0
	if !allowIncludes {
		return []ResultReason{
			{
				Code:        TypeResultCodeNotAllowed,
				Description: fmt.Sprintf("include %s is not allowed", rejectedIncludes),
			},
		}
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
		return []ResultReason{
			{
				Code:        TypeResultCodeNotAllowed,
				Description: fmt.Sprintf("revinclude %s is not allowed", rejectedRevincludes),
			},
		}
	}

	return nil
}
