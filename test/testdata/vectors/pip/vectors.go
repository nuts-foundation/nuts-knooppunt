package pip

import (
	"net/url"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func HAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "policy-information-point",
		ID:   8,
	}
}

func Patients() []fhir.Patient {
	return []fhir.Patient{
		{
			Id: to.Ptr("3E439979-017F-40AA-594D-EBCF880FFD97"),
			Identifier: []fhir.Identifier{
				{
					System: to.Ptr("http://fhir.nl/fhir/NamingSystem/bsn"),
					Value:  to.Ptr("176286603"),
				},
			},
		},
	}
}

func Resources(fhirBaseURL *url.URL) []fhir.HasId {
	var resources []fhir.HasId
	for _, endpoint := range Patients() {
		resources = append(resources, &endpoint)
	}
	return resources
}
