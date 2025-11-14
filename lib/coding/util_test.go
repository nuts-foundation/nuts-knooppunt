package coding

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func TestCodableIncludesCode(t *testing.T) {
	codable := fhir.CodeableConcept{
		Coding: []fhir.Coding{
			{
				System: to.Ptr("foo"),
				Code:   to.Ptr("bar"),
			},
		},
	}

	partialCode := fhir.Coding{
		Code: to.Ptr("bar"),
	}
	assert.True(t, CodableIncludesCode(codable, partialCode))

	partialCode.System = to.Ptr("baz")
	assert.False(t, CodableIncludesCode(codable, partialCode))

	partialCode.System = to.Ptr("foo")
	assert.True(t, CodableIncludesCode(codable, partialCode))

	partialCode.Code = to.Ptr("quz")
	assert.False(t, CodableIncludesCode(codable, partialCode))
}

func TestCodablesIncludesCode(t *testing.T) {
	codables := []fhir.CodeableConcept{
		{
			Coding: []fhir.Coding{
				{
					System: to.Ptr("foo"),
					Code:   to.Ptr("bar"),
				},
			},
		},
	}

	codeOne := fhir.Coding{
		System: to.Ptr("foo"),
		Code:   to.Ptr("bar"),
	}

	codeTwo := fhir.Coding{
		System: to.Ptr("qux"),
		Code:   to.Ptr("fred"),
	}

	assert.True(t, CodablesIncludesCode(codables, codeOne))
	assert.False(t, CodablesIncludesCode(codables, codeTwo))
}
