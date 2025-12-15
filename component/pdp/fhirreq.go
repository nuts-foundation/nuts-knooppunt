package pdp

import (
	"strings"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type PathDef struct {
	Interaction fhir.TypeRestfulInteraction
	PathDef     []string
	Verb        string
}

var definitions = []PathDef{
	{
		Interaction: fhir.TypeRestfulInteractionRead,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "GET",
	},
	{
		Interaction: fhir.TypeRestfulInteractionHistorySystem,
		PathDef:     []string{"_history"},
		Verb:        "GET",
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
}

type Tokens struct {
	Interaction   fhir.TypeRestfulInteraction
	ResourceType  *fhir.ResourceType
	ResourceId    string
	OperationName string
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
	}

	policyInput.Action.Properties = PolicyActionProperties{
		InteractionType: tokens.Interaction,
	}

	if tokens.OperationName != "" {
		policyInput.Action.Properties.Operation = &tokens.OperationName
	}

	// TODO: Place query params

	return policyInput, true
}

// https://hl7.org/fhir/R4/http.html
