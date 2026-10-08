package lrza

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

const hospitalPlataanID = "8ae34d4d-89f5-526f-a289-4139de4edd4f"

// HospitalPlataan is Ziekenhuis De Plataan's mCSD Organization resource, as
// it appears in the central addressing directory.
func HospitalPlataan() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr(hospitalPlataanID),
		Meta: &fhir.Meta{
			Profile: []string{organizationProfile},
		},
		Name:       to.Ptr("Ziekenhuis De Plataan"),
		Identifier: []fhir.Identifier{uraIdentifier(plataan.URA)},
		Type:       sbiType("8610", "Hospital activities"),
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

// HospitalPlataanEndpoints are De Plataan's FHIR and authorization-server
// endpoints, published directly in the central addressing directory. The FHIR
// endpoint's payloadType lists the categories De Plataan's patients tenant
// actually holds for the demo pool (see test/testdata/README.md).
func HospitalPlataanEndpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("cb75e784-b658-5a2f-acd7-6d513f9b0b43"),
			Address: plataan.EndpointAddress(),
			Meta: &fhir.Meta{
				Profile: []string{endpointProfile},
			},
			Status:               fhir.EndpointStatusActive,
			ConnectionType:       fhirRESTConnection(),
			ManagingOrganization: managedBy(hospitalPlataanID),
			PayloadType:          dataCategories("Patient", "Condition", "MedicationRequest"),
			PayloadMimeType:      []string{fhirR4MimeType},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-01T00:00:00Z"),
			},
		},
		{
			Id:      to.Ptr("7f2a6c94-51db-4e33-8a0c-9d4e17b3f605"),
			Address: plataan.AuthorizationServerAddress(),
			Meta: &fhir.Meta{
				Profile: []string{endpointProfile},
			},
			Status:               fhir.EndpointStatusActive,
			ConnectionType:       oauthNutsConnection(),
			ManagingOrganization: managedBy(hospitalPlataanID),
			PayloadType:          noPayload(),
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-01T00:00:00Z"),
			},
		},
	}
}
