package lrza

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// CareHomeSunflower is Zorgcentrum De Zonnebloem's (Care Home Sunflower's)
// mCSD Organization resource, as it appears in the central addressing
// directory.
func CareHomeSunflower() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr("e5909595-767e-41c1-9b00-a23ddf33e5d1"),
		Meta: &fhir.Meta{
			Profile: []string{"http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-organization"},
		},
		Name: to.Ptr("Sunflower Care Home"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr(sunflower.URA),
			},
		},
		Endpoint: []fhir.Reference{
			{
				Reference: to.Ptr("Endpoint/f8a9c2d1-4567-489a-bcde-123456789abc"),
				Type:      to.Ptr("Endpoint"),
			},
			{
				Reference: to.Ptr("Endpoint/3c1f0b6e-2d84-4a15-9f77-6b0e2a5c8d31"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

// CareHomeSunflowerEndpoints are Sunflower's FHIR and authorization-server
// endpoints, published directly in the central addressing directory (no more
// pointer to a separate per-org admin tenant).
//
// Codings follow https://build.fhir.org/ig/minvws/generiekefuncties-docs/branches/1.0.0/en/artifacts.html:
// connectionType from the NL GF Connection Types ValueSet
// (nl-gf-connection-types-vs); the FHIR endpoint's payloadType lists the
// nl-gf-data-categories-cs categories Sunflower's patients tenant actually
// holds for the demo pool (see test/testdata/README.md). The authorization
// server endpoint has no documented/canonical payloadType for the oauth-nuts
// connection type, so it's omitted rather than guessed.
func CareHomeSunflowerEndpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("f8a9c2d1-4567-489a-bcde-123456789abc"),
			Address: sunflower.EndpointAddress(),
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
				{Coding: []fhir.Coding{{System: to.Ptr("http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs"), Code: to.Ptr("AllergyIntolerance")}}},
				{Coding: []fhir.Coding{{System: to.Ptr("http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs"), Code: to.Ptr("MedicationRequest")}}},
				{Coding: []fhir.Coding{{System: to.Ptr("http://fhir.generiekefuncties.nl/csd/CodeSystem/nl-gf-data-categories-cs"), Code: to.Ptr("Condition")}}},
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-01T00:00:00Z"),
			},
		},
		{
			Id:      to.Ptr("3c1f0b6e-2d84-4a15-9f77-6b0e2a5c8d31"),
			Address: sunflower.AuthorizationServerAddress(),
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

// CareHomeSunflowerHealthcareServices are Sunflower's services: ward De
// Vlinder, the sublocation the GF Sandbox shows for De Zonnebloem (see
// sandbox/DESIGN.md §5.1), which a FHIR R4 Endpoint alone cannot express.
//
// Conforms to nl-gf-healthcareservice: a custodian-assigned identifier
// (nl-gf-custodianassignedidentifier, assigned by Sunflower's URA), a type from
// nl-gf-service-types-vs, and the organization providing it. It is served
// through Sunflower's FHIR endpoint.
func CareHomeSunflowerHealthcareServices() []fhir.HealthcareService {
	return []fhir.HealthcareService{
		{
			Id: to.Ptr("6d2b9e41-0c7a-4f38-b5e2-8a14c3f7d920"),
			Meta: &fhir.Meta{
				Profile: []string{"http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-healthcareservice"},
			},
			Identifier: []fhir.Identifier{
				{
					Use:    to.Ptr(fhir.IdentifierUseOfficial),
					System: to.Ptr("http://zonnebloem.example.org/fhir/NamingSystem/healthcareservice"),
					Value:  to.Ptr("de-vlinder"),
					Assigner: &fhir.Reference{
						Identifier: &fhir.Identifier{
							Type: &fhir.CodeableConcept{Coding: []fhir.Coding{{
								System: to.Ptr("http://terminology.hl7.org/CodeSystem/provenance-participant-type"),
								Code:   to.Ptr("custodian"),
							}}},
							System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
							Value:  to.Ptr(sunflower.URA),
						},
					},
				},
			},
			Active:     to.Ptr(true),
			ProvidedBy: &fhir.Reference{Reference: to.Ptr("Organization/e5909595-767e-41c1-9b00-a23ddf33e5d1"), Type: to.Ptr("Organization")},
			Type: []fhir.CodeableConcept{{Coding: []fhir.Coding{{
				System:  to.Ptr("http://terminology.hl7.org/CodeSystem/service-type"),
				Code:    to.Ptr("4"),
				Display: to.Ptr("Aged Residential Care"),
			}}}},
			Name: to.Ptr("Afdeling De Vlinder"),
			Endpoint: []fhir.Reference{
				{Reference: to.Ptr("Endpoint/f8a9c2d1-4567-489a-bcde-123456789abc"), Type: to.Ptr("Endpoint")},
			},
		},
	}
}
