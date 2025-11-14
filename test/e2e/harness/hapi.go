package harness

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func startHAPI(t *testing.T, dockerNetworkName string) *url.URL {
	t.Log("Starting HAPI FHIR server...")
	ctx := t.Context()
	req := testcontainers.ContainerRequest{
		Name:         "knooppunt-unittest-fhirstore",
		Image:        "ghcr.io/nuts-foundation/fake-nvi:latest",
		ExposedPorts: []string{"8080/tcp"},
		//Networks:     []string{dockerNetworkName},
		Env: map[string]string{
			"hapi.fhir.fhir_version":                                    "R4",
			"hapi.fhir.partitioning.allow_references_across_partitions": "false",
			"hapi.fhir.server_id_strategy":                              "UUID",
			"hapi.fhir.client_id_strategy":                              "ANY",
			"hapi.fhir.store_meta_source_information":                   "SOURCE_URI",
			// Enable system-wide $expunge operation for test data cleanup
			"hapi.fhir.delete_expunge_enabled": "true",
			"hapi.fhir.allow_multiple_delete":  "true",
			"NVI_AUDIENCE":                     "nvi",
			"NVI_TENANT":                       "nvi",
		},
		WaitingFor: wait.ForHTTP("/fhir/DEFAULT/Account"),
		LogConsumerCfg: &testcontainers.LogConsumerConfig{
			Consumers: []testcontainers.LogConsumer{&testcontainers.StdoutLogConsumer{}},
		},
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Reuse:            true,
	})
	require.NoError(t, err)

	endpoint, err := container.Endpoint(ctx, "http")
	require.NoError(t, err)
	u, err := url.Parse(endpoint)
	require.NoError(t, err)
	return u.JoinPath("fhir")
}
