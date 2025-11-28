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
type Component struct {
	config       Config
	fhirClientFn func(baseURL *url.URL) fhirclient.Client

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
}

type DirectoryConfig struct {
	FHIRBaseURL string `koanf:"fhirbaseurl"`
}

type UpdateReport map[string]DirectoryUpdateReport

type administrationDirectory struct {
	fhirBaseURL      string
	resourceTypes    []string
	discover         bool
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

func New(config Config) (*Component, error) {
	result := &Component{
		config: config,
		fhirClientFn: func(baseURL *url.URL) fhirclient.Client {
			return fhirclient.New(baseURL, http.DefaultClient, &fhirclient.Config{
				UsePostSearch: false,
			})
		},
		directoryResourceTypes: config.DirectoryResourceTypes,
		lastUpdateTimes:        make(map[string]string),
		updateMux:              &sync.RWMutex{},
	}
	for _, rootDirectory := range config.AdministrationDirectories {
		if err := result.registerAdministrationDirectory(context.Background(), rootDirectory.FHIRBaseURL, rootDirectoryResourceTypes, true, "", ""); err != nil {
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
			log.Ctx(ctx).Error().Err(err).Msg("mCSD update failed")
			http.Error(w, "Failed to update mCSD: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	})
}

func (c *Component) registerAdministrationDirectory(ctx context.Context, fhirBaseURL string, resourceTypes []string, discover bool, sourceURL string, authoritativeUra string) error {
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
		resourceTypes:    resourceTypes,
		fhirBaseURL:      fhirBaseURL,
		discover:         discover,
		sourceURL:        sourceURL,
		authoritativeUra: authoritativeUra,
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
		report, err := c.updateFromDirectory(ctx, adminDirectory.fhirBaseURL, adminDirectory.resourceTypes, adminDirectory.discover, adminDirectory.authoritativeUra)
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

// discoverAndRegisterEndpoints processes endpoint discovery and registration for the given parent organizations.
// It finds endpoints from the entries that match parent organization endpoint references and registers them.
func (c *Component) discoverAndRegisterEndpoints(ctx context.Context, entries []fhir.BundleEntry, parentOrganizationsMap parentOrganizationMap, report DirectoryUpdateReport) DirectoryUpdateReport {
	if parentOrganizationsMap == nil {
		return report
	}

	for parentOrg := range parentOrganizationsMap {
		uraIdentifiers := libfhir.FilterIdentifiersBySystem(parentOrg.Identifier, coding.URANamingSystem)
		if len(uraIdentifiers) == 0 || uraIdentifiers[0].Value == nil {
			continue
		}
		authoritativeUra := *uraIdentifiers[0].Value

		if parentOrg.Endpoint == nil || len(parentOrg.Endpoint) == 0 {
			continue
		}

		// find endpoint in entries
		endpoints := make(map[string]*fhir.Endpoint)
		for _, entry := range entries {
			if entry.Resource == nil {
				continue
			}
			var endpoint fhir.Endpoint
			if err := json.Unmarshal(entry.Resource, &endpoint); err != nil {
				continue
			}
			// find all Endpoint resources from entries that reference the parent organization's Endpoint resources'
			if endpoint.Id != nil {
				endpointID := *endpoint.Id
				for _, parentEndpoint := range parentOrg.Endpoint {
					if parentEndpoint.Reference != nil {
						refID := extractReferenceID(parentEndpoint.Reference)
						if endpointID == refID {
							if entry.FullUrl != nil {
								endpoints[*entry.FullUrl] = &endpoint
							}
							break // Found a match, move to next entry
						}
					}
				}
			}
		}

		payloadCoding := fhir.Coding{
			System: to.Ptr(coding.MCSDPayloadTypeSystem),
			Code:   to.Ptr(coding.MCSDPayloadTypeDirectoryCode),
		}

		for fullUrl, endpoint := range endpoints {
			if coding.CodablesIncludesCode(endpoint.PayloadType, payloadCoding) {
				log.Ctx(ctx).Debug().Msgf("Discovered mCSD Directory: %s", endpoint.Address)

				err := c.registerAdministrationDirectory(ctx, endpoint.Address, c.directoryResourceTypes, false, fullUrl, authoritativeUra)
				if err != nil {
					report.Warnings = append(report.Warnings, fmt.Sprintf("failed to register discovered mCSD Directory at %s: %s", endpoint.Address, err.Error()))
				}
			}
		}
	}

	return report
}

func (c *Component) updateFromDirectory(ctx context.Context, fhirBaseURLRaw string, allowedResourceTypes []string, allowDiscovery bool, authoritativeUra string) (DirectoryUpdateReport, error) {
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
	for i, resourceType := range allowedResourceTypes {
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

	// Find parent organizations with URA identifier and all organizations linked to them
	// This is used when validating organizations that don't have their own URA identifier
	// If no parent organizations are found in deduplicatedEntries, queries all organizations from the directory
	// If authoritativeUra is provided, filters to only include parent organizations with that URA
	parentOrganizationsMap, err := c.ensureParentOrganizationsMap(ctx, fhirBaseURLRaw, remoteAdminDirectoryFHIRClient, deduplicatedEntries, authoritativeUra)

	if err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("failed to build parent organization map: %w", err)
	}

	// Validate all parent organizations once before processing resources
	if err := ValidateParentOrganizations(parentOrganizationsMap); err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("parent organization (one that supposedly has ura identifier - and only only) validation failed: %w", err)
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
		_, err := buildUpdateTransaction(ctx, &tx, entry, ValidationRules{AllowedResourceTypes: allowedResourceTypes}, parentOrganizationsMap, allowDiscovery, fhirBaseURLRaw)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("entry #%d: %s", i, err.Error()))
			continue
		}
	}

	// Handle Endpoint discovery and registration
	if allowDiscovery {
		report = c.discoverAndRegisterEndpoints(ctx, entries, parentOrganizationsMap, report)
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

// buildOrganizationKey creates a composite key from organization ID and VersionId for deduplication.
// The key includes both the ID and VersionId (if available) to differentiate between versions.
func buildOrganizationKey(org *fhir.Organization) string {
	key := *org.Id
	if org.Meta != nil && org.Meta.VersionId != nil {
		key = key + ":" + *org.Meta.VersionId
	}
	return key
}

// findParentOrganizationsWithURA finds a parent organization with a URA identifier and all organizations linked to it.
// It loops through ALL entries to find organizations with URA identifiers
// ensureParentOrganizationsMap builds the parent organizations map from deduplicatedEntries.
// If no parent organizations with URA are found, it queries all organizations from the directory as a fallback.
// If authoritativeUra is provided, filters the map to only include parent organizations with that URA identifier.
func (c *Component) ensureParentOrganizationsMap(ctx context.Context, fhirBaseURLRaw string, remoteAdminDirectoryFHIRClient fhirclient.Client, deduplicatedEntries []fhir.BundleEntry, authoritativeUra string) (parentOrganizationMap, error) {
	// First try to find parent organizations from deduplicatedEntries
	parentOrganizationsMap, err := findParentOrganizationsWithURA(ctx, deduplicatedEntries)
	if err != nil {
		return nil, err
	}

	// If no parent organizations with URA were found, query all organizations from the directory
	if len(parentOrganizationsMap) == 0 {
		log.Ctx(ctx).Debug().Str("fhir_server", fhirBaseURLRaw).Msg("No parent organizations with URA found in deduplicatedEntries, querying all organizations")
		orgEntries, _, err := c.queryHistory(ctx, remoteAdminDirectoryFHIRClient, "Organization", url.Values{
			"_count": []string{strconv.Itoa(searchPageSize)},
		})
		if err != nil {
			log.Ctx(ctx).Err(err).Str("fhir_server", fhirBaseURLRaw).Err(err).Msg("Failed to query all organizations, aborting parent organization map build")
			return nil, err
		}

		parentOrganizationsMap, err = findParentOrganizationsWithURA(ctx, orgEntries)
		if err != nil {
			log.Ctx(ctx).Err(err).Str("fhir_server", fhirBaseURLRaw).Err(err).Msg("Failed to build parent organization map from all organizations, aborting parent organization map build")
			return nil, err
		}
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
func findParentOrganizationsWithURA(ctx context.Context, entries []fhir.BundleEntry) (parentOrganizationMap, error) {
	result := make(parentOrganizationMap)

	// Build a map of all organizations for efficient lookup using ID and VersionId as composite key
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
			key := buildOrganizationKey(&org)
			orgMap[key] = &org
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
