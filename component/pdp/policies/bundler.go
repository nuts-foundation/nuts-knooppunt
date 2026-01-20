package policies

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed */*
var policies embed.FS

// bundles maps scope names to their embedded bundles
// This is populated lazily when first accessed
var bundles map[string][]byte

func Bundles(ctx context.Context) (map[string][]byte, error) {
	if bundles == nil {
		if err := initBundles(ctx); err != nil {
			return nil, fmt.Errorf("failed to initialize OPA bundles: %w", err)
		}
	}
	return bundles, nil
}

func initBundles(ctx context.Context) error {
	bundleDir, err := generateBundles(ctx)
	if err != nil {
		return err
	}
	return readBundles(bundleDir)
}

func generateBundles(ctx context.Context) (string, error) {
	slog.DebugContext(ctx, "Generating Open Policy Agent bundles...")
	bundlesDir, err := os.MkdirTemp("", "pdp-bundles-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary bundles directory: %w", err)
	}

	// Find all policy directories (subdirectories of policies/)
	entries, err := policies.ReadDir(".")
	if err != nil {
		return "", fmt.Errorf("failed to read policies directory: %w", err)
	}

	for _, entry := range entries {
		// Skip non-directories and the bundles directory
		if !entry.IsDir() || entry.Name() == "bundles" {
			continue
		}

		policyName := entry.Name()
		policyPath := filepath.Join(policyName, "policy.rego")

		// Check if policy.rego exists in this directory
		if h, err := policies.Open(policyPath); err != nil {
			return "", fmt.Errorf("policy.rego not found in %s", policyName)
		} else {
			_ = h.Close()
		}
		bundlePath := filepath.Join(bundlesDir, policyName+".tar.gz")
		if err := generateBundle(policyName, bundlePath, policyName); err != nil {
			return "", fmt.Errorf("failed to generate bundle for %s: %w", policyName, err)
		}
	}
	return bundlesDir, nil
}

func generateBundle(policyDir, bundlePath, policyName string) error {
	// Create the bundle file
	file, err := os.Create(bundlePath)
	if err != nil {
		return fmt.Errorf("failed to create bundle file: %w", err)
	}
	defer file.Close()

	// Create gzip writer
	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	// Create tar writer
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Create manifest
	manifest := map[string]interface{}{
		"revision": fmt.Sprintf("%d", time.Now().Unix()),
		"roots":    []string{policyName},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Write .manifest file
	if err := tarWriter.WriteHeader(&tar.Header{
		Name:    filepath.Join(policyName, ".manifest"),
		Mode:    0644,
		Size:    int64(len(manifestBytes)),
		ModTime: time.Now(),
	}); err != nil {
		return fmt.Errorf("failed to write manifest header: %w", err)
	}
	if _, err := tarWriter.Write(manifestBytes); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	// Read all files in the policy directory
	entries, err := policies.ReadDir(policyDir)
	if err != nil {
		return fmt.Errorf("failed to read policy directory: %w", err)
	}

	// Add all files from the policy directory to the bundle
	for _, entry := range entries {
		// Skip subdirectories
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		filePath := filepath.Join(policyDir, fileName)

		// Read file content
		fileContent, err := policies.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", fileName, err)
		}

		// Write file to tar
		if err := tarWriter.WriteHeader(&tar.Header{
			Name:    filepath.Join(policyName, fileName),
			Mode:    0644,
			Size:    int64(len(fileContent)),
			ModTime: time.Now(),
		}); err != nil {
			return fmt.Errorf("failed to write header for %s: %w", fileName, err)
		}
		if _, err := tarWriter.Write(fileContent); err != nil {
			return fmt.Errorf("failed to write content for %s: %w", fileName, err)
		}
	}

	return nil
}

func readBundles(bundleDir string) error {
	bundles = make(map[string][]byte)

	// Read all bundle files from the embedded filesystem
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		return fmt.Errorf("failed to read policies directory: %w", err)
	}

	for _, entry := range entries {
		entryPath := filepath.Join(bundleDir, entry.Name())
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}

		// Extract scope name from filename (remove .tar.gz extension)
		scope := strings.TrimSuffix(entry.Name(), ".tar.gz")

		// Read the bundle file
		bundleData, err := os.ReadFile(entryPath)
		if err != nil {
			return fmt.Errorf("failed to read bundle %s: %w", entryPath, err)
		}

		bundles[scope] = bundleData
	}
	return nil
}
