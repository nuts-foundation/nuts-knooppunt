package harness

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/google/uuid"
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

// Test_HAPI_IndexesReferencesToTargetsWrittenLater verifies that a reference written before its target
// is still found by _include once the target arrives. Accepting the write is not enough for that: HAPI
// only indexes a reference whose target exists, so this relies on the placeholder it creates for the
// missing target (auto_create_placeholder_reference_targets), which the later write fills in.
//
// The LRZA sync relies on this: Organizations sync before the Endpoints they reference, in separate
// transactions, and consumers look up an organization's endpoints with _include=Organization:endpoint.
func Test_HAPI_IndexesReferencesToTargetsWrittenLater(t *testing.T) {
	dockerNetwork, err := createDockerNetwork(t)
	require.NoError(t, err)
	hapiBaseURL := startHAPI(t, dockerNetwork.Name)

	client := fhirclient.New(hapiBaseURL.JoinPath("DEFAULT"), http.DefaultClient, &fhirclient.Config{UsePostSearch: false})
	endpointID := uuid.NewString()
	organizationID := uuid.NewString()

	require.NoError(t, client.UpdateWithContext(t.Context(), "Organization/"+organizationID, fhir.Organization{
		Id:       to.Ptr(organizationID),
		Endpoint: []fhir.Reference{{Reference: to.Ptr("Endpoint/" + endpointID)}},
	}, nil))

	var placeholder map[string]any
	require.NoError(t, client.ReadWithContext(t.Context(), "Endpoint/"+endpointID, &placeholder))
	require.Equal(t, []any{map[string]any{
		"url":          "http://hapifhir.io/fhir/StructureDefinition/resource-placeholder",
		"valueBoolean": true,
	}}, placeholder["extension"], "the missing target must be created as a placeholder")

	require.NoError(t, client.UpdateWithContext(t.Context(), "Endpoint/"+endpointID, fhir.Endpoint{
		Id:      to.Ptr(endpointID),
		Status:  fhir.EndpointStatusActive,
		Address: "https://example.com/fhir",
	}, nil))

	var bundle fhir.Bundle
	require.NoError(t, client.SearchWithContext(t.Context(), "Organization", url.Values{
		"_id":      []string{organizationID},
		"_include": []string{"Organization:endpoint"},
	}, &bundle))
	var endpoints []fhir.Endpoint
	for _, entry := range bundle.Entry {
		var endpoint fhir.Endpoint
		if err := json.Unmarshal(entry.Resource, &endpoint); err == nil && endpoint.Id != nil && *endpoint.Id == endpointID {
			endpoints = append(endpoints, endpoint)
		}
	}
	require.Len(t, endpoints, 1, "_include must find the endpoint written after the organization")
	require.Equal(t, "https://example.com/fhir", endpoints[0].Address, "the placeholder must have been filled in by the later write")
}
