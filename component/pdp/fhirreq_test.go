package pdp

import (
	"net/http"
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

func TestComponent_group_params(t *testing.T) {
	queryParams := map[string][]string{
		"_since": {
			"1985-04-01",
		},
		"_revinclude": {
			"PractitionerRole:Location",
		},
		"_include": {
			"Location:managingOrganization",
		},
	}

	groupedParam := groupParams(queryParams)
	assert.Contains(t, groupedParam.SearchParams, "_since")
	assert.Contains(t, groupedParam.Include, "Location:managingOrganization")
	assert.Contains(t, groupedParam.Revinclude, "PractitionerRole:Location")
}

func TestComponent_params_in_body(t *testing.T) {
	pdpRequest := PDPRequest{
		Input: PDPInput{
			Request: HTTPRequest{
				Method:   "POST",
				Protocol: "HTTP/1.1",
				Path:     "/Patient/_search?",
				Header: http.Header{
					"Content-Type": []string{"application/x-www-form-urlencoded"},
				},
				Body: "identifier=775645332",
			},
		},
	}

	policyInput, policyResult := NewPolicyInput(pdpRequest)
	assert.True(t, policyResult.Allow)
	assert.Contains(t, policyInput.Action.Properties.SearchParams, "identifier")
}

func TestComponent_filter_result_param(t *testing.T) {
	queryParams := map[string][]string{
		"_total": {"10"},
	}
	params := groupParams(queryParams)
	assert.NotContains(t, params.SearchParams, "_total")
}
