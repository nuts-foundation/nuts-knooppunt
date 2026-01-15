package bundles

//go:generate go run generate_bundles/main.go

import (
	"embed"
	"fmt"
	"strings"
)

// Embedded policy bundles directory - embeds all .tar.gz files in the bundles directory
//
//go:embed *.tar.gz
var bundlesFS embed.FS

// BundleMap maps scope names to their embedded bundles
// This is populated lazily when first accessed
var BundleMap map[string][]byte

// init initializes the BundleMap by reading all bundles from the embedded filesystem
func init() {
	BundleMap = make(map[string][]byte)

	// Read all bundle files from the embedded filesystem
	entries, err := bundlesFS.ReadDir(".")
	if err != nil {
		panic(fmt.Sprintf("failed to read bundles directory: %v", err))
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}

		// Extract scope name from filename (remove .tar.gz extension)
		scope := strings.TrimSuffix(entry.Name(), ".tar.gz")

		// Read the bundle file
		bundleData, err := bundlesFS.ReadFile(entry.Name())
		if err != nil {
			panic(fmt.Sprintf("failed to read bundle %s: %v", entry.Name(), err))
		}

		BundleMap[scope] = bundleData
	}
}
