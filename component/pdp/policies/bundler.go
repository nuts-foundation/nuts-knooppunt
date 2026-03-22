package policies

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
)

//go:embed */*
var policies embed.FS

// bundles maps scope names to their embedded bundles
// This is populated lazily when first accessed
var bundles map[string][]byte

// Bundles returns the embedded OPA bundles in gzipped tar format, keyed by scope name.
// They are generated on first access.
// Test-only policy directories (prefixed with "test_") are excluded.
func Bundles(ctx context.Context) (map[string][]byte, error) {
	if bundles == nil {
		slog.DebugContext(ctx, "Generating Open Policy Agent bundles...")
		var err error
		bundles, err = GenerateBundles(func(name string) bool {
			return strings.HasPrefix(name, "test_")
		})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OPA bundles: %w", err)
		}
	}
	return bundles, nil
}

// GenerateBundles builds in-memory OPA bundles for all policy directories,
// skipping any directory whose name matches the skip predicate.
func GenerateBundles(skip func(name string) bool) (map[string][]byte, error) {
	entries, err := policies.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("failed to read policies directory: %w", err)
	}

	result := make(map[string][]byte)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "bundles" || skip(entry.Name()) {
			continue
		}
		policyName := entry.Name()

		// Check if policy.rego exists in this directory
		policyPath := filepath.Join(policyName, "policy.rego")
		if h, err := policies.Open(policyPath); err != nil {
			return nil, fmt.Errorf("policy.rego not found in %s", policyName)
		} else {
			_ = h.Close()
		}

		data, err := generateBundle(policyName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate bundle for %s: %w", policyName, err)
		}
		result[policyName] = data
	}
	return result, nil
}

func generateBundle(policyName string) ([]byte, error) {
	var buf bytes.Buffer

	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	manifest := map[string]interface{}{
		"revision": fmt.Sprintf("%d", time.Now().Unix()),
		"roots":    []string{policyName},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}
	if err := tarWriter.WriteHeader(&tar.Header{
		Name:    filepath.Join(policyName, ".manifest"),
		Mode:    0644,
		Size:    int64(len(manifestBytes)),
		ModTime: time.Now(),
	}); err != nil {
		return nil, fmt.Errorf("failed to write manifest header: %w", err)
	}
	if _, err := tarWriter.Write(manifestBytes); err != nil {
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}

	entries, err := policies.ReadDir(policyName)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(policyName, entry.Name())
		fileContent, err := policies.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", entry.Name(), err)
		}
		if err := tarWriter.WriteHeader(&tar.Header{
			Name:    filepath.Join(policyName, entry.Name()),
			Mode:    0644,
			Size:    int64(len(fileContent)),
			ModTime: time.Now(),
		}); err != nil {
			return nil, fmt.Errorf("failed to write header for %s: %w", entry.Name(), err)
		}
		if _, err := tarWriter.Write(fileContent); err != nil {
			return nil, fmt.Errorf("failed to write content for %s: %w", entry.Name(), err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return buf.Bytes(), nil
}
