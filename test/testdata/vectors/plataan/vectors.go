// Package plataan holds the mCSD + FHIR vectors for Ziekenhuis De Plataan
// (URA 00000010), the consumer/requester hospital in the GF Sandbox BGZ demo.
// It mirrors the sunflower package (the source care home) so the two sides of
// the exchange are modelled the same way.
package plataan

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// URA is Ziekenhuis De Plataan's URA number. Previously unused in testdata.
const URA = "00000010"

// AdminHAPITenant is De Plataan's own mCSD admin directory tenant.
func AdminHAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "plataan-admin",
		ID:   9,
	}
}

// PatientsHAPITenant is the tenant holding De Plataan's own (hospital-side)
// clinical FHIR resources for the demo pool.
func PatientsHAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "plataan-patients",
		ID:   10,
	}
}

// Organization is De Plataan's mCSD Organization resource, as it appears in its
// own admin directory.
func Organization() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr("8ae34d4d-89f5-526f-a289-4139de4edd4f"),
		Meta: &fhir.Meta{
			Profile: []string{"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization"},
		},
		Name: to.Ptr("Ziekenhuis De Plataan"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr(URA),
			},
		},
		Endpoint: []fhir.Reference{
			{
				Reference: to.Ptr("Endpoint/cb75e784-b658-5a2f-acd7-6d513f9b0b43"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

// Endpoints are the endpoints published in De Plataan's own admin directory.
// The FHIR endpoint points at De Plataan's patients tenant.
//
// TODO(E4): like the sunflower endpoint, this points straight at HAPI. When E4
// wires retrieval through the PEP and hosted addressing lands, make this
// deployment-aware and point it at De Plataan's PEP.
func Endpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("cb75e784-b658-5a2f-acd7-6d513f9b0b43"),
			Address: "http://localhost:7050/fhir/plataan-patients",
			Meta: &fhir.Meta{
				Profile: []string{"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ConnectionType: fhir.Coding{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/endpoint-connection-type"),
				Code:   to.Ptr("fhir"),
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-01T00:00:00Z"),
			},
		},
	}
}

// AdminResources returns the resources for De Plataan's admin directory tenant
// (Organization + Endpoints), for PUT-by-fixed-id upsert.
func AdminResources() []fhir.HasId {
	var resources []fhir.HasId
	for _, endpoint := range Endpoints() {
		resources = append(resources, to.Ptr(endpoint))
	}
	resources = append(resources, to.Ptr(Organization()))
	return resources
}
