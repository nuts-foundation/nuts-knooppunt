package harness

import (
	"context"
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
		Name:         "fhirstore",
		Image:        "hapiproject/hapi:v8.2.0-2",
		ExposedPorts: []string{"8080/tcp"},
		Networks:     []string{dockerNetworkName},
		Env: map[string]string{
			"hapi.fhir.fhir_version":                                    "R4",
			"hapi.fhir.partitioning.allow_references_across_partitions": "false",
			"hapi.fhir.server_id_strategy":                              "UUID",
			"hapi.fhir.client_id_strategy":                              "ANY",
		},
		WaitingFor: wait.ForHTTP("/fhir/DEFAULT/Account"),
		LogConsumerCfg: &testcontainers.LogConsumerConfig{
			Consumers: []testcontainers.LogConsumer{&testcontainers.StdoutLogConsumer{}},
		},
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			panic(err)
		}
	})
	endpoint, err := container.Endpoint(ctx, "http")
	require.NoError(t, err)
	u, err := url.Parse(endpoint)
	require.NoError(t, err)
	return u.JoinPath("fhir")
}
