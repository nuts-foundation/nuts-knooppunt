package pdp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestComponent_parse_read(t *testing.T) {
	var def = Definition{
		Interaction: fhir.TypeRestfulInteractionRead,
		PathDef:     []string{"[type]", "[id]"},
		Verb:        "GET",
	}

	var req = HTTPRequest{
		Method: "GET",
		Path:   "/Observation/12775",
	}
	tokens, ok := parseDefinition(def, req)

	assert.True(t, ok)
	assert.Equal(t, fhir.ResourceTypeObservation, tokens.ResourceType)
	assert.Equal(t, fhir.ResourceTypeObservation, tokens.ResourceType)
}

func TestComponent_parse_history_all(t *testing.T) {
	var def = Definition{
		Interaction: fhir.TypeRestfulInteractionHistorySystem,
		PathDef:     []string{"_history"},
		Verb:        "GET",
	}

	var req = HTTPRequest{
		Method: "GET",
		Path:   "/_history",
	}
	tokens, ok := parseDefinition(def, req)

	assert.True(t, ok)
	assert.Equal(t, fhir.TypeRestfulInteractionHistorySystem, tokens.InteractionType)
}
