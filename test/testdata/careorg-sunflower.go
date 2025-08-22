package testdata

import (
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func CareHomeSunflower() fhir.Organization {
	return fhir.Organization{
		Id:   to.Ptr("e5909595-767e-41c1-9b00-a23ddf33e5d1"),
		Name: to.Ptr("Sunflower Care Home"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr("00000020"),
			},
		},
	}
}

func CareHomeSunflowerRootEndpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("cadbb0ba-0cf0-4f4e-8ee2-5a48a9fae724"),
			Address: "https://example.com/sunflower/mcsd",
			Meta: &fhir.Meta{
				Profile: []string{"https://profiles.ihe.net/ITI/mCSD/StructureDefinition/IHE.mCSD.Endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ManagingOrganization: &fhir.Reference{
				Reference: to.Ptr("Organization/e5909595-767e-41c1-9b00-a23ddf33e5d1"),
				Type:      to.Ptr("Organization"),
			},
			ConnectionType: fhir.Coding{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/endpoint-connection-type"),
				Code:   to.Ptr("mcsd-directory"),
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-05-01T00:00:00Z"),
			},
		},
	}
}
