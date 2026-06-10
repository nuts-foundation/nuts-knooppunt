// Package lrza implements a synchronization client for a single trusted mCSD Directory (the LRZA -
// Landelijk Register Zorgaanbieders, the government-controlled registry of care providers).
//
// Unlike the mcsd component - which discovers peer directories and applies anti-spoofing validation
// to their contents - lrza syncs from one directory that is trusted wholesale. There is therefore no
// discovery phase and no per-resource validation: every resource the source returns is imported into
// the local query directory as-is. The read pattern is the same incremental one mcsd uses: query each
// resource type's _history endpoint with _since, deduplicate to the most recent version per resource,
// and replay the result as a FHIR transaction against the query directory.
package lrza

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/httpauth"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var _ component.Lifecycle = &Component{}

// defaultResourceTypes are the resource types synced from the trusted directory by default.
var defaultResourceTypes = []string{"Organization", "Endpoint", "Location", "HealthcareService", "PractitionerRole", "Practitioner"}

// maxUpdateEntries limits the number of entries processed in a single FHIR transaction to prevent
// excessive load on the FHIR server.
const maxUpdateEntries = 1000

// searchPageSize is a fixed FHIR search result page size, so behavior is deterministic across FHIR
// servers rather than relying on (widely varying) server defaults.
const searchPageSize = 100

// clockSkewBuffer is subtracted from local time when a Bundle's meta.lastUpdated is not available,
// to account for clock differences between this client and the FHIR server.
var clockSkewBuffer = 2 * time.Second

func DefaultConfig() Config {
	return Config{
		ResourceTypes: defaultResourceTypes,
	}
}

type Config struct {
	// LRZABaseUrl is the base URL of the trusted source directory (the LRZA) to sync from.
	LRZABaseUrl string `koanf:"lrzabaseurl"`
	// QueryBaseUrl is the base URL of the local mCSD query directory that synced resources are
	// written into.
	QueryBaseUrl string `koanf:"querybaseurl"`
	// ResourceTypes are the FHIR resource types to sync. Defaults to defaultResourceTypes.
	ResourceTypes []string `koanf:"resourcetypes"`
	// Auth optionally configures OAuth2 client-credentials authentication against the source.
	Auth httpauth.OAuth2Config `koanf:"auth"`
}

// UpdateReport summarizes the outcome of a single sync cycle.
type UpdateReport struct {
	CountCreated int      `json:"created"`
	CountUpdated int      `json:"updated"`
	CountDeleted int      `json:"deleted"`
	Warnings     []string `json:"warnings"`
	Errors       []string `json:"errors"`
}

// Component syncs a single trusted mCSD Directory into the local query directory.
type Component struct {
	config          Config
	sourceClient    fhirclient.Client
	fhirQueryClient fhirclient.Client

	resourceTypes  []string
	lastUpdateTime string // _since value for the next incremental sync; empty means full sync
	updateMux      *sync.Mutex
}

// syncRun holds the state of a single sync cycle, threaded through each step (fetch -> build -> apply
// -> record) so the steps take one argument instead of a growing parameter list. The search
// configuration is set when the run is created; the remaining fields are filled as the run
// progresses.
type syncRun struct {
	// configuration, set at construction
	queryStart   time.Time
	searchParams url.Values

	// working state, filled as the run progresses
	entries        []fhir.BundleEntry // deduplicated history entries to sync
	firstSearchSet fhir.Bundle        // first resource type's search set, for the next _since timestamp
	tx             fhir.Bundle        // transaction bundle applied to the query directory
	report         UpdateReport
}

func New(config Config) (*Component, error) {
	sourceBaseURL, err := url.Parse(config.LRZABaseUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid LRZA source FHIR base URL (url=%s): %w", config.LRZABaseUrl, err)
	}

	// HTTP client for the source directory, with optional OAuth2 authentication.
	// TODO: Implement and test end-to-end flow against iRealisatie proeftuin
	sourceHTTPClient := tracing.NewHTTPClient()
	if config.Auth.IsConfigured() {
		slog.Info("LRZA: OAuth2 authentication configured", slog.String("token_endpoint", config.Auth.TokenEndpoint))
		sourceHTTPClient, err = httpauth.NewOAuth2HTTPClient(config.Auth, tracing.WrapTransport(nil))
		if err != nil {
			return nil, fmt.Errorf("failed to create OAuth2 HTTP client for LRZA: %w", err)
		}
	}

	queryBaseURL, err := url.Parse(config.QueryBaseUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid LRZA query directory FHIR base URL (url=%s): %w", config.QueryBaseUrl, err)
	}

	resourceTypes := config.ResourceTypes
	if len(resourceTypes) == 0 {
		resourceTypes = append([]string(nil), defaultResourceTypes...)
	}

	return &Component{
		config:          config,
		sourceClient:    fhirclient.New(sourceBaseURL, sourceHTTPClient, &fhirclient.Config{UsePostSearch: false}),
		fhirQueryClient: fhirclient.New(queryBaseURL, tracing.NewHTTPClient(), &fhirclient.Config{UsePostSearch: false}),
		resourceTypes:   resourceTypes,
		updateMux:       &sync.Mutex{},
	}, nil
}

func (c *Component) Start() error { return nil }

func (c *Component) Stop(ctx context.Context) error { return nil }

func (c *Component) RegisterHttpHandlers(publicMux, internalMux *http.ServeMux) {
	internalMux.HandleFunc("POST /lrza/update", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		report, err := c.update(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "LRZA update failed", logging.Error(err))
			http.Error(w, "Failed to update LRZA: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(report)
	})
}

// update runs one sync cycle: fetch the trusted source's history, build a transaction from it, apply
// it to the query directory, and record the timestamp for the next incremental sync.
func (c *Component) update(ctx context.Context) (UpdateReport, error) {
	c.updateMux.Lock()
	defer c.updateMux.Unlock()

	run := c.newSyncRun()
	slog.InfoContext(ctx, "Updating from trusted LRZA directory", logging.FHIRServer(c.config.LRZABaseUrl), slog.Any("resourceTypes", c.resourceTypes))
	if c.lastUpdateTime == "" {
		slog.InfoContext(ctx, "No last update time, doing full LRZA sync")
	}

	if err := c.fetchEntries(ctx, run); err != nil {
		return UpdateReport{}, err
	}
	c.buildTransaction(ctx, run)
	if len(run.tx.Entry) > 0 {
		if err := c.applyTransaction(ctx, run); err != nil {
			return UpdateReport{}, err
		}
	}
	c.recordSyncTimestamp(ctx, run)

	return run.finalizedReport(), nil
}

// newSyncRun creates a run with the search parameters for this cycle: a fixed page size, newest-first
// ordering (deduplication relies on it), and the _since value for incremental sync when known.
func (c *Component) newSyncRun() *syncRun {
	params := url.Values{
		"_count": []string{strconv.Itoa(searchPageSize)},
		// Pin newest-first ordering: deduplication relies on it (a history Bundle is sorted with
		// oldest versions last). Don't trust the server default.
		"_sort": []string{"-_lastUpdated"},
	}
	if c.lastUpdateTime != "" {
		params.Set("_since", c.lastUpdateTime)
	}
	return &syncRun{
		queryStart:   time.Now(),
		searchParams: params,
	}
}

// fetchEntries queries the _history endpoint of every configured resource type, combines the
// results, and deduplicates them to the most recent version per resource.
func (c *Component) fetchEntries(ctx context.Context, run *syncRun) error {
	var entries []fhir.BundleEntry
	for i, resourceType := range c.resourceTypes {
		curr, searchSet, err := c.queryHistory(ctx, resourceType, cloneValues(run.searchParams))
		if err != nil {
			return fmt.Errorf("failed to query %s history: %w", resourceType, err)
		}
		entries = append(entries, curr...)
		if i == 0 {
			run.firstSearchSet = searchSet
		}
	}
	run.entries = libfhir.DeduplicateHistoryEntries(entries)
	return nil
}

// queryHistory queries a single resource type's _history endpoint, following pagination up to
// maxUpdateEntries.
func (c *Component) queryHistory(ctx context.Context, resourceType string, searchParams url.Values) ([]fhir.BundleEntry, fhir.Bundle, error) {
	var searchSet fhir.Bundle
	if err := c.sourceClient.SearchWithContext(ctx, "", searchParams, &searchSet, fhirclient.AtPath(resourceType+"/_history")); err != nil {
		return nil, fhir.Bundle{}, fmt.Errorf("_history search failed: %w", err)
	}

	var entries []fhir.BundleEntry
	err := fhirclient.Paginate(ctx, c.sourceClient, searchSet, func(set *fhir.Bundle) (bool, error) {
		entries = append(entries, set.Entry...)
		if len(entries) >= maxUpdateEntries {
			return false, fmt.Errorf("too many entries (%d), aborting update to prevent excessive memory usage", len(entries))
		}
		return true, nil
	})
	if err != nil {
		return nil, fhir.Bundle{}, fmt.Errorf("pagination of _history search failed: %w", err)
	}
	return entries, searchSet, nil
}

// buildTransaction converts the deduplicated history entries into a FHIR transaction bundle for the
// query directory. Entries that can't be processed are recorded as warnings rather than failing the
// whole sync.
func (c *Component) buildTransaction(ctx context.Context, run *syncRun) {
	run.tx = fhir.Bundle{
		Type:  fhir.BundleTypeTransaction,
		Entry: make([]fhir.BundleEntry, 0, len(run.entries)),
	}
	for i, entry := range run.entries {
		if err := c.appendTransactionEntry(ctx, run, entry); err != nil {
			run.report.Warnings = append(run.report.Warnings, fmt.Sprintf("entry #%d: %s", i, err.Error()))
		}
	}
}

// appendTransactionEntry translates one deduplicated history entry into a conditional upsert or
// delete against the query directory and appends it to run.tx. Resources are tagged with a
// deterministic _source so updates are idempotent and deletes target the right resource.
//
// This is lrza's trimmed counterpart to mcsd's buildUpdateTransaction: because the source is trusted,
// there is no anti-spoofing validation and no discoverable-directory filtering - every entry is
// imported as-is.
func (c *Component) appendTransactionEntry(ctx context.Context, run *syncRun, entry fhir.BundleEntry) error {
	if entry.Request == nil {
		return errors.New("missing 'request' field")
	}

	// DELETE entries carry no resource body; translate to a conditional delete keyed by _source.
	if entry.Request.Method == fhir.HTTPVerbDELETE {
		resourceType, resourceID, ok := libfhir.TypeAndIDFromReference(entry.Request.Url)
		if !ok {
			return fmt.Errorf("invalid DELETE URL format: %s", entry.Request.Url)
		}
		sourceURL, err := libfhir.BuildSourceURL(c.config.LRZABaseUrl, resourceType, resourceID)
		if err != nil {
			return fmt.Errorf("failed to build source URL for DELETE: %w", err)
		}
		slog.DebugContext(ctx, "Deleting resource", slog.String("type", resourceType), slog.String("id", resourceID))
		run.tx.Entry = append(run.tx.Entry, fhir.BundleEntry{
			Request: &fhir.BundleEntryRequest{
				Method: fhir.HTTPVerbDELETE,
				Url:    resourceType + "?" + url.Values{"_source": []string{sourceURL}}.Encode(),
			},
		})
		return nil
	}

	// CREATE/UPDATE entries carry a resource body; rewrite to a conditional update keyed by _source.
	if entry.Resource == nil {
		return errors.New("missing 'resource' field for non-DELETE operation")
	}
	resource := make(map[string]any)
	if err := json.Unmarshal(entry.Resource, &resource); err != nil {
		return fmt.Errorf("failed to unmarshal resource: %w", err)
	}
	resourceType, ok := resource["resourceType"].(string)
	if !ok {
		return errors.New("resource has no resourceType")
	}
	resourceID, ok := resource["id"].(string)
	if !ok {
		return errors.New("resource has no id")
	}
	sourceURL, err := libfhir.BuildSourceURL(c.config.LRZABaseUrl, resourceType, resourceID)
	if err != nil {
		return fmt.Errorf("failed to build source URL: %w", err)
	}

	setResourceSource(resource, sourceURL)
	// Drop the source's id so the query directory resolves the resource by _source instead.
	delete(resource, "id")
	if err := convertReferencesToConditional(resource, c.config.LRZABaseUrl); err != nil {
		return fmt.Errorf("failed to convert references: %w", err)
	}

	resourceJSON, err := json.Marshal(resource)
	if err != nil {
		return err
	}
	slog.DebugContext(ctx, "Updating resource", slog.String("type", resourceType), slog.String("id", resourceID))
	run.tx.Entry = append(run.tx.Entry, fhir.BundleEntry{
		Resource: resourceJSON,
		Request: &fhir.BundleEntryRequest{
			Method: fhir.HTTPVerbPUT,
			Url:    resourceType + "?" + url.Values{"_source": []string{sourceURL}}.Encode(),
		},
	})
	return nil
}

// applyTransaction sends the transaction bundle to the query directory and tallies the per-entry
// outcomes (created/updated/deleted) into the report.
func (c *Component) applyTransaction(ctx context.Context, run *syncRun) error {
	var txResult fhir.Bundle
	if err := c.fhirQueryClient.CreateWithContext(ctx, run.tx, &txResult, fhirclient.AtPath("/")); err != nil {
		return fmt.Errorf("failed to apply LRZA update to query directory: %w", err)
	}

	for i, entry := range txResult.Entry {
		if entry.Response == nil {
			run.report.Warnings = append(run.report.Warnings, fmt.Sprintf("Skipping entry with no response: #%d", i))
			continue
		}
		switch {
		case strings.HasPrefix(entry.Response.Status, "201"):
			run.report.CountCreated++
		case strings.HasPrefix(entry.Response.Status, "200"):
			run.report.CountUpdated++
		case strings.HasPrefix(entry.Response.Status, "204"):
			run.report.CountDeleted++
		default:
			run.report.Warnings = append(run.report.Warnings, fmt.Sprintf("Unknown HTTP response status %v (url=%v)", entry.Response.Status, entry.FullUrl))
		}
	}
	return nil
}

// recordSyncTimestamp stores the timestamp used as the _since value for the next incremental sync.
// It prefers the search result Bundle's meta.lastUpdated (the FHIR server's own clock, avoiding
// skew) and falls back to the local query start time minus a buffer.
func (c *Component) recordSyncTimestamp(ctx context.Context, run *syncRun) {
	if run.firstSearchSet.Meta != nil && run.firstSearchSet.Meta.LastUpdated != nil {
		c.lastUpdateTime = *run.firstSearchSet.Meta.LastUpdated
		return
	}
	c.lastUpdateTime = run.queryStart.Add(-clockSkewBuffer).Format(time.RFC3339Nano)
	slog.WarnContext(ctx, "Bundle meta.lastUpdated not available, using local time with buffer - may cause clock skew issues", logging.FHIRServer(c.config.LRZABaseUrl))
}

// finalizedReport returns the run's report with nil slices replaced by empty ones, for a nicer JSON
// REST response.
func (r *syncRun) finalizedReport() UpdateReport {
	report := r.report
	if report.Warnings == nil {
		report.Warnings = []string{}
	}
	if report.Errors == nil {
		report.Errors = []string{}
	}
	return report
}

// setResourceSource sets meta.source on a resource and drops the source server's versionId and
// lastUpdated, which are meaningless in the query directory.
func setResourceSource(resource map[string]any, source string) {
	meta, ok := resource["meta"].(map[string]any)
	if !ok {
		meta = make(map[string]any)
		resource["meta"] = meta
	}
	meta["source"] = source
	delete(meta, "versionId")
	delete(meta, "lastUpdated")
}

// convertReferencesToConditional rewrites every relative "ResourceType/id" reference in the resource
// into a deterministic conditional reference keyed by _source, so references resolve against the
// query directory's copies of the same source resources.
// conditional references are explained in more detail in the FHR documentation here:
// http://hl7.org/fhir/R4/http.html#trules
func convertReferencesToConditional(obj any, sourceBaseURL string) error {
	switch v := obj.(type) {
	case map[string]any:
		if ref, ok := v["reference"].(string); ok {
			parts := strings.Split(ref, "/")
			if len(parts) == 2 {
				resourceType := parts[0]
				sourceURL, err := libfhir.BuildSourceURL(sourceBaseURL, ref)
				if err != nil {
					return fmt.Errorf("failed to build source URL for reference: %w", err)
				}
				v["reference"] = resourceType + "?_source=" + url.QueryEscape(sourceURL)
			}
		}
		for _, value := range v {
			if err := convertReferencesToConditional(value, sourceBaseURL); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := convertReferencesToConditional(item, sourceBaseURL); err != nil {
				return err
			}
		}
	}
	return nil
}

// cloneValues returns a shallow copy of the given url.Values, so per-request mutations don't affect
// the shared run search parameters.
func cloneValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
