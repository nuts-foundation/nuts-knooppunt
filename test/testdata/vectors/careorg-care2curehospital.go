package vectors

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Care2CureHospital() fhir.Organization {
	return fhir.Organization{
		Id:   toPtr("ef860868-b886-4459-aa87-216955c05289"),
		Name: toPtr("Care2Cure Hospital"),
		Identifier: []fhir.Identifier{
			{
				System: toPtr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  toPtr("00000030"),
			},
		},
	}
}

func Care2CureHospitalRootEndpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      toPtr("099177e6-5523-4e49-a1c2-0fd8955853d"),
			Address: "https://example.com/care2curehospital/mcsd",
			Meta: &fhir.Meta{
				Profile: []string{"https://profiles.ihe.net/ITI/mCSD/StructureDefinition/IHE.mCSD.Endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ManagingOrganization: &fhir.Reference{
				Reference: toPtr("Organization/ef860868-b886-4459-aa87-216955c05289"),
				Type:      toPtr("Organization"),
			},
			ConnectionType: fhir.Coding{
				System: toPtr("http://fhir.nl/fhir/NamingSystem/endpoint-connection-type"),
				Code:   toPtr("mcsd-directory"),
			},
			Period: &fhir.Period{
				Start: toPtr("2025-01-01T00:00:00Z"),
			},
		},
	}
}
