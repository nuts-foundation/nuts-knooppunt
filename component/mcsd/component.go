package mcsd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/rs/zerolog/log"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var _ component.Lifecycle = &Component{}

var rootDirectoryResourceTypes = []string{"Organization", "Endpoint"}
var directoryResourceTypes = []string{"Organization", "Endpoint", "Location", "HealthcareService"}

// clockSkewBuffer is subtracted from local time when Bundle meta.lastUpdated is not available
// to account for potential clock differences between client and FHIR server
const clockSkewBuffer = 2 * time.Second

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
}

type DirectoryConfig struct {
	FHIRBaseURL string `koanf:"fhirbaseurl"`
}

type UpdateReport map[string]DirectoryUpdateReport

type administrationDirectory struct {
	fhirBaseURL   string
	resourceTypes []string
	discover      bool
}

type DirectoryUpdateReport struct {
	CountCreated int      `json:"created"`
	CountUpdated int      `json:"updated"`
	CountDeleted int      `json:"deleted"`
	Warnings     []string `json:"warnings"`
	Errors       []string `json:"errors"`
}

func New(config Config) *Component {
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
		result.registerAdministrationDirectory(rootDirectory.FHIRBaseURL, rootDirectoryResourceTypes, true)
	}
	return result
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

func (c *Component) registerAdministrationDirectory(fhirBaseURL string, resourceTypes []string, discover bool) error {
	// Must be a valid http or https URL
	parsedFHIRBaseURL, err := url.Parse(fhirBaseURL)
	if err != nil {
		return fmt.Errorf("invalid FHIR base URL (url=%s): %w", fhirBaseURL, err)
	}
	parsedFHIRBaseURL.Scheme = strings.ToLower(parsedFHIRBaseURL.Scheme)
	if (parsedFHIRBaseURL.Scheme != "https" && parsedFHIRBaseURL.Scheme != "http") || parsedFHIRBaseURL.Host == "" {
		return fmt.Errorf("invalid FHIR base URL (url=%s)", fhirBaseURL)
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
	})
	return nil
}

func (c *Component) update(ctx context.Context) (UpdateReport, error) {
	c.updateMux.Lock()
	defer c.updateMux.Unlock()

	result := make(UpdateReport)
	for i := 0; i < len(c.administrationDirectories); i++ {
		adminDirectory := c.administrationDirectories[i]
		report, err := c.updateFromDirectory(ctx, adminDirectory.fhirBaseURL, adminDirectory.resourceTypes, adminDirectory.discover)
		if err != nil {
			log.Ctx(ctx).Err(err).Str("directory", adminDirectory.fhirBaseURL).Msg("mCSD Directory update failed")
			report.Errors = append(report.Errors, err.Error())
		}
		result[adminDirectory.fhirBaseURL] = report
	}
	return result, nil
}

func (c *Component) updateFromDirectory(ctx context.Context, fhirBaseURLRaw string, allowedResourceTypes []string, allowDiscovery bool) (DirectoryUpdateReport, error) {
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

	// Query remote directory
	var bundle fhir.Bundle
	// TODO: Pagination

	// Get last update time for incremental sync
	lastUpdate, hasLastUpdate := c.lastUpdateTimes[fhirBaseURLRaw]

	// Capture query start time as fallback for servers that don't provide Bundle meta.lastUpdated.
	queryStartTime := time.Now()

	searchParams := url.Values{}
	if hasLastUpdate {
		searchParams.Set("_since", lastUpdate)
		log.Ctx(ctx).Debug().Str("fhir_server", fhirBaseURLRaw).Str("_since", lastUpdate).Msg("Using _since parameter for incremental sync from FHIR server")
	} else {
		log.Ctx(ctx).Info().Str("fhir_server", fhirBaseURLRaw).Msg("No last update time, doing full sync from FHIR server")
	}

	if err = remoteAdminDirectoryFHIRClient.SearchWithContext(ctx, "", searchParams, &bundle, fhirclient.AtPath("/_history")); err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("_history search failed: %w", err)
	}

	// Update local directory
	tx := fhir.Bundle{
		Type:  fhir.BundleTypeTransaction,
		Entry: make([]fhir.BundleEntry, 0, len(bundle.Entry)),
	}
	remoteRefToLocalRefMap := make(map[string]string)
	var report DirectoryUpdateReport
	for i, entry := range bundle.Entry {
		resourceType, err := buildUpdateTransaction(&tx, entry, allowedResourceTypes, allowDiscovery, remoteRefToLocalRefMap)
		if err != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("entry #%d: %s", i, err.Error()))
			continue
		}
		if allowDiscovery && resourceType == "Endpoint" {
			var endpoint fhir.Endpoint
			if err := json.Unmarshal(entry.Resource, &endpoint); err != nil {
				report.Warnings = append(report.Warnings, fmt.Sprintf("entry #%d: failed to unmarshal Endpoint resource: %s", i, err.Error()))
				continue
			}
			if coding.EqualsCode(endpoint.ConnectionType, coding.MCSDConnectionTypeSystem, coding.MCSDConnectionTypeDirectoryCode) {
				err := c.registerAdministrationDirectory(endpoint.Address, directoryResourceTypes, false)
				if err != nil {
					report.Warnings = append(report.Warnings, fmt.Sprintf("entry #%d: failed to register discovered mCSD Directory at %s: %s", i, endpoint.Address, err.Error()))
				} else {
					log.Ctx(ctx).Info().Msgf("Discovered and registered new mCSD Directory at %s", endpoint.Address)
				}
			}
		}
	}
	if len(tx.Entry) == 0 {
		return report, nil
	}

	var txResult fhir.Bundle
	if err := queryDirectoryFHIRClient.CreateWithContext(ctx, tx, &txResult, fhirclient.AtPath("/")); err != nil {
		return DirectoryUpdateReport{}, fmt.Errorf("failed to apply mCSD update to query directory: %w", err)
	}

	// Process result
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

	// Update last sync timestamp on successful completion.
	// Use the search result Bundle's meta.lastUpdated if available, otherwise fall back to query start time.
	// This uses the FHIR server's own timestamp string, eliminating clock skew issues.
	var nextSyncTime string
	if bundle.Meta != nil && bundle.Meta.LastUpdated != nil {
		nextSyncTime = *bundle.Meta.LastUpdated
	} else {
		// Fallback to local time with buffer to account for potential clock skew
		nextSyncTime = queryStartTime.Add(-clockSkewBuffer).Format(time.RFC3339)
		log.Ctx(ctx).Warn().Str("fhir_server", fhirBaseURLRaw).Msg("Bundle meta.lastUpdated not available, using local time with buffer - may cause clock skew issues")
	}
	c.lastUpdateTimes[fhirBaseURLRaw] = nextSyncTime

	return report, nil
}
