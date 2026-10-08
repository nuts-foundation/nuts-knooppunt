package lrza

import (
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Care2CureURA is Care2Cure Hospital's URA number.
const Care2CureURA = "00000030"

const care2CureID = "a3e4080d-8d53-4e53-bfbc-564e85158649"

// Care2Cure is Care2Cure Hospital's mCSD Organization resource, as it appears
// in the central addressing directory. Care2Cure is a future-scenario
// placeholder organization (see test/testdata/README.md) with no demo
// patient data of its own.
func Care2Cure() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr(care2CureID),
		Meta: &fhir.Meta{
			Profile: []string{organizationProfile},
		},
		Name:       to.Ptr("Care2Cure Hospital"),
		Identifier: []fhir.Identifier{uraIdentifier(Care2CureURA)},
		Type:       sbiType("8610", "Hospital activities"),
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
				Profile: []string{endpointProfile},
			},
			Status:               fhir.EndpointStatusActive,
			ConnectionType:       fhirRESTConnection(),
			ManagingOrganization: managedBy(care2CureID),
			PayloadType:          dataCategories("CareServiceEntities"),
			PayloadMimeType:      []string{fhirR4MimeType},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-02T00:00:00Z"),
			},
		},
	}
}
