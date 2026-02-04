package pdp

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, "1985-04-01", groupedParam.SearchParams["_since"])
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
				Body: "identifier=http://fhir.nl/fhir/NamingSystem/bsn|775645332",
			},
		},
	}

	policyInput, policyResult := NewPolicyInput(pdpRequest)
	assert.True(t, policyResult.Allow)
	assert.Equal(t, "http://fhir.nl/fhir/NamingSystem/bsn|775645332", policyInput.Action.Properties.SearchParams["identifier"])
	assert.Equal(t, "775645332", policyInput.Context.PatientBSN)
}

func TestComponent_filter_result_param(t *testing.T) {
	queryParams := map[string][]string{
		"_total": {"10"},
	}
	params := groupParams(queryParams)
	assert.Empty(t, params.SearchParams)
}

func TestComponent_parse_patient_id(t *testing.T) {
	pdpRequest := PDPRequest{
		Input: PDPInput{
			Request: HTTPRequest{
				Method:   "GET",
				Protocol: "HTTP/1.1",
				Path:     "/Patient/12345",
			},
		},
	}
	policyInput, _ := NewPolicyInput(pdpRequest)
	assert.Equal(t, "12345", policyInput.Context.PatientID)

	pdpRequest = PDPRequest{
		Input: PDPInput{
			Request: HTTPRequest{
				Method:   "GET",
				Protocol: "HTTP/1.1",
				Path:     "/Patient?",
				QueryParams: url.Values{
					"_id": []string{"56789"},
				},
			},
		},
	}
	policyInput, _ = NewPolicyInput(pdpRequest)
	assert.Equal(t, "56789", policyInput.Context.PatientID)

	pdpRequest = PDPRequest{
		Input: PDPInput{
			Request: HTTPRequest{
				Method:   "GET",
				Protocol: "HTTP/1.1",
				Path:     "/Encounter?",
				QueryParams: url.Values{
					"patient": []string{"Patient/98765"},
				},
			},
		},
	}
	policyInput, _ = NewPolicyInput(pdpRequest)
	assert.Equal(t, "98765", policyInput.Context.PatientID)
}

func TestNewPolicyInput(t *testing.T) {
	t.Run("patient resource ID parsing", func(t *testing.T) {
		t.Run("from Patient resource ID in path", func(t *testing.T) {
			pdpRequest := PDPRequest{
				Input: PDPInput{
					Request: HTTPRequest{
						Method:   "GET",
						Protocol: "HTTP/1.1",
						Path:     "/Patient/12345",
					},
				},
			}
			policyInput, _ := NewPolicyInput(pdpRequest)
			assert.Equal(t, "12345", policyInput.Context.PatientID)
		})
		t.Run("from _id query parameter", func(t *testing.T) {
			pdpRequest := PDPRequest{
				Input: PDPInput{
					Request: HTTPRequest{
						Method:   "GET",
						Protocol: "HTTP/1.1",
						Path:     "/Patient?",
						QueryParams: url.Values{
							"_id": []string{"56789"},
						},
					},
				},
			}
			policyInput, _ := NewPolicyInput(pdpRequest)
			assert.Equal(t, "56789", policyInput.Context.PatientID)
		})
		t.Run("from patient query parameter", func(t *testing.T) {
			pdpRequest := PDPRequest{
				Input: PDPInput{
					Request: HTTPRequest{
						Method:   "GET",
						Protocol: "HTTP/1.1",
						Path:     "/Encounter?",
						QueryParams: url.Values{
							"patient": []string{"Patient/98765"},
						},
					},
				},
			}
			policyInput, _ := NewPolicyInput(pdpRequest)
			assert.Equal(t, "98765", policyInput.Context.PatientID)
		})
		t.Run("multiple patient parameters", func(t *testing.T) {
			pdpRequest := PDPRequest{
				Input: PDPInput{
					Request: HTTPRequest{
						Method:   "GET",
						Protocol: "HTTP/1.1",
						Path:     "/Encounter?",
						QueryParams: url.Values{
							"patient": []string{"Patient/123", "Patient/456"},
						},
					},
				},
			}
			policyInput, result := NewPolicyInput(pdpRequest)
			assert.True(t, result.Allow)
			require.Len(t, result.Reasons, 1)
			assert.Equal(t, "patient_id: multiple patient parameters found", result.Reasons[0].Description)
			assert.Empty(t, policyInput.Context.PatientID)
		})
		t.Run("multiple _id parameters", func(t *testing.T) {
			pdpRequest := PDPRequest{
				Input: PDPInput{
					Request: HTTPRequest{
						Method:   "GET",
						Protocol: "HTTP/1.1",
						Path:     "/Patient?",
						QueryParams: url.Values{
							"_id": []string{"123", "456"},
						},
					},
				},
			}
			policyInput, result := NewPolicyInput(pdpRequest)
			assert.True(t, result.Allow)
			require.Len(t, result.Reasons, 1)
			assert.Equal(t, "patient_id: multiple _id parameters found", result.Reasons[0].Description)
			assert.Empty(t, policyInput.Context.PatientID)
		})
		t.Run("no patient ID provided", func(t *testing.T) {
			pdpRequest := PDPRequest{
				Input: PDPInput{
					Request: HTTPRequest{
						Method:   "GET",
						Protocol: "HTTP/1.1",
						Path:     "/Observation?",
					},
				},
			}
			policyInput, result := NewPolicyInput(pdpRequest)
			assert.True(t, result.Allow)
			assert.Empty(t, result.Reasons)
			assert.Empty(t, policyInput.Context.PatientID)
		})
	})
	t.Run("patient BSN parsing", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			pdpRequest := PDPRequest{
				Input: PDPInput{
					Request: HTTPRequest{
						Method:   "GET",
						Protocol: "HTTP/1.1",
						Path:     "/Patient",
						QueryParams: url.Values{
							"identifier": []string{"http://fhir.nl/fhir/NamingSystem/bsn|900186021"},
						},
						Header: http.Header{
							"Content-Type": []string{"application/fhir+json"},
						},
					},
				},
			}
			policyInput, _ := NewPolicyInput(pdpRequest)
			assert.Equal(t, "900186021", policyInput.Context.PatientBSN)
		})
		t.Run("incorrect system", func(t *testing.T) {
			pdpRequest := PDPRequest{
				Input: PDPInput{
					Request: HTTPRequest{
						Method:   "GET",
						Protocol: "HTTP/1.1",
						Path:     "/Patient",
						QueryParams: url.Values{
							"identifier": []string{"http://fhir.nl/fhir/NamingSystem/other|900186021"},
						},
						Header: http.Header{
							"Content-Type": []string{"application/fhir+json"},
						},
					},
				},
			}
			policyInput, result := NewPolicyInput(pdpRequest)
			assert.True(t, result.Allow)
			require.Len(t, result.Reasons, 1)
			assert.Equal(t, "patient_bsn: expected identifier system to be 'http://fhir.nl/fhir/NamingSystem/bsn', found 'http://fhir.nl/fhir/NamingSystem/other'", result.Reasons[0].Description)
			assert.Empty(t, policyInput.Context.PatientBSN)
		})
		t.Run("provided by PEP", func(t *testing.T) {
			pdpRequest := PDPRequest{
				Input: PDPInput{
					Request: HTTPRequest{
						Method:   "GET",
						Protocol: "HTTP/1.1",
						Path:     "/Patient",
						Header: http.Header{
							"Content-Type": []string{"application/fhir+json"},
						},
					},
					Context: PDPContext{
						PatientBSN: "900186021",
					},
				},
			}
			policyInput, _ := NewPolicyInput(pdpRequest)
			assert.Equal(t, "900186021", policyInput.Context.PatientBSN)
		})
	})

}
