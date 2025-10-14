package lrza

import (
	"net/url"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func Care2Cure() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr("ef860868-b886-4459-aa87-216955c05289"),
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
				Reference: to.Ptr("Endpoint/08e9e83b-5c3b-43ce-be6f-e0ede8975615"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

func Care2CureEndpoints(hapiBaseURL *url.URL) []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("08e9e83b-5c3b-43ce-be6f-e0ede8975615"),
			Address: hapiBaseURL.JoinPath("care2cure-admin").String(),
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
				Start: to.Ptr("2025-01-01T00:00:00Z"),
			},
		},
	}
}
