package sunflower

import (
	"os"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// URA is Zorgcentrum De Zonnebloem's (Care Home Sunflower's) URA number.
const URA = "00000020"

// EndpointAddressEnvVar overrides the address published in Zonnebloem's mCSD
// Endpoint.
//
// The seeded address is consumed in two incompatible contexts: docker compose,
// where a PEP fronts the data and the address must point at it over the compose
// network, and the e2e harness, where the address must resolve straight to HAPI
// unless a test puts a PEP in front of it. Defaulting to the HAPI-direct address
// keeps the harness working unchanged; docker compose sets this variable on the
// seed, and sandbox/app's full-chain test sets it and seeds again once its PEP
// has a port.
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

func PatientsHAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "sunflower-patients",
		ID:   7,
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
	return "http://localhost:8080/nuts/oauth2/" + URA
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

func PatientsResources() []fhir.HasId {
	var resources []fhir.HasId
	for _, patient := range Patients() {
		resources = append(resources, to.Ptr(patient))
	}
	return resources
}
