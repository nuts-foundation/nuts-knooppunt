package lrza

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// lrza-root-directory is the single central addressing directory: every care
// organization's Organization, Endpoint and HealthcareService resources.
func HAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "lrza-root-directory",
		ID:   11,
	}
}

// Organizations returns every care organization in the central addressing directory.
func Organizations() []fhir.Organization {
	return []fhir.Organization{
		CareHomeSunflower(),
		Care2Cure(),
		HospitalPlataan(),
	}
}

// Endpoints returns every organization's endpoints in the central
// addressing directory.
func Endpoints() []fhir.Endpoint {
	var allEndpoints []fhir.Endpoint
	allEndpoints = append(allEndpoints, CareHomeSunflowerEndpoints()...)
	allEndpoints = append(allEndpoints, Care2CureEndpoints()...)
	allEndpoints = append(allEndpoints, HospitalPlataanEndpoints()...)
	return allEndpoints
}

// HealthcareServices returns every organization's healthcare services in the
// central addressing directory.
func HealthcareServices() []fhir.HealthcareService {
	return CareHomeSunflowerHealthcareServices()
}

func Resources() []fhir.HasId {
	var resources []fhir.HasId
	for _, endpoint := range Endpoints() {
		resources = append(resources, &endpoint)
	}
	for _, org := range Organizations() {
		resources = append(resources, &org)
	}
	for _, service := range HealthcareServices() {
		resources = append(resources, &service)
	}
	return resources
}
