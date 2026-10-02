package harness

import (
	"net/http"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/stretchr/testify/require"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Test_HAPI_AcceptsUnresolvedReferences verifies that the hapi-fhir configuration this harness starts
// (mirroring docker-compose.yml) accepts a resource whose reference points at a resource that does not
// exist, rather than rejecting the write with a referential-integrity error.
//
// The LRZA sync relies on this: per https://minvws.github.io/generiekefuncties-docs/en/csd.html, a
// Local Replica SHALL accept resources whose references cannot (yet) be resolved - e.g. Organization
// and Endpoint reference each other, so neither can always be synced before the other.
// enforce_referential_integrity_on_write is a server-wide HAPI setting, not scoped to a single
// partition, so testing it against the DEFAULT partition is representative of every tenant.
func Test_HAPI_AcceptsUnresolvedReferences(t *testing.T) {
	dockerNetwork, err := createDockerNetwork(t)
	require.NoError(t, err)
	hapiBaseURL := startHAPI(t, dockerNetwork.Name)

	client := fhirclient.New(hapiBaseURL.JoinPath("DEFAULT"), http.DefaultClient, &fhirclient.Config{UsePostSearch: false})

	reference := "Endpoint/does-not-exist"
	org := fhir.Organization{
		Name:     to.Ptr("Referential integrity test org"),
		Endpoint: []fhir.Reference{{Reference: &reference}},
	}

	var created fhir.Organization
	err = client.CreateWithContext(t.Context(), org, &created)
	require.NoError(t, err, "HAPI should accept a resource that references a resource it doesn't know about yet")
}
