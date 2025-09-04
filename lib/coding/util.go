package coding

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func EqualsCode(coding fhir.Coding, system string, value string) bool {
	return coding.System != nil && *coding.System == system &&
		coding.Code != nil && *coding.Code == value
}
