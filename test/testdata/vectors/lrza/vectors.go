package lrza

import (
	"net/url"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func HAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "lrza-mcsd-admin",
		ID:   3,
	}
}

// Organizations returns all organizations in the LRZa root directory
func Organizations() []fhir.Organization {
	return []fhir.Organization{
		CareHomeSunflower(),
		Care2Cure(),
	}
}

// Endpoints returns all root directory endpoints in the LRZa directory
func Endpoints(fhirBaseURL *url.URL) []fhir.Endpoint {
	var allEndpoints []fhir.Endpoint
	allEndpoints = append(allEndpoints, CareHomeSunflowerEndpoints(fhirBaseURL)...)
	allEndpoints = append(allEndpoints, Care2CureEndpoints(fhirBaseURL)...)
	return allEndpoints
}

func Resources(fhirBaseURL *url.URL) []fhir.HasId {
	var resources []fhir.HasId
	for _, endpoint := range Endpoints(fhirBaseURL) {
		resources = append(resources, &endpoint)
	}
	for _, org := range Organizations() {
		resources = append(resources, &org)
	}
	return resources
}
