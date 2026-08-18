// Package seed holds the parts of the local seed that are shared between the
// data seed (test/testdata) and the credential seed (test/testdata/cmd/credentials),
// which run as separate containers in docker compose.
package seed

import (
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/plataan"
	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/sunflower"
)

// DiscoveryServiceID is the Nuts discovery service the demo organizations
// register on. It must match a definition in config/discovery/.
const DiscoveryServiceID = "bgz-test"

// DIDDirEnvVar names a directory used to hand each created subject's DID from
// the data seed to the credential issuers, as "<Key>.did". did:web DIDs embed a
// fresh UUID per subject creation, so an X509Credential — which binds
// credentialSubject.id to the DID — cannot be generated ahead of time and
// committed; it has to be issued against the DID that actually exists.
//
// Unset (as in the e2e harness, which issues credentials in-process) means
// "don't write".
const DIDDirEnvVar = "SEED_DID_DIR"

// Organization is a demo organization that participates in the authorization
// chain: it has a Nuts subject, an X509Credential minted from a test UZI
// certificate, and a discovery registration.
type Organization struct {
	// Key is the short name used for per-organization files (certificates in
	// test/e2e/pep/certs, and the .did/.jwt handoff files).
	Key string
	// Subject is the Nuts subject name, which is the organization's URA.
	Subject string
	// FHIRBaseURL is advertised on discovery as where this organization serves
	// FHIR. It is deployment-dependent: in compose it points at the
	// organization's PEP, so a resolved address leads through the authorization
	// chain rather than around it.
	FHIRBaseURL string
}

// Organizations returns the demo organizations, in seed order.
func Organizations() []Organization {
	return []Organization{
		// Ziekenhuis De Plataan (consumer/requester) — revived per E5.
		{
			Key:         "plataan",
			Subject:     plataan.URA,
			FHIRBaseURL: plataan.EndpointAddress(),
		},
		// Zorgcentrum De Zonnebloem (source / data holder).
		{
			Key:         "zonnebloem",
			Subject:     *sunflower.Organization().Identifier[0].Value,
			FHIRBaseURL: sunflower.EndpointAddress(),
		},
	}
}
