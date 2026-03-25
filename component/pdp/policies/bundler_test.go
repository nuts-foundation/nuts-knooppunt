package policies

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadBundles_DuplicatePolicyAfterNormalization(t *testing.T) {
	bundleDir := t.TempDir()

	// Create two bundle files that normalize to the same name after dash-to-underscore + lowercasing
	// e.g. "my-bgz.tar.gz" and "my_bgz.tar.gz" both normalize to "my_bgz"
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "my-bgz.tar.gz"), []byte("bundle1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(bundleDir, "my_bgz.tar.gz"), []byte("bundle2"), 0644))

	// Reset package-level bundles
	bundles = nil
	defer func() { bundles = nil }()

	err := readBundles(bundleDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "my_bgz")
}
