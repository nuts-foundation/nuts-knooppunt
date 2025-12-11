package pdp

import (
	"strings"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

type Definition struct {
	Interaction fhir.TypeRestfulInteraction
	PathDef     []string
	Verb        string
}

var definitions = []Definition{
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
}

type Tokens struct {
	Definition   Definition
	ResourceType fhir.ResourceType
	ResourceId   string
	Operation    string
}

func parseDefinition(def Definition, req HTTPRequest) (Tokens, bool) {
	var out Tokens
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
		isToken := strings.HasPrefix(part, "[")
		switch part {
		case "[type]":
			var t fhir.ResourceType
			err := t.UnmarshalJSON([]byte(path[idx]))
			if err != nil {
				return Tokens{}, false
			}
			out.ResourceType = t
			continue
		case "[id]":
			out.ResourceId = path[idx]
			continue
		}

		isLiteral := !isToken
		if isLiteral {
			if path[idx] == part {
				return Tokens{}, false
			}
			continue
		}
	}

	out.Definition = def
	return out, true
}

// https://hl7.org/fhir/R4/http.html
