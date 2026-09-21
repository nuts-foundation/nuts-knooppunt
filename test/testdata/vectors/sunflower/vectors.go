package sunflower

import (
	"os"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// EndpointAddressEnvVar overrides the address published in Zonnebloem's mCSD
// Endpoint.
//
// The seeded address is consumed in two incompatible contexts: docker compose,
// where a PEP fronts the data and the address must point at it over the compose
// network, and the e2e harness, where no PEP runs and the address must resolve
// straight to HAPI. Defaulting to the HAPI-direct address keeps the harness
// working unchanged; docker compose sets this variable on the seed.
const EndpointAddressEnvVar = "SEED_ZONNEBLOEM_ENDPOINT_ADDRESS"

// EndpointAddress returns the address to publish in the mCSD Endpoint, and to
// advertise as the organization's FHIR base URL on discovery. See
// EndpointAddressEnvVar for why this is deployment-dependent.
func EndpointAddress() string {
	if v := os.Getenv(EndpointAddressEnvVar); v != "" {
		return v
	}
	return "http://localhost:7050/fhir/sunflower-patients"
}

func AdminHAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "sunflower-admin",
		ID:   5,
	}
}

func PatientsHAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "sunflower-patients",
		ID:   7,
	}
}

func Organization() fhir.Organization {
	return fhir.Organization{
		Id: to.Ptr("e5909595-767e-41c1-9b00-a23ddf33e5d1"),
		Meta: &fhir.Meta{
			Profile: []string{"http://nuts-foundation.github.io/nl-generic-functions-ig/StructureDefinition/nl-gf-organization"},
		},
		Name: to.Ptr("Sunflower Care Home"),
		Identifier: []fhir.Identifier{
			{
				System: to.Ptr("http://fhir.nl/fhir/NamingSystem/ura"),
				Value:  to.Ptr("00000020"),
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

func Endpoints() []fhir.Endpoint {
	return []fhir.Endpoint{
		{
			Id:      to.Ptr("f8a9c2d1-4567-489a-bcde-123456789abc"),
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
			Id:      to.Ptr("3c1f0b6e-2d84-4a15-9f77-6b0e2a5c8d31"),
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

// AuthorizationServerAddress is the authorization server protecting Zonnebloem's
// data, published so a consumer reads it from Addressing instead of building it
// out of the URA. The Nuts subject is the URA, and the address is the one the
// node advertises as its issuer, which it also has to be able to fetch metadata
// from itself: inside the node's own container that is its public listener on
// 8080.
func AuthorizationServerAddress() string {
	if v := os.Getenv(AuthorizationServerEnvVar); v != "" {
		return v
	}
	return "http://localhost:8080/nuts/oauth2/00000020"
}

// AuthorizationServerEnvVar overrides the published authorization server, for a
// deployment whose node is not reachable at the default.
const AuthorizationServerEnvVar = "SEED_ZONNEBLOEM_AUTH_SERVER_ADDRESS"

func Patients() []fhir.Patient {
	return []fhir.Patient{
		{
			Id: to.Ptr("e9772fee-ecfb-41c2-83a2-51e833d43347"),
			Identifier: []fhir.Identifier{
				{
					System: to.Ptr("http://fhir.nl/fhir/NamingSystem/bsn"),
					Value:  to.Ptr("999992193"),
				},
			},
			Name: []fhir.HumanName{
				{
					Given:  []string{"Jan"},
					Family: to.Ptr("Jansen"),
				},
			},
		},
	}
}

func AdminResources() []fhir.HasId {
	var resources []fhir.HasId
	for _, endpoint := range Endpoints() {
		resources = append(resources, to.Ptr(endpoint))
	}
	resources = append(resources, to.Ptr(Organization()))
	return resources
}

func PatientsResources() []fhir.HasId {
	var resources []fhir.HasId
	for _, patient := range Patients() {
		resources = append(resources, to.Ptr(patient))
	}
	return resources
}
