package lrza

import (
	"net/url"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// HospitalPlataan is Ziekenhuis De Plataan's Organization as registered in the
// LRZa root directory (URA 00000010). Its endpoint points at De Plataan's own
// admin directory, which the mCSD update process then syncs into the query
// directory.
func HospitalPlataan() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr("ea7d76a8-d380-56d4-add9-f27225aa71c6"),
		Meta: &fhir.Meta{
			Profile: []string{"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization"},
		},
		Name: to.Ptr("Ziekenhuis De Plataan"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr("00000010"),
			},
		},
		Endpoint: []fhir.Reference{
			{
				Reference: to.Ptr("Endpoint/ba13e2aa-7f8e-548d-8a68-31f6adc31bbf"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

func HospitalPlataanEndpoints(hapiBaseURL *url.URL) []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("ba13e2aa-7f8e-548d-8a68-31f6adc31bbf"),
			Address: hapiBaseURL.JoinPath("plataan-admin").String(),
			Meta: &fhir.Meta{
				Profile: []string{"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			PayloadType: []fhir.CodeableConcept{
				{
					Coding: []fhir.Coding{
						{
							System: to.Ptr("http://nuts-foundation.github.io/nl-generic-functions-ig/CodeSystem/nl-gf-data-exchange-capabilities"),
							Code:   to.Ptr("http://nuts-foundation.github.io/nl-generic-functions-ig/CapabilityStatement/nl-gf-admin-directory-update-client"),
						},
					},
				},
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-05-01T00:00:00Z"),
			},
		},
	}
}
