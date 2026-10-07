package lrza

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const careHomeSunflowerID = "e5909595-767e-41c1-9b00-a23ddf33e5d1"

// CareHomeSunflower is Zorgcentrum De Zonnebloem's (Care Home Sunflower's)
// mCSD Organization resource, as it appears in the central addressing
// directory.
func CareHomeSunflower() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr(careHomeSunflowerID),
		Meta: &fhir.Meta{
			Profile: []string{organizationProfile},
		},
		Name:       to.Ptr("Sunflower Care Home"),
		Identifier: []fhir.Identifier{uraIdentifier(sunflower.URA)},
		Type:       sbiType("8710", "Residential nursing care activities"),
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
// The FHIR endpoint's payloadType lists the nl-gf-data-categories-cs categories
// Sunflower's patients tenant actually holds for the demo pool (see
// test/testdata/README.md).
func CareHomeSunflowerEndpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("f8a9c2d1-4567-489a-bcde-123456789abc"),
			Address: sunflower.EndpointAddress(),
			Meta: &fhir.Meta{
				Profile: []string{endpointProfile},
			},
			Status:               fhir.EndpointStatusActive,
			ConnectionType:       fhirRESTConnection(),
			ManagingOrganization: managedBy(careHomeSunflowerID),
			PayloadType:          dataCategories("Patient", "AllergyIntolerance", "MedicationRequest", "Condition"),
			PayloadMimeType:      []string{fhirR4MimeType},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-01T00:00:00Z"),
			},
		},
		{
			Id:      to.Ptr("3c1f0b6e-2d84-4a15-9f77-6b0e2a5c8d31"),
			Address: sunflower.AuthorizationServerAddress(),
			Meta: &fhir.Meta{
				Profile: []string{endpointProfile},
			},
			Status:               fhir.EndpointStatusActive,
			ConnectionType:       oauthNutsConnection(),
			ManagingOrganization: managedBy(careHomeSunflowerID),
			PayloadType:          noPayload(),
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
				Profile: []string{healthcareServiceProfile},
			},
			Identifier: []fhir.Identifier{
				{
					Use:      to.Ptr(fhir.IdentifierUseOfficial),
					System:   to.Ptr("http://zonnebloem.example.org/fhir/NamingSystem/healthcareservice"),
					Value:    to.Ptr("de-vlinder"),
					Assigner: custodian(URANamingSystem, sunflower.URA),
				},
			},
			Active:     to.Ptr(true),
			ProvidedBy: &fhir.Reference{Reference: to.Ptr("Organization/" + careHomeSunflowerID), Type: to.Ptr("Organization")},
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
