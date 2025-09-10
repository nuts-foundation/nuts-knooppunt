package coding

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func EqualsCode(coding fhir.Coding, system string, value string) bool {
	return coding.System != nil && *coding.System == system &&
		coding.Code != nil && *coding.Code == value
}

func CodableIncludesCode(codable fhir.CodeableConcept, code fhir.Coding) bool {
	for _, c := range codable.Coding {
		if *c.System == *code.System && *c.Code == *code.Code {
			return true
		}
	}
	return false
}

func CodablesIncludesCode(codables []fhir.CodeableConcept, code fhir.Coding) bool {
	for _, codable := range codables {
		if CodableIncludesCode(codable, code) {
			return true
		}
	}
	return false
}

func CodeToCodable(code fhir.Coding) fhir.CodeableConcept {
	return fhir.CodeableConcept{
		Coding: []fhir.Coding{code},
	}
}
