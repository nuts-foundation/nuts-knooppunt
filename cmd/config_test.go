package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_Default(t *testing.T) {
	config, err := LoadConfig()
	require.NoError(t, err)

	// Should have default values
	assert.True(t, config.Nuts.Enabled)
	assert.Equal(t, "", config.MCSDAdmin.FHIRBaseURL)
}

func TestLoadConfig_FromYAML(t *testing.T) {
	// Create config directory and file
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	err := os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	yamlContent := `
mcsd:
  rootdirectories:
    "test-org":
      fhirbaseurl: "https://test.example.org/fhir"
  localdirectory:
    fhirbaseurl: "http://localhost:9090/fhir"

mcsdadmin:
  fhirbaseurl: "http://localhost:9090/fhir"

nuts:
  enabled: false
`

	configFile := filepath.Join(configDir, "knooppunt.yml")
	err = os.WriteFile(configFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// Change to temp directory so config/knooppunt.yml is found
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	config, err := LoadConfig()
	require.NoError(t, err)

	// Check loaded values
	assert.False(t, config.Nuts.Enabled)
	assert.Equal(t, "http://localhost:9090/fhir", config.MCSDAdmin.FHIRBaseURL)
	assert.Equal(t, "http://localhost:9090/fhir", config.MCSD.LocalDirectory.FHIRBaseURL)

	// Check map values
	require.Contains(t, config.MCSD.RootDirectories, "test-org")
	assert.Equal(t, "https://test.example.org/fhir", config.MCSD.RootDirectories["test-org"].FHIRBaseURL)
}

func TestLoadConfig_FromEnvironmentVariables(t *testing.T) {
	// Set environment variables
	t.Setenv("KNPT_NUTS_ENABLED", "false")
	t.Setenv("KNPT_MCSDADMIN_FHIRBASEURL", "http://env-test:8080/fhir")

	config, err := LoadConfig()
	require.NoError(t, err)

	// Environment variables should override defaults
	assert.False(t, config.Nuts.Enabled)
	assert.Equal(t, "http://env-test:8080/fhir", config.MCSDAdmin.FHIRBaseURL)
}

func TestLoadConfig_EnvOverridesYAML(t *testing.T) {
	// Create config directory and file
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	err := os.MkdirAll(configDir, 0755)
	require.NoError(t, err)

	yamlContent := `
nuts:
  enabled: true
mcsdadmin:
  fhirbaseurl: "http://yaml:8080/fhir"
`

	configFile := filepath.Join(configDir, "knooppunt.yml")
	err = os.WriteFile(configFile, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// Change to temp directory so config/knooppunt.yml is found
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Set environment variables to override YAML
	t.Setenv("KNPT_NUTS_ENABLED", "false")
	t.Setenv("KNPT_MCSDADMIN_FHIRBASEURL", "http://env:8080/fhir")

	config, err := LoadConfig()
	require.NoError(t, err)

	// Environment should override YAML
	assert.False(t, config.Nuts.Enabled)                                  // env override
	assert.Equal(t, "http://env:8080/fhir", config.MCSDAdmin.FHIRBaseURL) // env override
}
