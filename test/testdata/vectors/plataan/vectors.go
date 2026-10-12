// Package plataan holds the mCSD + FHIR vectors for Ziekenhuis De Plataan
// (URA 00000010), the consumer/requester hospital in the GF Sandbox BGZ demo.
// It mirrors the sunflower package (the source care home) so the two sides of
// the exchange are modelled the same way.
package plataan

import (
	"os"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/hapi"
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

// PatientsHAPITenant is the tenant holding De Plataan's own (hospital-side)
// clinical FHIR resources for the demo pool.
func PatientsHAPITenant() hapi.Tenant {
	return hapi.Tenant{
		Name: "plataan-patients",
		ID:   10,
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
