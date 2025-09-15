package lrza

import (
	"net/url"

	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

func CareHomeSunflower() fhir.Organization {
	return fhir.Organization{
		Id:   to.Ptr("fce3bc5d-0cca-41ed-8072-4734fbac9dcf"),
		Name: to.Ptr("Sunflower Care Home"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr("00000020"),
			},
		},
		Endpoint: []fhir.Reference{
			{
				Reference: to.Ptr("Endpoint/cadbb0ba-0cf0-4f4e-8ee2-5a48a9fae724"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

func CareHomeSunflowerEndpoints(hapiBaseURL *url.URL) []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("cadbb0ba-0cf0-4f4e-8ee2-5a48a9fae724"),
			Address: hapiBaseURL.JoinPath("sunflower-admin").String(),
			Meta: &fhir.Meta{
				Profile: []string{"https://profiles.ihe.net/ITI/mCSD/StructureDefinition/IHE.mCSD.Endpoint"},
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
