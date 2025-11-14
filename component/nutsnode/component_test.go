package nutsnode

import (
	"context"
	"os"
	"testing"

	"github.com/nuts-foundation/nuts-knooppunt/test"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComponent_Start(t *testing.T) {
	// Ensure zerolog.DefaultContextLogger is initialized to avoid nil pointer panic
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	zerolog.DefaultContextLogger = &log.Logger

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
