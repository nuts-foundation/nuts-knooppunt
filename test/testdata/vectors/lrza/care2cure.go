package lrza

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Care2CureURA is Care2Cure Hospital's URA number.
const Care2CureURA = "00000030"

// Care2Cure is Care2Cure Hospital's mCSD Organization resource, as it appears
// in the central addressing directory. Care2Cure is a future-scenario
// placeholder organization (see test/testdata/README.md) with no demo
// patient data of its own.
func Care2Cure() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr("a3e4080d-8d53-4e53-bfbc-564e85158649"),
		Meta: &fhir.Meta{
			Profile: []string{"http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-organization"},
		},
		Name: to.Ptr("Care2Cure Hospital"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr(Care2CureURA),
			},
		},
		Endpoint: []fhir.Reference{
			{
				Reference: to.Ptr("Endpoint/bce8a799-e6ba-4c06-8a1c-bc052f01a636"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

// Care2CureEndpoints are Care2Cure's published endpoints. Care2Cure has no
// backing service, so its FHIR endpoint is a fixed example address
// rather than an env-var override like Sunflower's/Plataan's, and its
// payloadType is the generic CareServiceEntities category (nl-gf-data-categories-cs)
// rather than a specific clinical category it doesn't actually hold.
func Care2CureEndpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("bce8a799-e6ba-4c06-8a1c-bc052f01a636"),
			Address: "https://example.com/care2curehospital/fhir",
			Meta: &fhir.Meta{
				Profile: []string{"http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ConnectionType: fhir.Coding{
				System: to.Ptr("http://terminology.hl7.org/CodeSystem/endpoint-connection-type"),
				Code:   to.Ptr("hl7-fhir-rest"),
			},
			PayloadType: []fhir.CodeableConcept{
				{Coding: []fhir.Coding{{System: to.Ptr("http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs"), Code: to.Ptr("CareServiceEntities")}}},
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-02T00:00:00Z"),
			},
		},
	}
}
