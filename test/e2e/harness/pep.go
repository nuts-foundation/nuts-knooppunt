package harness

import (
	"net/url"
	"runtime"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// defaultDNSResolver returns the appropriate DNS resolver for the current platform.
// Docker Desktop (macOS/Windows) uses a different DNS server than Linux.
func defaultDNSResolver() string {
	switch runtime.GOOS {
	case "darwin", "windows":
		// Docker Desktop DNS server
		return "192.168.65.7 8.8.8.8"
	default:
		// Docker embedded DNS for Linux containers
		return "127.0.0.11 8.8.8.8"
	}
}

type PEPConfig struct {
	FHIRBackendHost           string
	FHIRBackendPort           string
	FHIRBasePath              string // e.g. "/fhir" or "/fhir/DEFAULT"
	KnooppuntPDPHost          string
	KnooppuntPDPPort          string
	NutsNodeHost              string
	NutsNodePort              string
	DataHolderOrganizationURA string
	DataHolderFacilityType    string
	// Optional settings (leave empty for defaults)
	DNSResolver string // DNS resolver for ngx.fetch (e.g., "192.168.65.7 8.8.8.8")
}

// PEPContainerResult contains the PEP URL and container for additional operations
type PEPContainerResult struct {
	URL       *url.URL
	Container testcontainers.Container
}

// StartPEPContainer starts the PEP container and returns the URL and container
// Use this when you need access to the container (e.g., for logs). Otherwise use startPEP.
func StartPEPContainer(t *testing.T, config PEPConfig) PEPContainerResult {
	t.Helper()
	ctx := t.Context()

	env := map[string]string{
		"FHIR_BACKEND_HOST":            config.FHIRBackendHost,
		"FHIR_BACKEND_PORT":            config.FHIRBackendPort,
		"FHIR_BASE_PATH":               config.FHIRBasePath,
		"KNOOPPUNT_PDP_HOST":           config.KnooppuntPDPHost,
		"KNOOPPUNT_PDP_PORT":           config.KnooppuntPDPPort,
		"NUTS_NODE_HOST":               config.NutsNodeHost,
		"NUTS_NODE_INTERNAL_PORT":      config.NutsNodePort,
		"DATA_HOLDER_ORGANIZATION_URA": config.DataHolderOrganizationURA,
		"DATA_HOLDER_FACILITY_TYPE":    config.DataHolderFacilityType,
	}

	// Set DNS resolver (required for ngx.fetch in njs)
	// Use provided value or detect platform-appropriate default
	dnsResolver := config.DNSResolver
	if dnsResolver == "" {
		dnsResolver = defaultDNSResolver()
	}
	env["DNS_RESOLVER"] = dnsResolver

	pepReq := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../../../pep/nginx",
			Dockerfile: "Dockerfile",
		},
		ExposedPorts: []string{"8080/tcp"},
		Env:          env,
		WaitingFor:   wait.ForHTTP("/health").WithPort("8080"),
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

// startPEP is the internal function that returns only the URL (used by StartPEP harness)
func startPEP(t *testing.T, config PEPConfig) *url.URL {
	return StartPEPContainer(t, config).URL
}
