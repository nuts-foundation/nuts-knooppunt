package nutsnode

import (
	"context"
	"os"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponent_Start(t *testing.T) {

	t.Run("without config file", func(t *testing.T) {
		_ = test.TempDir(t) // Change to tmp dir to avoid writing to repository root
		t.Setenv("NUTS_STRICTMODE", "false")
		t.Setenv("NUTS_URL", "http://localhost")
		t.Setenv("NUTS_AUTH_CONTRACTVALIDATORS", "dummy")

		cfg := Config{Enabled: true}
		c, err := New(cfg)
		require.NoError(t, err, "component should initialize without config file")

		err = c.Start()
		defer c.Stop(context.Background())
		assert.NoError(t, err, "component should start without config file")
	})

	t.Run("with config file", func(t *testing.T) {
		tmpDir := test.TempDir(t) // Change to tmp dir to avoid writing to repository root

		configDir := tmpDir + "/config"
		_ = os.Mkdir(configDir, 0o755)
		configFile := configDir + "/nuts.yml"

		// Find a free port for the test
		const configContent = `strictmode: false
url: http://localhost
auth:
  contractvalidators:
    - dummy
`
		_ = os.WriteFile(configFile, []byte(configContent), 0o644)

		cfg := Config{Enabled: true}
		c, err := New(cfg)
		require.NoError(t, err, "component should initialize with config file")

		err = c.Start()
		defer c.Stop(context.Background())
		assert.NoError(t, err, "component should start with config file")
	})
}

func TestTracingConfigEnvVars(t *testing.T) {
	t.Run("sets NUTS_TRACING env vars when tracing is configured", func(t *testing.T) {
		_ = test.TempDir(t)
		t.Setenv("NUTS_CONFIGFILE", "") // Clear any config file from previous tests
		t.Setenv("NUTS_STRICTMODE", "false")
		t.Setenv("NUTS_URL", "http://localhost")
		t.Setenv("NUTS_AUTH_CONTRACTVALIDATORS", "dummy")

		cfg := Config{
			Enabled: true,
			TracingConfig: TracingConfig{
				OTLPEndpoint: "jaeger:4318",
				Insecure:     true,
			},
		}
		c, err := New(cfg)
		require.NoError(t, err)

		err = c.Start()
		defer c.Stop(context.Background())
		require.NoError(t, err)

		assert.Equal(t, "jaeger:4318", os.Getenv("NUTS_TRACING_ENDPOINT"))
		assert.Equal(t, "true", os.Getenv("NUTS_TRACING_INSECURE"))
	})

	t.Run("sets NUTS_TRACING_INSECURE to false when insecure is false", func(t *testing.T) {
		_ = test.TempDir(t)
		t.Setenv("NUTS_CONFIGFILE", "") // Clear any config file from previous tests
		t.Setenv("NUTS_STRICTMODE", "false")
		t.Setenv("NUTS_URL", "http://localhost")
		t.Setenv("NUTS_AUTH_CONTRACTVALIDATORS", "dummy")

		cfg := Config{
			Enabled: true,
			TracingConfig: TracingConfig{
				OTLPEndpoint: "http://jaeger:4318",
				Insecure:     false,
			},
		}
		c, err := New(cfg)
		require.NoError(t, err)

		err = c.Start()
		defer c.Stop(context.Background())
		require.NoError(t, err)

		assert.Equal(t, "http://jaeger:4318", os.Getenv("NUTS_TRACING_ENDPOINT"))
		assert.Equal(t, "false", os.Getenv("NUTS_TRACING_INSECURE"))
	})

	t.Run("does not set NUTS_TRACING env vars when endpoint is empty", func(t *testing.T) {
		_ = test.TempDir(t)
		t.Setenv("NUTS_CONFIGFILE", "") // Clear any config file from previous tests
		t.Setenv("NUTS_STRICTMODE", "false")
		t.Setenv("NUTS_URL", "http://localhost")
		t.Setenv("NUTS_AUTH_CONTRACTVALIDATORS", "dummy")
		// Clear any existing tracing env vars
		t.Setenv("NUTS_TRACING_ENDPOINT", "")
		t.Setenv("NUTS_TRACING_INSECURE", "")

		cfg := Config{
			Enabled: true,
			TracingConfig: TracingConfig{
				OTLPEndpoint: "",
			},
		}
		c, err := New(cfg)
		require.NoError(t, err)

		err = c.Start()
		defer c.Stop(context.Background())
		require.NoError(t, err)

		assert.Empty(t, os.Getenv("NUTS_TRACING_ENDPOINT"))
		assert.Empty(t, os.Getenv("NUTS_TRACING_INSECURE"))
	})
}
