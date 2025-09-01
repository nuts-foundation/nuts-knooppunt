package lrza

import (
	"net/url"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Care2Cure() fhir.Organization {
	return fhir.Organization{
		Id:   to.Ptr("ef860868-b886-4459-aa87-216955c05289"),
		Name: to.Ptr("Care2Cure Hospital"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr("00000030"),
			},
		},
		Endpoint: []fhir.Reference{
			{
				Reference: to.Ptr("Endpoint/099177e6-5523-4e49-a1c2-0fd8955853d"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

func Care2CureEndpoints(hapiBaseURL *url.URL) []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("099177e6-5523-4e49-a1c2-0fd8955853d"),
			Address: hapiBaseURL.JoinPath("care2cure-admin").String(),
			Meta: &fhir.Meta{
				Profile: []string{"https://profiles.ihe.net/ITI/mCSD/StructureDefinition/IHE.mCSD.Endpoint"},
			},
			Status: fhir.EndpointStatusActive,
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
