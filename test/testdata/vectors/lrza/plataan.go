package lrza

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// HospitalPlataan is Ziekenhuis De Plataan's mCSD Organization resource, as
// it appears in the central addressing directory.
func HospitalPlataan() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr("8ae34d4d-89f5-526f-a289-4139de4edd4f"),
		Meta: &fhir.Meta{
			Profile: []string{"http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-organization"},
		},
		Name: to.Ptr("Ziekenhuis De Plataan"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr(plataan.URA),
			},
		},
		Endpoint: []fhir.Reference{
			{
				Reference: to.Ptr("Endpoint/cb75e784-b658-5a2f-acd7-6d513f9b0b43"),
				Type:      to.Ptr("Endpoint"),
			},
			{
				Reference: to.Ptr("Endpoint/7f2a6c94-51db-4e33-8a0c-9d4e17b3f605"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

// HospitalPlataanEndpoints are De Plataan's FHIR and
// authorization-server endpoints, published directly in the central
// addressing directory. See CareHomeSunflowerEndpoints for the coding
// sources; the FHIR endpoint's payloadType lists the categories De Plataan's
// patients tenant actually holds for the demo pool (see
// test/testdata/README.md).
func HospitalPlataanEndpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("cb75e784-b658-5a2f-acd7-6d513f9b0b43"),
			Address: plataan.EndpointAddress(),
			Meta: &fhir.Meta{
				Profile: []string{"http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ConnectionType: fhir.Coding{
				System: to.Ptr("http://terminology.hl7.org/CodeSystem/endpoint-connection-type"),
				Code:   to.Ptr("hl7-fhir-rest"),
			},
			PayloadType: []fhir.CodeableConcept{
				{Coding: []fhir.Coding{{System: to.Ptr("http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs"), Code: to.Ptr("Patient")}}},
				{Coding: []fhir.Coding{{System: to.Ptr("http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs"), Code: to.Ptr("Condition")}}},
				{Coding: []fhir.Coding{{System: to.Ptr("http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs"), Code: to.Ptr("MedicationRequest")}}},
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-01T00:00:00Z"),
			},
		},
		{
			Id:      to.Ptr("7f2a6c94-51db-4e33-8a0c-9d4e17b3f605"),
			Address: plataan.AuthorizationServerAddress(),
			Meta: &fhir.Meta{
				Profile: []string{"http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ConnectionType: fhir.Coding{
				System: to.Ptr("http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-authorization-server-cs"),
				Code:   to.Ptr("oauth-nuts"),
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-01T00:00:00Z"),
			},
		},
	}
}
