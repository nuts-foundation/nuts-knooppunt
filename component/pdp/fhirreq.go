package pdp

import (
	"strings"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
	"golang.org/x/exp/maps"
)

type PathDef struct {
	Interaction fhir.TypeRestfulInteraction
	PathDef     []string
	Verb        string
}

// https://hl7.org/fhir/R4/http.html
var definitions = []PathDef{
	{
		Interaction: fhir.TypeRestfulInteractionRead,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionVread,
		PathDef:     []string{"[type]", "[id]", "_history", "[vid]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionUpdate,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "PUT",
	},
	{
		Interaction: fhir.TypeRestfulInteractionPatch,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "PATCH",
	},
	{
		Interaction: fhir.TypeRestfulInteractionDelete,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "DELETE",
	},
	{
		Interaction: fhir.TypeRestfulInteractionCreate,
		PathDef:     []string{"[type]"},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionSearchType,
		PathDef:     []string{"[type]?"},
		Verb:        "GET",
	},
	// TODO: Do we need to parse body params?
	{
		Interaction: fhir.TypeRestfulInteractionSearchType,
		PathDef:     []string{"[type]", "_search?"},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionSearchSystem,
		PathDef:     []string{"?"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionCapabilities,
		PathDef:     []string{"metadata"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionTransaction,
		PathDef:     []string{},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionHistoryInstance,
		PathDef:     []string{"[type]", "[id]", "_history"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionHistoryType,
		PathDef:     []string{},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionHistorySystem,
		PathDef:     []string{"_history"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"$[name]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"$[name]"},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"[type]", "$[name]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"[type]", "$[name]"},
		Verb:        "POST",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"[type]", "[id]", "$[name]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"[type]", "[id]", "$[name]"},
		Verb:        "POST",
	},
}

type Tokens struct {
	Interaction   fhir.TypeRestfulInteraction
	ResourceType  *fhir.ResourceType
	ResourceId    string
	OperationName string
	VersionId     string
}

func parsePath(def PathDef, req HTTPRequest) (Tokens, bool) {
	var out Tokens

	if def.Verb != req.Method {
		return Tokens{}, false
	}

	// Preprocesses the path for easier manipulation
	strPath := req.Path
	if strings.HasPrefix(strPath, "/") {
		strPath = strPath[1:]
	}
	path := strings.Split(strPath, "/")

	// Early return if the path has a different length than this definition
	if len(path) != len(def.PathDef) {
		return Tokens{}, false
	}

	for idx, part := range def.PathDef {
		switch part {
		case "[type]":
			ptr, ok := parseResourceType(path[idx])
			if !ok {
				return Tokens{}, false
			}
			out.ResourceType = ptr
			continue
		case "[type]?":
			str := strings.TrimSuffix(path[idx], "?")
			ptr, ok := parseResourceType(str)
			if !ok {
				return Tokens{}, false
			}
			out.ResourceType = ptr
			continue
		case "[id]":
			out.ResourceId = path[idx]
			continue
		case "[vid]":
			out.VersionId = path[idx]
			continue
		case "$[name]":
			out.OperationName = strings.TrimPrefix(path[idx], "$")
			continue
		}

		if path[idx] != part {
			return Tokens{}, false
		}
	}

	return out, true
}

func parseResourceType(str string) (*fhir.ResourceType, bool) {
	var t fhir.ResourceType
	err := t.UnmarshalJSON([]byte(str))
	if err != nil {
		return nil, false
	}
	return &t, true
}

func parseRequest(request HTTPRequest) (Tokens, bool) {
	var tokens Tokens
	var def PathDef
	var ok bool
	for _, d := range definitions {
		tokens, ok = parsePath(def, request)
		if ok {
			def = d
			break
		}
	}

	if !ok {
		return tokens, false
	}

	tokens.Interaction = def.Interaction
	return tokens, true
}

type Params struct {
	SearchParams []string
	Revinclude   []string
	Include      []string
}

func groupParams(queryParams map[string][]string) Params {
	var params Params

	params.Include = queryParams["_include"]
	delete(queryParams, "_include")

	params.Revinclude = queryParams["_revinclude"]
	delete(queryParams, "_revinclude")

	params.SearchParams = maps.Keys(queryParams)

	return params
}

func NewPolicyInput(request PDPRequest) (PolicyInput, bool) {
	var policyInput PolicyInput

	tokens, ok := parseRequest(request.Input.Request)
	if !ok {
		return PolicyInput{}, false
	}

	if tokens.ResourceType != nil {
		policyInput.Resource.Type = *tokens.ResourceType
		if tokens.ResourceId != "" {
			policyInput.Resource.Properties.ResourceId = tokens.ResourceId
		}
		if tokens.VersionId != "" {
			policyInput.Resource.Properties.VersionId = tokens.VersionId
		}
	}

	policyInput.Action.Properties = PolicyActionProperties{
		InteractionType: tokens.Interaction,
	}

	if tokens.OperationName != "" {
		policyInput.Action.Properties.Operation = &tokens.OperationName
	}

	params := groupParams(request.Input.Request.QueryParams)
	policyInput.Action.Properties.Include = params.Include
	policyInput.Action.Properties.Revinclude = params.Revinclude
	policyInput.Action.Properties.SearchParams = params.SearchParams

	return policyInput, true
}
