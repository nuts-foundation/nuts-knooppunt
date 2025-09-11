package coding

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func EqualsCoding(leftCoding fhir.Coding, rightCoding fhir.Coding) bool {
	var systemValid bool
	if leftCoding.System != nil && rightCoding.System != nil {
		systemValid = *leftCoding.System == *rightCoding.System
	} else {
		// If either or both systems are missing assume they are the same system
		systemValid = true
	}

	var codeValid bool
	if leftCoding.Code != nil && rightCoding.Code != nil {
		codeValid = *leftCoding.Code == *rightCoding.Code
	} else {
		codeValid = false
	}

	return systemValid && codeValid
}

func CodableIncludesCode(codable fhir.CodeableConcept, code fhir.Coding) bool {
	for _, c := range codable.Coding {
		if EqualsCoding(c, code) {
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
