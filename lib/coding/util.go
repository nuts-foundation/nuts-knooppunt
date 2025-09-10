package coding

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func EqualsCode(coding fhir.Coding, system string, value string) bool {
	return coding.System != nil && *coding.System == system &&
		coding.Code != nil && *coding.Code == value
}

func CodableIncludesCode(codable fhir.CodeableConcept, coding fhir.Coding) bool {
	for _, c := range codable.Coding {
		if *c.System == *coding.System && *c.Code == *coding.Code {
			return true
		}
	}
	return false
}

func CodablesIncludesCode(codables []fhir.CodeableConcept, coding fhir.Coding) bool {
	for _, codable := range codables {
		if CodableIncludesCode(codable, coding) {
			return true
		}
	}
	return false
}
