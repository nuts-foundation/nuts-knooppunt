package mcsd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var _ component.Lifecycle = &Component{}

type Component struct {
	config             Config
	fhirClientFn       func(baseURL *url.URL) fhirclient.Client
	lastUpdateTimes    map[string]time.Time
	lastUpdateTimesMux *sync.RWMutex
}

type Config struct {
	RootDirectories map[string]DirectoryConfig `koanf:"rootdirectories"`
	LocalDirectory  DirectoryConfig            `koanf:"localdirectory"`
}

type DirectoryConfig struct {
	FHIRBaseURL string `koanf:"fhirbaseurl"`
}

type UpdateReport map[string]DirectoryUpdateReport

type DirectoryUpdateReport struct {
	CountCreated int      `json:"created"`
	CountUpdated int      `json:"updated"`
	CountDeleted int      `json:"deleted"`
	Warnings     []string `json:"warnings,omitempty"`
	Error        error    `json:"error,omitempty"`
}

func New(config Config) *Component {
	return &Component{
		config: config,
		fhirClientFn: func(baseURL *url.URL) fhirclient.Client {
			return fhirclient.New(baseURL, http.DefaultClient, nil)
		},
		lastUpdateTimes:    make(map[string]time.Time),
		lastUpdateTimesMux: &sync.RWMutex{},
	}
}

func (c *Component) Start() error {
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	return nil
}

func (c *Component) RegisterHttpHandlers(publicMux, internalMux *http.ServeMux) {
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

func (c *Component) update(ctx context.Context) (UpdateReport, error) {
	result := make(UpdateReport)
	for _, root := range c.config.RootDirectories {
		report, err := c.updateFromDirectory(ctx, root.FHIRBaseURL, []string{"Organization", "Endpoint"})
		if err != nil {
			log.Ctx(ctx).Err(err).Str("directory", root.FHIRBaseURL).Msg("mCSD root directory update failed")
			report.Error = errors.Join(err, report.Error)
		}
		result[root.FHIRBaseURL] = report

	}
	return result, nil
}

func (c *Component) updateFromDirectory(ctx context.Context, fhirBaseURLRaw string, allowedResourceTypes []string) (DirectoryUpdateReport, error) {
	remoteDirFHIRBaseURL, err := url.Parse(fhirBaseURLRaw)
	if err != nil {
		return DirectoryUpdateReport{}, err
	}
	remoteDirFHIRClient := c.fhirClientFn(remoteDirFHIRBaseURL)

	localDirFHIRBaseURL, err := url.Parse(c.config.LocalDirectory.FHIRBaseURL)
	if err != nil {
		return DirectoryUpdateReport{}, err
	}
	localDirFHIRClient := c.fhirClientFn(localDirFHIRBaseURL)

	// Query remote directory
	var bundle fhir.Bundle
	// TODO: Pagination

	// Get last update time for incremental sync
	c.lastUpdateTimesMux.RLock()
	lastUpdate, hasLastUpdate := c.lastUpdateTimes[fhirBaseURLRaw]
	c.lastUpdateTimesMux.RUnlock()

	searchParams := url.Values{}
	if hasLastUpdate {
		searchParams.Set("_since", lastUpdate.Format(time.RFC3339))
	}

	if err = remoteDirFHIRClient.SearchWithContext(ctx, "", searchParams, &bundle, fhirclient.AtPath("/_history")); err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("_history search failed: %w", err)
	}

	// Update local directory
	tx := fhir.Bundle{
		Type:  fhir.BundleTypeTransaction,
		Entry: make([]fhir.BundleEntry, 0, len(bundle.Entry)),
	}
	warnings, err := buildUpdateTransaction(ctx, &tx, bundle.Entry, allowedResourceTypes)
	if err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("failed to build update transaction: %w", err)
	}
	var txResult fhir.Bundle
	if err := localDirFHIRClient.CreateWithContext(ctx, tx, &txResult, fhirclient.AtPath("/")); err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("failed to apply mCSD update to local directory: %w", err)
	}

	// Process result
	report := DirectoryUpdateReport{
		Warnings: warnings,
	}
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

	// Update last sync timestamp on successful completion
	c.lastUpdateTimesMux.Lock()
	if c.lastUpdateTimes == nil {
		c.lastUpdateTimes = make(map[string]time.Time)
	}
	c.lastUpdateTimes[fhirBaseURLRaw] = time.Now()
	c.lastUpdateTimesMux.Unlock()

	return report, nil
}
