package mcsd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var _ component.Lifecycle = &Component{}

type Component struct {
	config Config
}

type Config struct {
	RootDirectories map[string]DirectoryConfig `json:"roots"`
	LocalDirectory  DirectoryConfig            `json:"local"`
}

type DirectoryConfig struct {
	FHIRBaseURL string `json:"url"`
}

type UpdateReport map[string]DirectoryUpdateReport

type DirectoryUpdateReport struct {
	CountCreated int      `json:"created"`
	CountUpdated int      `json:"updated"`
	CountDeleted int      `json:"deleted"`
	Warnings     []string `json:"warnings,omitempty"`
	Error        error    `json:"error,omitempty"`
}

func New(config Config) (*Component, error) {
	return &Component{config: config}, nil
}

func (c Component) Start() error {
	return nil
}

func (c Component) Stop(ctx context.Context) error {
	return nil
}

func (c Component) RegisterHttpHandlers(publicMux, internalMux *http.ServeMux) {
	internalMux.HandleFunc("POST /mcsd/update", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		result, err := c.update(ctx)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("mCSD update failed")
			http.Error(w, "Failed to update mCSD: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	})
}

func (c Component) update(ctx context.Context) (UpdateReport, error) {
	result := make(UpdateReport)
	for _, root := range c.config.RootDirectories {
		directoryUpdateReport, err := c.updateFromDirectory(ctx, root.FHIRBaseURL, []string{"Endpoint"})
		if err != nil {
			log.Ctx(ctx).Error().Str("directory", root.FHIRBaseURL).Msg("mCSD root directory update failed")
			result[root.FHIRBaseURL] = DirectoryUpdateReport{
				Error: err,
			}
		} else {
			result[root.FHIRBaseURL] = directoryUpdateReport
		}

	}
	return result, nil
}

func (c Component) updateFromDirectory(ctx context.Context, fhirBaseURLRaw string, allowedResourceTypes []string) (DirectoryUpdateReport, error) {
	fhirBaseURL, err := url.Parse(fhirBaseURLRaw)
	if err != nil {
		return DirectoryUpdateReport{}, err
	}
	// Query remote directory
	fhirClient := fhirclient.New(fhirBaseURL, http.DefaultClient, nil)
	var bundle fhir.Bundle
	// TODO: Pagination
	if err = fhirClient.SearchWithContext(ctx, "", nil, &bundle, fhirclient.AtPath("/_history")); err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("_history search failed: %w", err)
	}

	// Update local directory
	tx := fhir.Bundle{
		Type:  fhir.BundleTypeTransaction,
		Entry: make([]fhir.BundleEntry, 0, len(bundle.Entry)),
	}
	if err = buildUpdateTransaction(ctx, &tx, bundle.Entry, allowedResourceTypes); err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("failed to build update transaction: %w", err)
	}
	var txResult fhir.Bundle
	if err := fhirClient.CreateWithContext(ctx, tx, &txResult, fhirclient.AtPath("/")); err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("failed to apply mCSD update to local directory: %w", err)
	}

	// Process result
	report := DirectoryUpdateReport{}
	for _, entry := range txResult.Entry {
		if entry.Response == nil {
			log.Ctx(ctx).Warn().Msgf("Skipping entry with no response: %v", entry)
			continue
		}
		if entry.Response.Status == "" {
			log.Ctx(ctx).Warn().Msgf("Skipping entry with empty response status: %v", entry)
			continue
		}
		switch {
		case strings.HasPrefix(entry.Response.Status, "201"):
			report.CountCreated++
		case strings.HasPrefix(entry.Response.Status, "200"):
			report.CountUpdated++
		case strings.HasPrefix(entry.Response.Status, "204"):
			report.CountDeleted++
		default:
			msg := fmt.Sprintf("Unknown HTTP response status %v (url=%v)", entry.Response.Status, entry.FullUrl)
			report.Warnings = append(report.Warnings, msg)
		}
	}
	return report, nil
}
