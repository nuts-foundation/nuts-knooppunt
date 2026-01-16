package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	// Get the directory where this generator is located
	// When run via go:generate, the working directory is the bundles package directory
	policiesDir := "../policies"
	bundlesDir := "."

	// Bundles will be written to current directory (bundles/)

	fmt.Println("Generating Open Policy Agent bundles...")

	// Find all policy directories (subdirectories of policies/)
	entries, err := os.ReadDir(policiesDir)
	if err != nil {
		log.Fatalf("Failed to read policies directory: %v", err)
	}

	for _, entry := range entries {
		// Skip non-directories and the bundles directory
		if !entry.IsDir() || entry.Name() == "bundles" {
			continue
		}

		policyName := entry.Name()
		policyDir := filepath.Join(policiesDir, policyName)
		policyPath := filepath.Join(policyDir, "policy.rego")

		// Check if policy.rego exists in this directory
		if _, err := os.Stat(policyPath); os.IsNotExist(err) {
			panic(fmt.Sprintf("policy.rego not found in %s", policyDir))
		}

		bundlePath := filepath.Join(bundlesDir, policyName+".tar.gz")

		fmt.Printf("  Building bundle: %s\n", policyName)

		if err := generateBundle(policyDir, bundlePath, policyName); err != nil {
			log.Fatalf("Failed to generate bundle %s: %v", policyName, err)
		}

		fmt.Printf("    Created: %s\n", bundlePath)
	}

	fmt.Println("Bundle generation complete!")
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
	entries, err := os.ReadDir(policyDir)
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
		fileContent, err := os.ReadFile(filePath)
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
