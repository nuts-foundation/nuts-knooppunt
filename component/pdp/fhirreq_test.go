package pdp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_parse_tokens(t *testing.T) {
	var def = PathDef{
		Interaction: fhir.TypeRestfulInteractionRead,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "GET",
	}

	var req = HTTPRequest{
		Method: "GET",
		Path:   "/Observation/12775",
	}
	tokens, ok := parsePath(def, req)

	assert.True(t, ok)
	assert.Equal(t, "12775", tokens.ResourceId)
	assert.Equal(t, fhir.ResourceTypeObservation, *tokens.ResourceType)
}

func TestComponent_parse_literals(t *testing.T) {
	var def = PathDef{
		Interaction: fhir.TypeRestfulInteractionHistorySystem,
		PathDef:     []string{"_history"},
		Verb:        "GET",
	}

	var req = HTTPRequest{
		Method: "GET",
		Path:   "/_history",
	}
	_, ok := parsePath(def, req)

	assert.True(t, ok)
}

func TestComponent_parse_trailing_question_mark(t *testing.T) {
	var def = PathDef{
		Interaction: fhir.TypeRestfulInteractionSearchType,
		PathDef:     []string{"[type]?"},
		Verb:        "GET",
	}

	var req = HTTPRequest{
		Method: "GET",
		Path:   "/Observation?",
	}
	tokens, ok := parsePath(def, req)

	assert.True(t, ok)
	assert.Equal(t, fhir.ResourceTypeObservation, *tokens.ResourceType)
}

func TestComponent_parse_leading_dollar(t *testing.T) {
	var def = PathDef{
		Interaction: fhir.TypeRestfulInteractionOperation,
		PathDef:     []string{"[type]", "[id]", "$[name]"},
		Verb:        "GET",
	}

	var req = HTTPRequest{
		Method: "GET",
		Path:   "/Observation/123123/$validate",
	}
	tokens, ok := parsePath(def, req)

	assert.True(t, ok)
	assert.Equal(t, "validate", tokens.OperationName)
}
