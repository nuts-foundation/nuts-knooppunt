package pdp

import (
	"net/url"
	"regexp"
	"slices"
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
		PathDef:     []string{"[type]", "_history"},
		Verb:        "GET",
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

var regexId = regexp.MustCompile(`^[A-Za-z0-9\-\.]{1,64}$`)
var regexOperation = regexp.MustCompile(`^\$[a-z\-\.]+$`)

type Tokens struct {
	Interaction fhir.TypeRestfulInteraction

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
			ok := regexId.MatchString(path[idx])
			if !ok {
				return Tokens{}, false
			}
			out.ResourceId = path[idx]
			continue
		case "[vid]":
			ok := regexId.MatchString(path[idx])
			if !ok {
				return Tokens{}, false
			}
			out.VersionId = path[idx]
			continue
		case "$[name]":
			ok := regexOperation.MatchString(path[idx])
			if !ok {
				return Tokens{}, false
			}
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

func parseRequestPath(request HTTPRequest) (Tokens, bool) {
	var tokens Tokens
	var def PathDef
	var ok bool
	for _, d := range definitions {
		tokens, ok = parsePath(d, request)
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

var generalParams = []string{
	"_format",
	"_pretty",
	"_summary",
	"_elements",
}

var resultParams = []string{
	"_sort",
	"_count",
	"_include",
	"_revinclude",
	"_summary",
	"_total",
	"_elements",
	"_contained",
	"_containedType",
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

func NewPolicyInput(request PDPRequest) (PolicyInput, PolicyResult) {
	var policyInput PolicyInput

	tokens, ok := parseRequestPath(request.Input.Request)
	if !ok {
		reason := ResultReason{Code: TypeResultCodeUnexpectedInput, Description: "Not a valid FHIR request path"}
		return PolicyInput{}, Deny(reason)
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

	var rawParams map[string][]string
	contentType := request.Input.Request.Header.Get("Content-Type")
	hasFormData := contentType == "application/x-www-form-urlencoded"
	interWithBody := []fhir.TypeRestfulInteraction{
		fhir.TypeRestfulInteractionSearchType,
		fhir.TypeRestfulInteractionOperation,
	}
	paramsInBody :=
		slices.Contains(interWithBody, tokens.Interaction) &&
			hasFormData

	if paramsInBody {
		values, err := url.ParseQuery(request.Input.Request.Body)
		if err != nil {
			reason := ResultReason{
				Code:        TypeResultCodeUnexpectedInput,
				Description: "Could not parse form encoded data",
			}
			return PolicyInput{}, Deny(reason)
		}
		rawParams = values
	} else {
		rawParams = request.Input.Request.QueryParams
	}

	params := groupParams(rawParams)
	policyInput.Action.Properties.Include = params.Include
	policyInput.Action.Properties.Revinclude = params.Revinclude
	policyInput.Action.Properties.SearchParams = params.SearchParams
	policyInput.Subject = request.Input.Subject
	policyInput.Context.DataHolderOrganizationId = request.Input.Context.DataHolderOrganizationId
	policyInput.Context.DataHolderFacilityType = request.Input.Context.DataHolderFacilityType

	return policyInput, Allow()
}
