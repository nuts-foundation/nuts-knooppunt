package lrza

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// The mandates. The LRZA only accepts a write by an IT vendor that the care provider authorized, by
// registering an OrganizationAffiliation itself: organization is the care provider,
// participatingOrganization the vendor, code the kind of mandate. The mock LRZA
// (mock-components/lrza) enforces this, so the seed grants the vendor operating the Knooppunt the
// mandate for De Zonnebloem and De Plataan. Care2Cure grants none: tests use it for a care provider that
// hasn't (yet).

const (
	// VendorKVKNamingSystem and VendorKVK identify the IT vendor operating the Knooppunt: ExampleSoft
	// B.V., the vendor of the IG's example mandate. The mock LRZA treats every request as this vendor's
	// (LRZA_CALLER), as the real LRZA would from the Knooppunt's certificate.
	VendorKVKNamingSystem = kvkNamingSystem
	VendorKVK             = "90001234"

	// AuthorizationTypeSystem is the code system of the mandate kinds, and CareProviderAdminMandate the
	// mandate to administer all of a care provider's addressing resources.
	AuthorizationTypeSystem  = "http://fhir.generiekefuncties.nl/CodeSystem/nl-gf-authorization-type-cs"
	CareProviderAdminMandate = "lrza-careprovider-admin"

	organizationAffiliationProfile = "http://fhir.generiekefuncties.nl/csd/StructureDefinition/nl-gf-organizationaffiliation"
)

// VendorCaller is the vendor as the mock LRZA's LRZA_CALLER: a system|value token.
func VendorCaller() string {
	return VendorKVKNamingSystem + "|" + VendorKVK
}

// CareProviderCaller is a care provider as the mock LRZA's X-Mock-Caller: its URA as a system|value token.
func CareProviderCaller(ura string) string {
	return URANamingSystem + "|" + ura
}

// Mandate is the OrganizationAffiliation by which the care provider (organizationID, with its URA)
// grants the vendor the lrza-careprovider-admin mandate, conforming to nl-gf-organizationaffiliation.
func Mandate(id, organizationID, ura string) fhir.OrganizationAffiliation {
	return fhir.OrganizationAffiliation{
		Id: to.Ptr(id),
		Meta: &fhir.Meta{
			Profile: []string{organizationAffiliationProfile},
		},
		Identifier: []fhir.Identifier{{
			Use:      to.Ptr(fhir.IdentifierUseOfficial),
			System:   to.Ptr("urn:ietf:rfc:3986"),
			Value:    to.Ptr("urn:uuid:" + id),
			Assigner: custodian(URANamingSystem, ura),
		}},
		Active:       to.Ptr(true),
		Organization: &fhir.Reference{Reference: to.Ptr("Organization/" + organizationID), Type: to.Ptr("Organization")},
		ParticipatingOrganization: &fhir.Reference{
			Identifier: &fhir.Identifier{System: to.Ptr(VendorKVKNamingSystem), Value: to.Ptr(VendorKVK)},
			Display:    to.Ptr("ExampleSoft B.V."),
		},
		Code: []fhir.CodeableConcept{{Coding: []fhir.Coding{{
			System:  to.Ptr(AuthorizationTypeSystem),
			Code:    to.Ptr(CareProviderAdminMandate),
			Display: to.Ptr("LRZa Care Provider Administration"),
		}}}},
	}
}

// OrganizationAffiliations returns the mandates in the central addressing directory: De Zonnebloem's and
// De Plataan's, none of Care2Cure's.
func OrganizationAffiliations() []fhir.OrganizationAffiliation {
	return []fhir.OrganizationAffiliation{
		Mandate("2b7e3f0a-5c4d-4e8f-9a1b-6c3d2e1f0a9b", careHomeSunflowerID, sunflower.URA),
		Mandate("9d4c1b2a-7e6f-4a3b-8c5d-0e1f2a3b4c5d", hospitalPlataanID, plataan.URA),
	}
}
