package harness

import (
	"net/url"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type PEPConfig struct {
	FHIRBackendHost           string
	FHIRBackendPort           string
	FHIRBasePath              string // e.g. "/fhir" or "/fhir/DEFAULT"
	KnooppuntPDPHost          string
	KnooppuntPDPPort          string
	DataHolderOrganizationURA string
	DataHolderFacilityType    string
	RequestingFacilityType    string
	PurposeOfUse              string
}

func startPEP(t *testing.T, config PEPConfig) *url.URL {
	t.Helper()
	ctx := t.Context()

	pepReq := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../../../pep/nginx",
			Dockerfile: "Dockerfile",
		},
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"FHIR_BACKEND_HOST":            config.FHIRBackendHost,
			"FHIR_BACKEND_PORT":            config.FHIRBackendPort,
			"FHIR_BASE_PATH":               config.FHIRBasePath,
			"KNOOPPUNT_PDP_HOST":           config.KnooppuntPDPHost,
			"KNOOPPUNT_PDP_PORT":           config.KnooppuntPDPPort,
			"DATA_HOLDER_ORGANIZATION_URA": config.DataHolderOrganizationURA,
			"DATA_HOLDER_FACILITY_TYPE":    config.DataHolderFacilityType,
			"REQUESTING_FACILITY_TYPE":     config.RequestingFacilityType,
			"PURPOSE_OF_USE":               config.PurposeOfUse,
		},
		WaitingFor: wait.ForHTTP("/health").WithPort("8080"),
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			// Map host.docker.internal to host gateway for Linux (GitHub Actions)
			// On macOS/Windows (Docker Desktop), host.docker.internal already exists
			// On Linux, this maps it to the bridge gateway IP automatically
			hostConfig.ExtraHosts = []string{"host.docker.internal:host-gateway"}
		},
	}

	pepContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: pepReq,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		pepContainer.Terminate(ctx)
	})

	// Get PEP endpoint
	endpoint, err := pepContainer.Endpoint(ctx, "http")
	require.NoError(t, err)
	u, err := url.Parse(endpoint)
	require.NoError(t, err)
	return u
}
