package formdata

import (
	"net/url"

	"github.com/nuts-foundation/nuts-knooppunt/component/mcsdadmin/valuesets"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func FilterEmpty(multiStrings []string) []string {
	out := make([]string, 0, len(multiStrings))
	for _, str := range multiStrings {
		if str != "" {
			out = append(out, str)
		}
	}
	return out
}

func CodablesFromForm(postform url.Values, set []fhir.Coding, key string) ([]fhir.CodeableConcept, bool) {
	nonEmpty := FilterEmpty(postform[key])
	return valuesets.CodablesFrom(set, nonEmpty)
}
