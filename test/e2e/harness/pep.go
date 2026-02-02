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
	FHIRBasePath              string // incoming path clients use, e.g. "/fhir"
	FHIRUpstreamPath          string // path on backend FHIR server, e.g. "/fhir/DEFAULT" (defaults to FHIRBasePath)
	KnooppuntPDPHost          string
	KnooppuntPDPPort          string
	NutsNodeHost              string
	NutsNodePort              string
	DataHolderOrganizationURA string
	DataHolderFacilityType    string
}

// PEPContainerResult contains the PEP URL and container for additional operations
type PEPContainerResult struct {
	URL       *url.URL
	Container testcontainers.Container
}

// StartPEPContainer starts the PEP container and returns the URL and container.
func StartPEPContainer(t *testing.T, config PEPConfig) PEPContainerResult {
	t.Helper()
	ctx := t.Context()

	env := map[string]string{
		"FHIR_BACKEND_HOST":            config.FHIRBackendHost,
		"FHIR_BACKEND_PORT":            config.FHIRBackendPort,
		"FHIR_BASE_PATH":               config.FHIRBasePath,
		"FHIR_UPSTREAM_PATH":           config.FHIRUpstreamPath,
		"KNOOPPUNT_PDP_HOST":           config.KnooppuntPDPHost,
		"KNOOPPUNT_PDP_PORT":           config.KnooppuntPDPPort,
		"NUTS_NODE_HOST":               config.NutsNodeHost,
		"NUTS_NODE_INTERNAL_PORT":      config.NutsNodePort,
		"DATA_HOLDER_ORGANIZATION_URA": config.DataHolderOrganizationURA,
		"DATA_HOLDER_FACILITY_TYPE":    config.DataHolderFacilityType,
	}

	pepReq := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../../../pep/nginx",
			Dockerfile: "Dockerfile",
		},
		ExposedPorts: []string{"8080/tcp"},
		Env:          env,
		WaitingFor:   wait.ForHTTP("/health").WithPort("8080"),
		HostConfigModifier: func(hostConfig *container.HostConfig) {
			// Map host.docker.internal to host gateway
			// On macOS/Windows (Docker Desktop), this is already available
			// On Linux, this adds it to /etc/hosts which Docker's DNS can resolve
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
	host, err := pepContainer.Host(ctx)
	require.NoError(t, err)
	mappedPort, err := pepContainer.MappedPort(ctx, "8080")
	require.NoError(t, err)
	u := &url.URL{
		Scheme: "http",
		Host:   host + ":" + mappedPort.Port(),
	}
	return PEPContainerResult{URL: u, Container: pepContainer}
}
