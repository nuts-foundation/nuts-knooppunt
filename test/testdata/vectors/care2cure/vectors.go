package care2cure

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func AdminHAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "care2cure-admin",
		ID:   4,
	}
}

func Organization() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr("a3e4080d-8d53-4e53-bfbc-564e85158649"),
		Meta: &fhir.Meta{
			Profile: []string{"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization"},
		},
		Name: to.Ptr("Care2Cure Hospital"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr("00000030"),
			},
		},
		Endpoint: []fhir.Reference{
			{
				Reference: to.Ptr("Endpoint/bce8a799-e6ba-4c06-8a1c-bc052f01a636"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

func Endpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("bce8a799-e6ba-4c06-8a1c-bc052f01a636"),
			Address: "https://example.com/care2curehospital/fhir",
			Meta: &fhir.Meta{
				Profile: []string{"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ConnectionType: fhir.Coding{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/endpoint-connection-type"),
				Code:   to.Ptr("fhir"),
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-02T00:00:00Z"),
			},
		},
	}
}

func Resources() []fhir.HasId {
	var resources []fhir.HasId
	for _, endpoint := range Endpoints() {
		resources = append(resources, to.Ptr(endpoint))
	}
	resources = append(resources, to.Ptr(Organization()))
	return resources
}
