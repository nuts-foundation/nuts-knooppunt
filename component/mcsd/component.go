package mcsd

import (
	"context"
	"encoding/json"
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
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/httpauth"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var _ component.Lifecycle = &Component{}

var rootDirectoryResourceTypes = []string{"Organization", "Endpoint"}
var defaultDirectoryResourceTypes = []string{"Organization", "Endpoint", "Location", "HealthcareService", "PractitionerRole", "Practitioner"}

// parentOrganizationMap maps parent organizations (with URA identifier) to their linked child organizations
type parentOrganizationMap map[*fhir.Organization][]*fhir.Organization

// clockSkewBuffer is subtracted from local time when Bundle meta.lastUpdated is not available
// to account for potential clock differences between client and FHIR server
var clockSkewBuffer = 2 * time.Second

// maxUpdateEntries limits the number of entries processed in a single FHIR transaction to prevent excessive load on the FHIR server
const maxUpdateEntries = 1000

// searchPageSize is an arbitrary FHIR search result limit (per page), so we have deterministic behavior across FHIR servers,
// and don't rely on server defaults (which may be very high or very low (Azure FHIR's default is 10)).
const searchPageSize = 100

// makeDirectoryKey creates a composite key from fhirBaseURL and authoritativeUra for tracking sync state per directory.
// This allows multiple directories with the same FHIR base URL but different authoritative URAs to maintain separate sync states.
func makeDirectoryKey(fhirBaseURL, authoritativeUra string) string {
	if authoritativeUra == "" {
		return fhirBaseURL
	}
	return fhirBaseURL + "|" + authoritativeUra
}

// Component implements a mCSD Update Client, which synchronizes mCSD FHIR resources from remote mCSD Directories to a local mCSD Directory for querying.
// It is configured with a root mCSD Directory, which is used to discover organizations and their mCSD Directory endpoints.
// Organizations refer to Endpoints through Organization.endpoint references.
// Synchronization is a 2-step process:
// 1. Query the root mCSD Directory for Organization resources and their associated Endpoint resources of type 'mcsd-directory-endpoint'.
// 2. For each discovered mCSD Directory Endpoint, query the remote mCSD Directory for its resources and copy them to the local mCSD Query Directory.
//   - The following resource types are synchronized: Organization, Endpoint, Location, HealthcareService
//   - An organization's mCSD Directory may only return Organization resources that:
//   - exist in the root mCSD Directory (link by identifier, name must be the same)
//   - have the same mcsd-directory-endpoint as the directory being queried
//   - These are mitigating measures to prevent an attacker to spoof another care organization.
//   - The organization's mcsd-directory-endpoint must be discoverable through the root mCSD Directory.'
//
// A root directory may be marked as 'trusted' (DirectoryConfig.Trusted), which skips the per-resource
// validation described above. This is intended for directories the operator controls or
// otherwise trusts (e.g. the LRZA). Trust does not affect sync cadence — incremental sync via _since
// still applies. Directories discovered from a trusted root are always registered as untrusted.
type Component struct {
	config            Config
	fhirAdminClientFn func(baseURL *url.URL) fhirclient.Client
	fhirQueryClient   fhirclient.Client

	administrationDirectories []administrationDirectory
	directoryResourceTypes    []string
	lastUpdateTimes           map[string]string
	updateMux                 *sync.RWMutex
}

func DefaultConfig() Config {
	return Config{
		DirectoryResourceTypes: defaultDirectoryResourceTypes,
	}
}

type Config struct {
	AdministrationDirectories map[string]DirectoryConfig `koanf:"admin"`
	QueryDirectory            DirectoryConfig            `koanf:"query"`
	ExcludeAdminDirectories   []string                   `koanf:"adminexclude"`
	DirectoryResourceTypes    []string                   `koanf:"directoryresourcetypes"`
	Auth                      httpauth.OAuth2Config      `koanf:"auth"`
}

type DirectoryConfig struct {
	FHIRBaseURL string `koanf:"fhirbaseurl"`
	// Trusted disables anti-spoofing validation on this directory's contents.
	// Only safe for directories you control or otherwise trust (like the LRZA).
	Trusted bool `koanf:"trusted"`
}

type UpdateReport map[string]DirectoryUpdateReport

type administrationDirectory struct {
	fhirBaseURL      string
	resourceTypes    []string
	discover         bool
	trusted          bool   // Skip validation checks on this directory's contents
	sourceURL        string // The fullUrl from the Bundle entry that created this Endpoint, used for unregistration on DELETE
	authoritativeUra string // URA of the organization that is authoritative for this directory
}

type DirectoryUpdateReport struct {
	CountCreated int      `json:"created"`
	CountUpdated int      `json:"updated"`
	CountDeleted int      `json:"deleted"`
	Warnings     []string `json:"warnings"`
	Errors       []string `json:"errors"`
}

// syncRun holds everything needed to sync a single administration directory into the query
// directory. The config fields (top) describe *what* to sync and are set when the run is
// constructed; the working-state fields (bottom) are filled while the run executes. Bundling them
// in one value lets the sync steps read as named intent instead of branching on positional bools.
type syncRun struct {
	// config
	fhirBaseURL      string
	allowedTypes     []string
	authoritativeUra string
	trusted          bool
	discoverable     bool // only used to compute the import filter; discovery itself is a separate phase

	// working state
	remoteClient   fhirclient.Client
	queryStart     time.Time
	searchParams   url.Values
	entries        []fhir.BundleEntry
	firstSearchSet fhir.Bundle
	parentOrgs     parentOrganizationMap
	healthcareSvcs []fhir.HealthcareService
}

func (d administrationDirectory) newSyncRun() *syncRun {
	return &syncRun{
		fhirBaseURL:      d.fhirBaseURL,
		allowedTypes:     d.resourceTypes,
		authoritativeUra: d.authoritativeUra,
		trusted:          d.trusted,
		discoverable:     d.discover,
	}
}

func (r *syncRun) key() string { return makeDirectoryKey(r.fhirBaseURL, r.authoritativeUra) }

// validates reports whether this directory's contents must pass anti-spoofing validation.
// Trusted directories are imported as-is, so they skip validation entirely.
func (r *syncRun) validates() bool { return !r.trusted }

// onlyDirectoryEndpoints reports whether the sync should import only mCSD directory Endpoints.
// This is true for an untrusted discoverable root: it is crawled for discovery, but its resource
// data is taken from the validated leaf directories rather than imported wholesale.
func (r *syncRun) onlyDirectoryEndpoints() bool { return r.discoverable && !r.trusted }

func (r *syncRun) validationRules() ValidationRules {
	return ValidationRules{AllowedResourceTypes: r.allowedTypes, Trusted: r.trusted}
}

func New(config Config) (*Component, error) {
	// Create HTTP client with optional OAuth2 authentication
	var httpClient *http.Client
	var err error
	if config.Auth.IsConfigured() {
		slog.Info("mCSD: OAuth2 authentication configured", slog.String("token_endpoint", config.Auth.TokenEndpoint))
		httpClient, err = httpauth.NewOAuth2HTTPClient(config.Auth, tracing.WrapTransport(nil))
		if err != nil {
			return nil, fmt.Errorf("failed to create OAuth2 HTTP client for mCSD: %w", err)
		}
	} else {
		httpClient = tracing.NewHTTPClient()
	}

	queryDirectoryFHIRBaseURL, err := url.Parse(config.QueryDirectory.FHIRBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Query Directory FHIR base URL (url=%s): %w", config.QueryDirectory.FHIRBaseURL, err)
	}

	result := &Component{
		config: config,
		fhirAdminClientFn: func(baseURL *url.URL) fhirclient.Client {
			return fhirclient.New(baseURL, tracing.NewHTTPClient(), &fhirclient.Config{
				UsePostSearch: false,
			})
		},
		fhirQueryClient: fhirclient.New(queryDirectoryFHIRBaseURL, httpClient, &fhirclient.Config{
			UsePostSearch: false,
		}),
		directoryResourceTypes: config.DirectoryResourceTypes,
		lastUpdateTimes:        make(map[string]string),
		updateMux:              &sync.RWMutex{},
	}
	for _, rootDirectory := range config.AdministrationDirectories {
		if err := result.registerAdministrationDirectory(context.Background(), rootDirectory.FHIRBaseURL, rootDirectoryResourceTypes, true, "", "", rootDirectory.Trusted); err != nil {
			return nil, fmt.Errorf("register root administration directory (url=%s): %w", rootDirectory.FHIRBaseURL, err)
		}
	}
	if result.config.DirectoryResourceTypes == nil || len(result.config.DirectoryResourceTypes) == 0 {
		result.config.DirectoryResourceTypes = append([]string(nil), defaultDirectoryResourceTypes...)
	}
	return result, nil
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
			slog.ErrorContext(ctx, "mCSD update failed", logging.Error(err))
			http.Error(w, "Failed to update mCSD: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	})
}

// update runs a full refresh cycle in two phases: first discovery, then sync.
//
// Phase 1 (discovery) crawls discoverable (root) directories and registers/unregisters the
// administration directories they point at; it only mutates c.administrationDirectories.
// Phase 2 (sync) imports resources from every administration directory — roots and the ones just
// discovered — into the query directory. Running discovery to completion first means a directory
// discovered this cycle is synced in the same cycle. Warnings/errors from discovery are merged into
// the sync report for the same directory.
//
// As a result a discoverable root is queried twice per cycle (a full scan in phase 1, then an
// incremental query in phase 2). This is intentional: the phases do different work — discovery only
// updates the in-memory registry, while sync imports the root's own mCSD directory Endpoints into
// the query directory. The separation is kept deliberately even though it costs the extra query.
func (c *Component) update(ctx context.Context) (UpdateReport, error) {
	c.updateMux.Lock()
	defer c.updateMux.Unlock()

	// Phase 1: discovery.
	result := c.discover(ctx)

	// Phase 2: sync.
	for _, adminDirectory := range c.administrationDirectories {
		run := adminDirectory.newSyncRun()
		report, err := c.runSyncJob(ctx, run)
		if err != nil {
			slog.ErrorContext(ctx, "mCSD Directory update failed", logging.FHIRServer(run.fhirBaseURL), logging.Error(err))
			report.Errors = append(report.Errors, err.Error())
		}
		// Merge discovery-phase warnings/errors for the same directory (roots only).
		if discoveryReport, ok := result[run.key()]; ok {
			report.Warnings = append(discoveryReport.Warnings, report.Warnings...)
			report.Errors = append(discoveryReport.Errors, report.Errors...)
		}
		// Return empty slices instead of null ones, makes a nicer REST API
		if report.Warnings == nil {
			report.Warnings = []string{}
		}
		if report.Errors == nil {
			report.Errors = []string{}
		}
		result[run.key()] = report
	}
	return result, nil
}

// runSyncJob imports resources from a single administration directory into the query directory.
// Discovery of new directories is a separate phase (see discover); this only syncs resources, so
// every directory — root or discovered — is treated the same here. It reads as the sequence of
// steps documented on each helper.
func (c *Component) runSyncJob(ctx context.Context, run *syncRun) (DirectoryUpdateReport, error) {
	slog.InfoContext(ctx, "Updating from mCSD Directory", logging.FHIRServer(run.fhirBaseURL), slog.Bool("trusted", run.trusted), slog.Any("resourceTypes", run.allowedTypes))

	if err := c.startSyncRun(ctx, run); err != nil {
		return DirectoryUpdateReport{}, err
	}
	if err := c.fetchEntries(ctx, run); err != nil {
		return DirectoryUpdateReport{}, err
	}
	if err := c.loadParentOrgs(ctx, run); err != nil {
		return DirectoryUpdateReport{}, err
	}

	// _history can return multiple versions of the same resource, but a transaction bundle must
	// contain each resource at most once, so keep only the most recent version of each.
	deduplicatedEntries := deduplicateHistoryEntries(run.entries)
	report, tx := c.buildTransaction(ctx, run, deduplicatedEntries)

	slog.DebugContext(ctx, "Got mCSD entries", logging.FHIRServer(run.fhirBaseURL), slog.Int("count", len(tx.Entry)))
	if len(tx.Entry) == 0 {
		return report, nil
	}

	report, err := c.applyTransaction(ctx, tx, report)
	if err != nil {
		return DirectoryUpdateReport{}, err
	}
	c.recordSyncTimestamp(ctx, run)
	return report, nil
}

// startSyncRun resolves the remote FHIR client and sets up the search parameters, applying the
// _since parameter for incremental sync when a previous sync timestamp is known for this directory.
func (c *Component) startSyncRun(ctx context.Context, run *syncRun) error {
	remoteBaseURL, err := url.Parse(run.fhirBaseURL)
	if err != nil {
		return err
	}
	run.remoteClient = c.fhirAdminClientFn(remoteBaseURL)
	// Capture query start time as fallback for servers that don't provide Bundle meta.lastUpdated.
	run.queryStart = time.Now()

	run.searchParams = url.Values{
		"_count": []string{strconv.Itoa(searchPageSize)},
	}
	if lastUpdate, ok := c.lastUpdateTimes[run.key()]; ok {
		run.searchParams.Set("_since", lastUpdate)
		slog.DebugContext(ctx, "Using _since parameter for incremental sync from FHIR server", logging.FHIRServer(run.fhirBaseURL), slog.String("_since", lastUpdate))
	} else {
		slog.InfoContext(ctx, "No last update time, doing full sync from FHIR server", logging.FHIRServer(run.fhirBaseURL))
	}
	return nil
}

// fetchEntries queries all allowed resource types and stores the results on the run. A URA
// identifier change can't be expressed incrementally (the _since query would mask it), so when one
// is detected the query is rerun in full. That check is skipped for trusted directories, which
// don't validate against URA identifiers.
func (c *Component) fetchEntries(ctx context.Context, run *syncRun) error {
	entries, firstSearchSet, err := c.queryAllResourceTypes(ctx, run.remoteClient, run.allowedTypes, run.searchParams)
	if err != nil {
		return err
	}

	if run.validates() && checkForURAIdentifierChanges(entries) {
		slog.WarnContext(ctx, "Detected URA identifier change in organization history. Rerunning history query without _since parameter.", logging.FHIRServer(run.fhirBaseURL))
		run.searchParams.Del("_since")
		entries, firstSearchSet, err = c.queryAllResourceTypes(ctx, run.remoteClient, run.allowedTypes, run.searchParams)
		if err != nil {
			return err
		}
	}

	run.entries = entries
	run.firstSearchSet = firstSearchSet
	run.healthcareSvcs = extractHealthcareServices(entries)
	return nil
}

// extractHealthcareServices returns all HealthcareService resources among the entries. They are
// collected up front because validating other resources may need to look them up.
func extractHealthcareServices(entries []fhir.BundleEntry) []fhir.HealthcareService {
	var result []fhir.HealthcareService
	for _, entry := range entries {
		if entry.Resource == nil {
			continue
		}
		var healthcareService fhir.HealthcareService
		if err := json.Unmarshal(entry.Resource, &healthcareService); err == nil {
			result = append(result, healthcareService)
		}
	}
	return result
}

// loadParentOrgs builds the map of parent organizations (those with a URA identifier) to their
// linked children and validates it. Both are anti-spoofing measures used to validate organizations
// that don't carry their own URA identifier, so for trusted directories the map stays nil and
// nothing is validated.
func (c *Component) loadParentOrgs(ctx context.Context, run *syncRun) error {
	if !run.validates() {
		return nil
	}
	parentOrgs, err := c.ensureParentOrganizationsMap(ctx, run.fhirBaseURL, run.remoteClient, run.authoritativeUra)
	if err != nil {
		return fmt.Errorf("failed to build parent organization map: %w", err)
	}
	if err := ValidateParentOrganizations(parentOrgs); err != nil {
		return fmt.Errorf("parent organization (one that supposedly has ura identifier - and only only) validation failed: %w", err)
	}
	run.parentOrgs = parentOrgs
	return nil
}

// buildTransaction converts the deduplicated entries into a FHIR transaction bundle with
// deterministic conditional references. Entries that can't be processed are recorded as warnings
// rather than failing the whole sync.
func (c *Component) buildTransaction(ctx context.Context, run *syncRun, deduplicatedEntries []fhir.BundleEntry) (DirectoryUpdateReport, fhir.Bundle) {
	tx := fhir.Bundle{
		Type:  fhir.BundleTypeTransaction,
		Entry: make([]fhir.BundleEntry, 0, len(deduplicatedEntries)),
	}

	var report DirectoryUpdateReport
	for i, entry := range deduplicatedEntries {
		if entry.Request == nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("Skipping entry with no request: #%d", i))
			continue
		}
		slog.DebugContext(ctx, "Processing entry", logging.FHIRServer(run.fhirBaseURL), slog.String("url", entry.Request.Url))
		_, err := buildUpdateTransaction(ctx, &tx, entry, run.validationRules(), run.parentOrgs, run.healthcareSvcs, run.onlyDirectoryEndpoints(), run.fhirBaseURL)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("entry #%d: %s", i, err.Error()))
			continue
		}
	}
	return report, tx
}

// applyTransaction sends the transaction bundle to the query directory and tallies the per-entry
// outcomes (created/updated/deleted) into the report.
func (c *Component) applyTransaction(ctx context.Context, tx fhir.Bundle, report DirectoryUpdateReport) (DirectoryUpdateReport, error) {
	var txResult fhir.Bundle
	if err := c.fhirQueryClient.CreateWithContext(ctx, tx, &txResult, fhirclient.AtPath("/")); err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("failed to apply mCSD update to query directory: %w", err)
	}

	for i, entry := range txResult.Entry {
		if entry.Response == nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("Skipping entry with no response: #%d", i))
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
			report.Warnings = append(report.Warnings, fmt.Sprintf("Unknown HTTP response status %v (url=%v)", entry.Response.Status, entry.FullUrl))
		}
	}
	return report, nil
}

// recordSyncTimestamp stores the timestamp used as the _since value for the next incremental sync.
// It prefers the search result Bundle's meta.lastUpdated (the FHIR server's own clock, avoiding
// skew) and falls back to the local query start time minus a buffer.
func (c *Component) recordSyncTimestamp(ctx context.Context, run *syncRun) {
	var nextSyncTime string
	if run.firstSearchSet.Meta != nil && run.firstSearchSet.Meta.LastUpdated != nil {
		nextSyncTime = *run.firstSearchSet.Meta.LastUpdated
	} else {
		nextSyncTime = run.queryStart.Add(-clockSkewBuffer).Format(time.RFC3339Nano)
		slog.WarnContext(ctx, "Bundle meta.lastUpdated not available, using local time with buffer - may cause clock skew issues", logging.FHIRServer(run.fhirBaseURL))
	}
	c.lastUpdateTimes[run.key()] = nextSyncTime
}

// queryFHIR performs a FHIR search query with pagination and returns all matching entries.
// If includeHistory is true, it queries the _history endpoint to get resource versions.
func (c *Component) queryFHIR(ctx context.Context, client fhirclient.Client, resourceType string, searchParams url.Values, includeHistory bool) ([]fhir.BundleEntry, fhir.Bundle, error) {
	var searchSet fhir.Bundle
	var path string
	var searchErrMsg string
	var paginationErrMsg string

	if includeHistory {
		path = resourceType + "/_history"
		searchErrMsg = "_history search failed"
		paginationErrMsg = "pagination of _history search failed"
	} else {
		path = resourceType
		searchErrMsg = "query failed"
		paginationErrMsg = "pagination of search failed"
	}

	err := client.SearchWithContext(ctx, "", searchParams, &searchSet, fhirclient.AtPath(path))
	if err != nil {
		return nil, fhir.Bundle{}, fmt.Errorf("%s: %w", searchErrMsg, err)
	}

	var entries []fhir.BundleEntry
	err = fhirclient.Paginate(ctx, client, searchSet, func(searchSet *fhir.Bundle) (bool, error) {
		entries = append(entries, searchSet.Entry...)
		if len(entries) >= maxUpdateEntries {
			return false, fmt.Errorf("too many entries (%d), aborting update to prevent excessive memory usage", len(entries))
		}
		return true, nil
	})
	if err != nil {
		return nil, fhir.Bundle{}, fmt.Errorf("%s: %w", paginationErrMsg, err)
	}

	return entries, searchSet, nil
}

func (c *Component) queryHistory(ctx context.Context, remoteAdminDirectoryFHIRClient fhirclient.Client, resourceType string, searchParams url.Values) ([]fhir.BundleEntry, fhir.Bundle, error) {
	return c.queryFHIR(ctx, remoteAdminDirectoryFHIRClient, resourceType, searchParams, true)
}

func (c *Component) query(ctx context.Context, remoteAdminDirectoryFHIRClient fhirclient.Client, resourceType string, searchParams url.Values) ([]fhir.BundleEntry, fhir.Bundle, error) {
	return c.queryFHIR(ctx, remoteAdminDirectoryFHIRClient, resourceType, searchParams, false)
}

// deduplicateHistoryEntries keeps only the most recent version of each resource
func deduplicateHistoryEntries(entries []fhir.BundleEntry) []fhir.BundleEntry {
	resourceMap := make(map[string]fhir.BundleEntry)
	var entriesWithoutID []fhir.BundleEntry

	for _, entry := range entries {
		var resourceID string

		if entry.Resource == nil {
			if entry.Request != nil && entry.Request.Method == fhir.HTTPVerbDELETE {
				resourceID = extractResourceIDFromURL(entry)
			}
		} else {
			if info, err := libfhir.ExtractResourceInfo(entry.Resource); err == nil {
				resourceID = info.ID
			}
		}

		if resourceID != "" {
			existing, exists := resourceMap[resourceID]
			if !exists || isMoreRecent(entry, existing) {
				resourceMap[resourceID] = entry
			}
		} else {
			entriesWithoutID = append(entriesWithoutID, entry)
		}
	}

	var result []fhir.BundleEntry
	for _, entry := range resourceMap {
		result = append(result, entry)
	}
	result = append(result, entriesWithoutID...)
	return result
}

// isMoreRecent compares two entries, returns true if first is more recent
func isMoreRecent(entry1, entry2 fhir.BundleEntry) bool {
	time1 := getLastUpdated(entry1)
	time2 := getLastUpdated(entry2)
	if !time1.IsZero() && !time2.IsZero() {
		return time1.After(time2)
	}
	// Fallback: cannot determine which is more recent, do not overwrite
	return false
}

// getLastUpdated extracts lastUpdated timestamp from an entry
func getLastUpdated(entry fhir.BundleEntry) time.Time {
	if entry.Resource == nil {
		return time.Time{}
	}
	info, err := libfhir.ExtractResourceInfo(entry.Resource)
	if err != nil || info.LastUpdated == nil {
		return time.Time{}
	}
	return *info.LastUpdated
}

// extractResourceIDFromURL extracts the resource ID from a DELETE operation's URL
func extractResourceIDFromURL(entry fhir.BundleEntry) string {
	// First try to extract from Request.Url (e.g., "Organization/123")
	if entry.Request != nil && entry.Request.Url != "" {
		parts := strings.Split(entry.Request.Url, "/")
		if len(parts) >= 2 {
			return parts[1] // Return the ID part
		}
	}

	// Fallback: extract from fullUrl (e.g., "http://example.org/fhir/Organization/123")
	if entry.FullUrl != nil {
		parts := strings.Split(*entry.FullUrl, "/")
		if len(parts) >= 1 {
			return parts[len(parts)-1] // Return the last part (ID)
		}
	}

	return ""
}

// queryAllResourceTypes queries all specified resource types from the FHIR server and returns combined entries.
func (c *Component) queryAllResourceTypes(ctx context.Context, fhirClient fhirclient.Client, resourceTypes []string, searchParams url.Values) ([]fhir.BundleEntry, fhir.Bundle, error) {
	var entries []fhir.BundleEntry
	var firstSearchSet fhir.Bundle

	for i, resourceType := range resourceTypes {
		// Create a copy of searchParams for this resource type
		params := make(url.Values)
		for k, v := range searchParams {
			params[k] = v
		}

		// Remove _single parameter for Organization resource type
		if resourceType == "Organization" {
			params.Del("_since")
		}

		currEntries, currSearchSet, err := c.queryHistory(ctx, fhirClient, resourceType, params)
		if err != nil {
			return nil, fhir.Bundle{}, fmt.Errorf("failed to query %s history: %w", resourceType, err)
		}
		entries = append(entries, currEntries...)
		if i == 0 {
			firstSearchSet = currSearchSet
		}
	}

	return entries, firstSearchSet, nil
}

// noURASentinel marks "this Organization version had no URA identifier" in the set of URA values
// tracked per organization, so that gaining or losing a URA counts as a change.
const noURASentinel = ""

// checkForURAIdentifierChanges detects if any Organization's URA identifier has changed between
// history versions. An organization whose history shows more than one distinct URA value — counting
// "no URA" (noURASentinel) as a distinct value — has a changed URA identifier.
func checkForURAIdentifierChanges(entries []fhir.BundleEntry) bool {
	uraValuesByOrg := make(map[string]map[string]bool) // orgID -> set of URA values seen across versions

	for _, entry := range entries {
		if entry.Resource == nil {
			continue
		}
		var org fhir.Organization
		if err := json.Unmarshal(entry.Resource, &org); err != nil {
			continue // Not an Organization, skip
		}
		if org.Id == nil {
			continue
		}

		seen := uraValuesByOrg[*org.Id]
		if seen == nil {
			seen = make(map[string]bool)
			uraValuesByOrg[*org.Id] = seen
		}
		for _, value := range organizationURAValues(org) {
			seen[value] = true
		}
	}

	for _, values := range uraValuesByOrg {
		if len(values) > 1 {
			return true
		}
	}
	return false
}

// organizationURAValues returns the organization's URA identifier values, or a single
// noURASentinel entry when it has no URA identifier at all.
func organizationURAValues(org fhir.Organization) []string {
	uraIdentifiers := libfhir.FilterIdentifiersBySystem(org.Identifier, coding.URANamingSystem)
	if len(uraIdentifiers) == 0 {
		return []string{noURASentinel}
	}
	var values []string
	for _, ura := range uraIdentifiers {
		if ura.Value != nil {
			values = append(values, *ura.Value)
		}
	}
	return values
}

func (c *Component) ensureParentOrganizationsMap(ctx context.Context, fhirBaseURLRaw string, remoteAdminDirectoryFHIRClient fhirclient.Client, authoritativeUra string) (parentOrganizationMap, error) {
	slog.DebugContext(ctx, "Querying organizations for authoritative check (parent organization map build)", logging.FHIRServer(fhirBaseURLRaw))
	orgEntries, _, err := c.query(ctx, remoteAdminDirectoryFHIRClient, "Organization", url.Values{
		"_count": []string{strconv.Itoa(searchPageSize)},
	})
	if err != nil {
		slog.ErrorContext(ctx, "Failed to query all organizations, aborting parent organization map build", logging.FHIRServer(fhirBaseURLRaw), logging.Error(err))
		return nil, err
	}

	parentOrganizationsMap, err := createOrganizationTree(orgEntries)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to build parent organization map from all organizations, aborting parent organization map build", logging.FHIRServer(fhirBaseURLRaw), logging.Error(err))
		return nil, err
	}

	// Filter to only include parent organizations matching the authoritative URA if provided
	if authoritativeUra != "" {
		filtered := make(parentOrganizationMap)
		for parentOrg, linkedOrgs := range parentOrganizationsMap {
			uraIdentifiers := libfhir.FilterIdentifiersBySystem(parentOrg.Identifier, coding.URANamingSystem)
			for _, ura := range uraIdentifiers {
				if ura.Value != nil && *ura.Value == authoritativeUra {
					filtered[parentOrg] = linkedOrgs
					break
				}
			}
		}
		parentOrganizationsMap = filtered
	}

	return parentOrganizationsMap, nil
}

// If no organization with URA is found directly, it traverses each organization's partOf chain to find a parent with URA.
// Returns the parent organization with the most linked organizations and a slice of all organizations whose
// partOf chain leads to the parent.
// Returns (nil, nil) if no organization with URA identifier is found (not an error condition).
func createOrganizationTree(entries []fhir.BundleEntry) (parentOrganizationMap, error) {
	result := make(parentOrganizationMap)

	// Build a map of all organizations for efficient lookup using ID as key
	orgMap := make(map[string]*fhir.Organization)
	for _, entry := range entries {
		if entry.Resource == nil {
			continue
		}
		var org fhir.Organization
		if err := json.Unmarshal(entry.Resource, &org); err != nil {
			continue
		}
		if org.Id != nil {
			orgMap[*org.Id] = &org
		}
	}

	// Loop through all organizations to find all with URA identifier
	for _, org := range orgMap {
		uraIdentifiers := libfhir.FilterIdentifiersBySystem(org.Identifier, coding.URANamingSystem)
		if len(uraIdentifiers) > 0 {
			// Found an organization with URA, find all organizations linked to it
			linkedOrgs := findOrganizationsLinkedToParent(orgMap, org)
			result[org] = linkedOrgs
		}
	}

	return result, nil
}

// findOrganizationsLinkedToParent returns all organizations whose partOf chain leads to the parent organization.
// It excludes the parent organization itself from the returned slice.
// Returns an empty slice (not nil) if no organizations are linked to the parent.
func findOrganizationsLinkedToParent(orgMap map[string]*fhir.Organization, parentOrg *fhir.Organization) []*fhir.Organization {
	linked := make([]*fhir.Organization, 0)

	for _, org := range orgMap {
		// Skip the parent organization itself
		if org.Id != nil && parentOrg.Id != nil && *org.Id == *parentOrg.Id {
			continue
		}

		// Check if this organization's partOf chain leads to the parent
		if organizationLinksToParent(orgMap, org, parentOrg) {
			linked = append(linked, org)
		}
	}

	return linked
}

// organizationLinksToParent checks if an organization's partOf chain eventually leads to the parent organization.
// It handles circular references by tracking visited organizations.
func organizationLinksToParent(orgMap map[string]*fhir.Organization, org *fhir.Organization, parentOrg *fhir.Organization) bool {
	const maxDepth = 10
	visited := make(map[string]bool)
	return organizationLinksToParentRecursive(orgMap, org, parentOrg, visited, 0, maxDepth)
}

// organizationLinksToParentRecursive is the recursive helper for organizationLinksToParent.
func organizationLinksToParentRecursive(orgMap map[string]*fhir.Organization, org *fhir.Organization, parentOrg *fhir.Organization, visited map[string]bool, depth int, maxDepth int) bool {
	if depth > maxDepth {
		return false // Depth exceeded
	}

	if org.Id != nil {
		if visited[*org.Id] {
			return false // Circular reference detected
		}
		visited[*org.Id] = true

		// Check if we found the parent
		if parentOrg.Id != nil && *org.Id == *parentOrg.Id {
			return true
		}
	}

	// Check if this organization has a partOf reference
	if org.PartOf == nil || org.PartOf.Reference == nil {
		return false // No more parents in the chain
	}

	// Extract the parent ID from the reference
	ref := *org.PartOf.Reference
	var parentID string
	if strings.Contains(ref, "/") {
		parts := strings.Split(ref, "/")
		parentID = parts[len(parts)-1]
	} else {
		parentID = ref
	}

	// Look up the parent organization
	nextOrg, exists := orgMap[parentID]
	if !exists {
		return false // Parent not found in map
	}

	// Recursively check the parent's chain
	return organizationLinksToParentRecursive(orgMap, nextOrg, parentOrg, visited, depth+1, maxDepth)
}
