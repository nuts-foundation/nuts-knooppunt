package coding

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func EqualsCode(coding fhir.Coding, system string, value string) bool {
	return coding.System != nil && *coding.System == system &&
		coding.Code != nil && *coding.Code == value
}

func isCompleteCode(c fhir.Coding) bool {
	if c.System == nil || c.Code == nil {
		return false
	}
	return true
}

func CodableIncludesCode(codable fhir.CodeableConcept, code fhir.Coding) bool {
	if !isCompleteCode(code) {
		return false
	}

	for _, c := range codable.Coding {
		if !isCompleteCode(c) {
			continue
		}
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
