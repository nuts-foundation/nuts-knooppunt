// Package plataan holds the mCSD + FHIR vectors for Ziekenhuis De Plataan
// (URA 00000010), the consumer/requester hospital in the GF Sandbox BGZ demo.
// It mirrors the sunflower package (the source care home) so the two sides of
// the exchange are modelled the same way.
package plataan

import (
	"os"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// URA is Ziekenhuis De Plataan's URA number. Previously unused in testdata.
const URA = "00000010"

// EndpointAddressEnvVar overrides the address published in De Plataan's mCSD
// Endpoint. See sunflower.EndpointAddressEnvVar for why the seeded address has
// to be deployment-dependent.
const EndpointAddressEnvVar = "SEED_PLATAAN_ENDPOINT_ADDRESS"

// EndpointAddress returns the address to publish in the mCSD Endpoint, and to
// advertise as the organization's FHIR base URL on discovery.
func EndpointAddress() string {
	if v := os.Getenv(EndpointAddressEnvVar); v != "" {
		return v
	}
	return "http://localhost:7050/fhir/plataan-patients"
}

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
			{
				Reference: to.Ptr("Endpoint/7f2a6c94-51db-4e33-8a0c-9d4e17b3f605"),
				Type:      to.Ptr("Endpoint"),
			},
		},
	}
}

// Endpoints are the endpoints published in De Plataan's own admin directory.
// The FHIR endpoint points at De Plataan's patients tenant, fronted by De
// Plataan's PEP where one is deployed (see EndpointAddressEnvVar).
func Endpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("cb75e784-b658-5a2f-acd7-6d513f9b0b43"),
			Address: EndpointAddress(),
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
		{
			Id:      to.Ptr("7f2a6c94-51db-4e33-8a0c-9d4e17b3f605"),
			Address: AuthorizationServerAddress(),
			Meta: &fhir.Meta{
				Profile: []string{"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-endpoint"},
			},
			Status: fhir.EndpointStatusActive,
			ConnectionType: fhir.Coding{
				System: to.Ptr("http://minvws.github.io/generiekefuncties-docs/CodeSystem/nl-gf-authorization-server-cs"),
				Code:   to.Ptr("oauth-nuts"),
			},
			Period: &fhir.Period{
				Start: to.Ptr("2025-01-01T00:00:00Z"),
			},
		},
	}
}

// AuthorizationServerAddress is the authorization server protecting De Plataan's
// own data, whose Nuts subject is its URA, matching the PEP in front of it
// (DATA_HOLDER_ORGANIZATION_URA in docker-compose.yml).
//
// It is also the server the sandbox asks for its own access token, which reads
// this entry under De Plataan's URA. Which wallet that request is made from is a
// separate choice and a different name: SANDBOX_NUTS_SUBJECT.
func AuthorizationServerAddress() string {
	if v := os.Getenv(AuthorizationServerEnvVar); v != "" {
		return v
	}
	return "http://localhost:8080/nuts/oauth2/" + URA
}

// AuthorizationServerEnvVar overrides the published authorization server, for a
// deployment whose node is not reachable at the default.
const AuthorizationServerEnvVar = "SEED_PLATAAN_AUTH_SERVER_ADDRESS"

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
