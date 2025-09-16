//go:generate go run codegen/main.go
package profile

import "github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"

const NLGenericFunctionOrganization = "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization"
const NLGenericFunctionEndpoint = "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-endpoint"
const NLGenericFunctionLocation = "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-location"
const NLGenericFunctionHealthcareService = "http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-healthcareservice"

func ForResourceType(resourceType string) *string {
	switch resourceType {
	case "Organization":
		return to.Ptr(NLGenericFunctionOrganization)
	case "Endpoint":
		return to.Ptr(NLGenericFunctionEndpoint)
	case "Location":
		return to.Ptr(NLGenericFunctionLocation)
	case "HealthcareService":
		return to.Ptr(NLGenericFunctionHealthcareService)
	}
	return nil
}
