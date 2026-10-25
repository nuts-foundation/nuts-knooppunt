package harness

import (
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test/testdata/vectors/lrza"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// defaultHAPIImage is the published HAPI image with the mock interceptors (mock-components/hapi).
const defaultHAPIImage = "ghcr.io/nuts-foundation/fake-nvi:main"

// hapiImageEnv overrides the HAPI image, so a change to the mock interceptors is tested before it is
// published: build it (docker build -t knooppunt-hapi:local -f mock-components/hapi/Dockerfile
// mock-components) and run the tests with KNPT_HAPI_IMAGE=knooppunt-hapi:local. CI does the same.
const hapiImageEnv = "KNPT_HAPI_IMAGE"

// hapiImage is the HAPI image to run, and the name of its (reused) container. An overridden image gets a
// container of its own, so switching images never reuses a container running the other one.
func hapiImage() (image, containerName string) {
	const reusedContainerName = "knooppunt-unittest-fhirstore"
	image = os.Getenv(hapiImageEnv)
	if image == "" {
		return defaultHAPIImage, reusedContainerName
	}
	return image, reusedContainerName + "-" + strings.Trim(nonNameCharacters.ReplaceAllString(image, "-"), "-")
}

var nonNameCharacters = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func startHAPI(t *testing.T, dockerNetworkName string) *url.URL {
	image, containerName := hapiImage()
	t.Logf("Starting HAPI FHIR server (%s)...", image)
	ctx := t.Context()
	req := testcontainers.ContainerRequest{
		Name:         containerName,
		Image:        image,
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
			// Match docker-compose.yml: the reset path deletes referenced
			// resources with _cascade=delete, which HAPI rejects unless this is
			// on. Without it the harness cannot reproduce compose's behaviour.
			"hapi.fhir.allow_cascading_deletes": "true",
			// Match docker-compose.yml: mCSD directories can legitimately receive a resource before a
			// resource it references (e.g. Organization <-> Endpoint are circular, per
			// https://minvws.github.io/generiekefuncties-docs/en/csd.html). Without this, writes with
			// an unresolved reference are rejected.
			"hapi.fhir.enforce_referential_integrity_on_write": "false",
			// Match docker-compose.yml: without a placeholder, HAPI never indexes a reference whose
			// target doesn't exist yet, so _include and chained search miss it even after the target
			// arrives. A placeholder gives the link a target that the later write fills in.
			"hapi.fhir.auto_create_placeholder_reference_targets": "true",
			"NVI_AUDIENCE": "nvi",
			// The mock LRZA (mock-components/lrza) takes every request without X-Mock-Caller as this
			// vendor's, as the real LRZA would from the Knooppunt's certificate. The seed grants it the
			// mandate for De Zonnebloem and De Plataan (vectors/lrza/mandates.go).
			"LRZA_CALLER": lrza.VendorCaller(),
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
