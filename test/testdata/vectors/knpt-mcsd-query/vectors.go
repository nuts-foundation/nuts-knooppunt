package knpt_mcsd_query

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func QueryHAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "knpt-mcsd-query",
		ID:   2,
	}
}
func Organization() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr("e5909595-767e-41c1-9b00-a23ddf33e5d2"),
		Meta: &fhir.Meta{
			Profile: []string{"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization"},
		},
		Name: to.Ptr("Sunflower Care Home"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr("00000020"),
			},
		},
		Endpoint: []fhir.Reference{
			{
				Reference: to.Ptr("Endpoint/f8a9c2d1-4567-489a-bcde-123456789abd"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

func Endpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("f8a9c2d1-4567-489a-bcde-123456789abd"),
			Address: "http://localhost:7050/fhir/sunflower-patients",
			Meta: &fhir.Meta{
				Profile: []string{"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ConnectionType: fhir.Coding{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/endpoint-connection-type"),
				Code:   to.Ptr("fhir"),
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-01T00:00:00Z"),
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
