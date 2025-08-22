package testdata

import (
	"github.com/nuts-foundation/nuts-knooppunt/lib/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Care2CureHospital() fhir.Organization {
	return fhir.Organization{
		Id:   to.Ptr("ef860868-b886-4459-aa87-216955c05289"),
		Name: to.Ptr("Care2Cure Hospital"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr("00000030"),
			},
		},
	}
}

func Care2CureHospitalRootEndpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("099177e6-5523-4e49-a1c2-0fd8955853d"),
			Address: "https://example.com/care2curehospital/mcsd",
			Meta: &fhir.Meta{
				Profile: []string{"https://profiles.ihe.net/ITI/mCSD/StructureDefinition/IHE.mCSD.Endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ManagingOrganization: &fhir.Reference{
				Reference: to.Ptr("Organization/ef860868-b886-4459-aa87-216955c05289"),
				Type:      to.Ptr("Organization"),
			},
			ConnectionType: fhir.Coding{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/endpoint-connection-type"),
				Code:   to.Ptr("mcsd-directory"),
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-01T00:00:00Z"),
			},
		},
	}
}
