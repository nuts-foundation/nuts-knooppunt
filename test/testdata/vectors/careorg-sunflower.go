package vectors

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func CareHomeSunflower() fhir.Organization {
	return fhir.Organization{
		Id:   toPtr("e5909595-767e-41c1-9b00-a23ddf33e5d1"),
		Name: toPtr("Sunflower Care Home"),
		Identifier: []fhir.Identifier{
			{
				System: toPtr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  toPtr("00000020"),
			},
		},
	}
}

func CareHomeSunflowerRootEndpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      toPtr("cadbb0ba-0cf0-4f4e-8ee2-5a48a9fae724"),
			Address: "https://example.com/sunflower/mcsd",
			Meta: &fhir.Meta{
				Profile: []string{"https://profiles.ihe.net/ITI/mCSD/StructureDefinition/IHE.mCSD.Endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ManagingOrganization: &fhir.Reference{
				Reference: toPtr("Organization/e5909595-767e-41c1-9b00-a23ddf33e5d1"),
				Type:      toPtr("Organization"),
			},
			ConnectionType: fhir.Coding{
				System: toPtr("http://fhir.nl/fhir/NamingSystem/endpoint-connection-type"),
				Code:   toPtr("mcsd-directory"),
			},
			Period: &fhir.Period{
				Start: toPtr("2025-05-01T00:00:00Z"),
			},
		},
	}
}
