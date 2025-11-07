package mcsd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/caramel/to"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var _ component.Lifecycle = &Component{}

var rootDirectoryResourceTypes = []string{"Organization", "Endpoint"}
var directoryResourceTypes = []string{"Organization", "Endpoint", "Location", "HealthcareService", "PractitionerRole"}

// clockSkewBuffer is subtracted from local time when Bundle meta.lastUpdated is not available
// to account for potential clock differences between client and FHIR server
var clockSkewBuffer = 2 * time.Second

// maxUpdateEntries limits the number of entries processed in a single FHIR transaction to prevent excessive load on the FHIR server
const maxUpdateEntries = 1000

// searchPageSize is an arbitrary FHIR search result limit (per page), so we have deterministic behavior across FHIR servers,
// and don't rely on server defaults (which may be very high or very low (Azure FHIR's default is 10)).
const searchPageSize = 100

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
type Component struct {
	config       Config
	fhirClientFn func(baseURL *url.URL) fhirclient.Client

	administrationDirectories []administrationDirectory
	lastUpdateTimes           map[string]string
	updateMux                 *sync.RWMutex
}

type Config struct {
	AdministrationDirectories map[string]DirectoryConfig `koanf:"admin"`
	QueryDirectory            DirectoryConfig            `koanf:"query"`
	ExcludeAdminDirectories   []string                   `koanf:"adminexclude"`
}

type DirectoryConfig struct {
	FHIRBaseURL string `koanf:"fhirbaseurl"`
}

type UpdateReport map[string]DirectoryUpdateReport

type administrationDirectory struct {
	fhirBaseURL   string
	resourceTypes []string
	discover      bool
	sourceURL     string // The fullUrl from the Bundle entry that created this Endpoint, used for unregistration on DELETE
}

type DirectoryUpdateReport struct {
	CountCreated int      `json:"created"`
	CountUpdated int      `json:"updated"`
	CountDeleted int      `json:"deleted"`
	Warnings     []string `json:"warnings"`
	Errors       []string `json:"errors"`
}

func New(config Config) (*Component, error) {
	result := &Component{
		config: config,
		fhirClientFn: func(baseURL *url.URL) fhirclient.Client {
			return fhirclient.New(baseURL, http.DefaultClient, &fhirclient.Config{
				UsePostSearch: false,
			})
		},
		lastUpdateTimes: make(map[string]string),
		updateMux:       &sync.RWMutex{},
	}
	for _, rootDirectory := range config.AdministrationDirectories {
		if err := result.registerAdministrationDirectory(context.Background(), rootDirectory.FHIRBaseURL, rootDirectoryResourceTypes, true, ""); err != nil {
			return nil, fmt.Errorf("register root administration directory (url=%s): %w", rootDirectory.FHIRBaseURL, err)
		}
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
			log.Ctx(ctx).Error().Err(err).Msg("mCSD update failed")
			http.Error(w, "Failed to update mCSD: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	})
}

func (c *Component) registerAdministrationDirectory(ctx context.Context, fhirBaseURL string, resourceTypes []string, discover bool, sourceURL string) error {
	// Must be a valid http or https URL
	parsedFHIRBaseURL, err := url.Parse(fhirBaseURL)
	if err != nil {
		return fmt.Errorf("invalid FHIR base URL (url=%s): %w", fhirBaseURL, err)
	}
	parsedFHIRBaseURL.Scheme = strings.ToLower(parsedFHIRBaseURL.Scheme)
	if (parsedFHIRBaseURL.Scheme != "https" && parsedFHIRBaseURL.Scheme != "http") || parsedFHIRBaseURL.Host == "" {
		return fmt.Errorf("invalid FHIR base URL (url=%s)", fhirBaseURL)
	}

	// Check if the URL is in the exclusion list (also trim exclusion list entries for consistent matching)
	trimmedFHIRBaseURL := strings.TrimRight(fhirBaseURL, "/")
	for _, excludedURL := range c.config.ExcludeAdminDirectories {
		if strings.TrimRight(excludedURL, "/") == trimmedFHIRBaseURL {
			log.Ctx(ctx).Info().Str("fhir_server", fhirBaseURL).Msg("Skipping administration directory registration: excluded by configuration")
			return nil
		}
	}

	exists := slices.ContainsFunc(c.administrationDirectories, func(directory administrationDirectory) bool {
		return directory.fhirBaseURL == fhirBaseURL
	})
	if exists {
		return nil
	}
	c.administrationDirectories = append(c.administrationDirectories, administrationDirectory{
		resourceTypes: resourceTypes,
		fhirBaseURL:   fhirBaseURL,
		discover:      discover,
		sourceURL:     sourceURL,
	})
	log.Ctx(ctx).Info().Str("fhir_server", fhirBaseURL).Msgf("Registered mCSD Directory (discover=%v)", discover)
	return nil
}

// unregisterAdministrationDirectory removes an administration directory from the list by its fullUrl.
// This is called when an Endpoint is deleted to prevent it from being fetched in future updates.
// The fullUrl parameter is the Bundle entry fullUrl that was used when the Endpoint was registered.
func (c *Component) unregisterAdministrationDirectory(ctx context.Context, fullUrl string) {
	initialCount := len(c.administrationDirectories)
	c.administrationDirectories = slices.DeleteFunc(c.administrationDirectories, func(dir administrationDirectory) bool {
		return dir.sourceURL == fullUrl
	})
	if len(c.administrationDirectories) < initialCount {
		log.Ctx(ctx).Info().Str("full_url", fullUrl).Msg("Unregistered mCSD Directory after Endpoint deletion")
	}
}

// processEndpointDeletes processes DELETE operations for Endpoints and unregisters them from administrationDirectories.
// This prevents deleted Endpoints from being fetched in future updates, fixing issue #241.
func (c *Component) processEndpointDeletes(ctx context.Context, entries []fhir.BundleEntry) {
	for _, entry := range entries {
		if entry.Request != nil && entry.Request.Method == fhir.HTTPVerbDELETE && entry.FullUrl != nil {
			parts := strings.Split(entry.Request.Url, "/")
			if len(parts) >= 2 && parts[0] == "Endpoint" {
				// Unregister the administration directory using the fullUrl
				// The fullUrl uniquely identifies the resource that was deleted
				c.unregisterAdministrationDirectory(ctx, *entry.FullUrl)
			}
		}
	}
}

func (c *Component) update(ctx context.Context) (UpdateReport, error) {
	c.updateMux.Lock()
	defer c.updateMux.Unlock()

	result := make(UpdateReport)
	for i := 0; i < len(c.administrationDirectories); i++ {
		adminDirectory := c.administrationDirectories[i]
		report, err := c.updateFromDirectory(ctx, adminDirectory.fhirBaseURL, adminDirectory.resourceTypes, adminDirectory.discover)
		if err != nil {
			log.Ctx(ctx).Err(err).Str("fhir_server", adminDirectory.fhirBaseURL).Msg("mCSD Directory update failed")
			report.Errors = append(report.Errors, err.Error())
		}
		// Return empty slices instead of null ones, makes a nicer REST API
		if report.Warnings == nil {
			report.Warnings = []string{}
		}
		if report.Errors == nil {
			report.Errors = []string{}
		}
		result[adminDirectory.fhirBaseURL] = report
	}
	return result, nil
}

func (c *Component) updateFromDirectory(ctx context.Context, fhirBaseURLRaw string, allowedResourceTypes []string, allowDiscovery bool) (DirectoryUpdateReport, error) {
	log.Ctx(ctx).Info().Str("fhir_server", fhirBaseURLRaw).Msg("Updating from mCSD Directory (discover=" + fmt.Sprint(allowDiscovery) + ", resourceTypes=" + strings.Join(allowedResourceTypes, ",") + ")")
	remoteAdminDirectoryFHIRBaseURL, err := url.Parse(fhirBaseURLRaw)
	if err != nil {
		return DirectoryUpdateReport{}, err
	}
	remoteAdminDirectoryFHIRClient := c.fhirClientFn(remoteAdminDirectoryFHIRBaseURL)

	queryDirectoryFHIRBaseURL, err := url.Parse(c.config.QueryDirectory.FHIRBaseURL)
	if err != nil {
		return DirectoryUpdateReport{}, err
	}
	queryDirectoryFHIRClient := c.fhirClientFn(queryDirectoryFHIRBaseURL)

	// Get last update time for incremental sync
	lastUpdate, hasLastUpdate := c.lastUpdateTimes[fhirBaseURLRaw]

	// Capture query start time as fallback for servers that don't provide Bundle meta.lastUpdated.
	queryStartTime := time.Now()

	searchParams := url.Values{
		"_count": []string{strconv.Itoa(searchPageSize)},
	}
	if hasLastUpdate {
		searchParams.Set("_since", lastUpdate)
		log.Ctx(ctx).Debug().Str("fhir_server", fhirBaseURLRaw).Str("_since", lastUpdate).Msg("Using _since parameter for incremental sync from FHIR server")
	} else {
		log.Ctx(ctx).Info().Str("fhir_server", fhirBaseURLRaw).Msg("No last update time, doing full sync from FHIR server")
	}

	var entries []fhir.BundleEntry
	var firstSearchSet fhir.Bundle
	for i, resourceType := range directoryResourceTypes {
		currEntries, currSearchSet, err := c.queryHistory(ctx, remoteAdminDirectoryFHIRClient, resourceType, searchParams)
		if err != nil {
			return DirectoryUpdateReport{}, fmt.Errorf("failed to query %s history: %w", resourceType, err)
		}
		entries = append(entries, currEntries...)
		if i == 0 {
			firstSearchSet = currSearchSet
		}
	}

	// Deduplicate resources from _history query - keep only the most recent version
	// _history can return multiple versions of the same resource, but transaction bundles must have unique resources
	deduplicatedEntries := deduplicateHistoryEntries(entries)

	// Pre-process Endpoint DELETEs to unregister administration directories
	if allowDiscovery {
		c.processEndpointDeletes(ctx, deduplicatedEntries)
	}

	// Build transaction with deterministic conditional references
	tx := fhir.Bundle{
		Type:  fhir.BundleTypeTransaction,
		Entry: make([]fhir.BundleEntry, 0, len(deduplicatedEntries)),
	}

	var report DirectoryUpdateReport
	for i, entry := range deduplicatedEntries {
		if entry.Request == nil {
			msg := fmt.Sprintf("Skipping entry with no request: #%d", i)
			report.Warnings = append(report.Warnings, msg)
			continue
		}
		log.Ctx(ctx).Trace().Str("fhir_server", fhirBaseURLRaw).Msgf("Processing entry: %s", entry.Request.Url)
		resourceType, err := buildUpdateTransaction(ctx, &tx, entry, allowedResourceTypes, allowDiscovery, fhirBaseURLRaw)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("entry #%d: %s", i, err.Error()))
			continue
		}

		// Handle Endpoint discovery and registration
		if allowDiscovery && resourceType == "Endpoint" && entry.Resource != nil {
			var endpoint fhir.Endpoint
			if err := json.Unmarshal(entry.Resource, &endpoint); err != nil {
				report.Warnings = append(report.Warnings, fmt.Sprintf("entry #%d: failed to unmarshal Endpoint resource: %s", i, err.Error()))
				continue
			}

			var payloadCoding = fhir.Coding{
				System: to.Ptr(coding.MCSDPayloadTypeSystem),
				Code:   to.Ptr(coding.MCSDPayloadTypeDirectoryCode),
			}

			if coding.CodablesIncludesCode(endpoint.PayloadType, payloadCoding) {
				// Use the fullUrl from the Bundle entry to track this Endpoint
				// The fullUrl uniquely identifies the resource and can be used for later unregistration
				if entry.FullUrl == nil {
					report.Warnings = append(report.Warnings, fmt.Sprintf("entry #%d: Endpoint missing fullUrl", i))
					continue
				}
				log.Ctx(ctx).Debug().Msgf("Discovered mCSD Directory: %s", endpoint.Address)
				err := c.registerAdministrationDirectory(ctx, endpoint.Address, directoryResourceTypes, false, *entry.FullUrl)
				if err != nil {
					report.Warnings = append(report.Warnings, fmt.Sprintf("entry #%d: failed to register discovered mCSD Directory at %s: %s", i, endpoint.Address, err.Error()))
				}
			}
		}
	}

	log.Ctx(ctx).Debug().Str("fhir_server", fhirBaseURLRaw).Msgf("Got %d mCSD entries", len(tx.Entry))
	if len(tx.Entry) == 0 {
		return report, nil
	}

	var txResult fhir.Bundle
	if err := queryDirectoryFHIRClient.CreateWithContext(ctx, tx, &txResult, fhirclient.AtPath("/")); err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("failed to apply mCSD update to query directory: %w", err)
	}

	// Process result
	for i, entry := range txResult.Entry {
		if entry.Response == nil {
			msg := fmt.Sprintf("Skipping entry with no response: #%d", i)
			report.Warnings = append(report.Warnings, msg)
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

	// Update last sync timestamp on successful completion.
	// Use the search result Bundle's meta.lastUpdated if available, otherwise fall back to query start time.
	// This uses the FHIR server's own timestamp string, eliminating clock skew issues.
	var nextSyncTime string
	if firstSearchSet.Meta != nil && firstSearchSet.Meta.LastUpdated != nil {
		nextSyncTime = *firstSearchSet.Meta.LastUpdated
	} else {
		// Fallback to local time with buffer to account for potential clock skew
		nextSyncTime = queryStartTime.Add(-clockSkewBuffer).Format(time.RFC3339Nano)
		log.Ctx(ctx).Warn().Str("fhir_server", fhirBaseURLRaw).Msg("Bundle meta.lastUpdated not available, using local time with buffer - may cause clock skew issues")
	}
	c.lastUpdateTimes[fhirBaseURLRaw] = nextSyncTime

	return report, nil
}

func (c *Component) queryHistory(ctx context.Context, remoteAdminDirectoryFHIRClient fhirclient.Client, resourceType string, searchParams url.Values) ([]fhir.BundleEntry, fhir.Bundle, error) {
	var searchSet fhir.Bundle
	// First, collect all resources from _history
	err := remoteAdminDirectoryFHIRClient.SearchWithContext(ctx, "", searchParams, &searchSet, fhirclient.AtPath(resourceType+"/_history"))
	if err != nil {
		return nil, fhir.Bundle{}, fmt.Errorf("_history search failed: %w", err)
	}
	var entries []fhir.BundleEntry
	err = fhirclient.Paginate(ctx, remoteAdminDirectoryFHIRClient, searchSet, func(searchSet *fhir.Bundle) (bool, error) {
		entries = append(entries, searchSet.Entry...)
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
