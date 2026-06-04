package mcsd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// discover runs the discovery phase for every discoverable (root) administration directory: it
// crawls each one to register newly-found mCSD directory endpoints and unregister deleted ones.
// It only mutates c.administrationDirectories. Warnings/errors are returned keyed by directory so
// update can merge them into the sync report for the same directory.
func (c *Component) discover(ctx context.Context) UpdateReport {
	result := make(UpdateReport)

	// Snapshot the discoverable directories first: discovery appends newly-found directories to the
	// registry, and those must not be crawled again in the same cycle.
	var roots []administrationDirectory
	for _, dir := range c.administrationDirectories {
		if dir.discover {
			roots = append(roots, dir)
		}
	}

	for _, root := range roots {
		result[makeDirectoryKey(root.fhirBaseURL, root.authoritativeUra)] = c.discoverFromDirectory(ctx, root)
	}
	return result
}

// discoverFromDirectory crawls a single root directory: it queries Organization and Endpoint
// history, unregisters administration directories whose Endpoint was deleted, and registers
// newly-found mCSD directory endpoints. It does a full scan (no _since) — roots are small, and a
// full scan keeps register/unregister deterministic without tracking a separate sync timestamp.
//
// Failures that prevent crawling (bad URL, query error) are logged but not reported here: the sync
// phase queries the same directory and will surface those errors, so reporting them here too would
// duplicate them.
func (c *Component) discoverFromDirectory(ctx context.Context, root administrationDirectory) DirectoryUpdateReport {
	slog.InfoContext(ctx, "Discovering mCSD Directories from root", logging.FHIRServer(root.fhirBaseURL))
	var report DirectoryUpdateReport

	baseURL, err := url.Parse(root.fhirBaseURL)
	if err != nil {
		slog.WarnContext(ctx, "Skipping discovery: invalid FHIR base URL", logging.FHIRServer(root.fhirBaseURL), logging.Error(err))
		return report
	}
	client := c.fhirAdminClientFn(baseURL)

	searchParams := url.Values{
		"_count": []string{strconv.Itoa(searchPageSize)},
	}
	entries, _, err := c.queryAllResourceTypes(ctx, client, root.resourceTypes, searchParams)
	if err != nil {
		slog.WarnContext(ctx, "Skipping discovery: failed to query root directory", logging.FHIRServer(root.fhirBaseURL), logging.Error(err))
		return report
	}

	// Unregister administration directories whose backing Endpoint was deleted upstream.
	c.processEndpointDeletes(ctx, deduplicateHistoryEntries(entries))

	parentOrganizationsMap, err := c.ensureParentOrganizationsMap(ctx, root.fhirBaseURL, client, root.authoritativeUra)
	if err != nil {
		slog.WarnContext(ctx, "Skipping endpoint discovery: failed to build parent organization map", logging.FHIRServer(root.fhirBaseURL), logging.Error(err))
		return report
	}

	return c.discoverAndRegisterEndpoints(ctx, entries, parentOrganizationsMap, report)
}

// discoverAndRegisterEndpoints registers every mCSD directory Endpoint referenced by a parent
// organization (one with a URA identifier) as a new administration directory. Registration failures
// are recorded as warnings on the report.
func (c *Component) discoverAndRegisterEndpoints(ctx context.Context, entries []fhir.BundleEntry, parentOrganizationsMap parentOrganizationMap, report DirectoryUpdateReport) DirectoryUpdateReport {
	for parentOrg := range parentOrganizationsMap {
		authoritativeUra, ok := parentOrgAuthoritativeURA(parentOrg)
		if !ok {
			continue
		}

		for fullUrl, endpoint := range endpointsReferencedByParent(entries, parentOrg) {
			if !coding.CodablesIncludesCode(endpoint.PayloadType, coding.PayloadCoding) {
				continue
			}
			slog.DebugContext(ctx, "Discovered mCSD Directory", slog.String("address", endpoint.Address))
			if err := c.registerAdministrationDirectory(ctx, endpoint.Address, c.directoryResourceTypes, false, fullUrl, authoritativeUra, false); err != nil {
				report.Warnings = append(report.Warnings, fmt.Sprintf("failed to register discovered mCSD Directory at %s: %s", endpoint.Address, err.Error()))
			}
		}
	}
	return report
}

// parentOrgAuthoritativeURA returns the organization's URA identifier value, if it has one.
func parentOrgAuthoritativeURA(org *fhir.Organization) (string, bool) {
	uraIdentifiers := libfhir.FilterIdentifiersBySystem(org.Identifier, coding.URANamingSystem)
	if len(uraIdentifiers) == 0 || uraIdentifiers[0].Value == nil {
		return "", false
	}
	return *uraIdentifiers[0].Value, true
}

// endpointsReferencedByParent returns the Endpoint resources among the entries that the parent
// organization references via Organization.endpoint, keyed by their Bundle entry fullUrl.
func endpointsReferencedByParent(entries []fhir.BundleEntry, parentOrg *fhir.Organization) map[string]*fhir.Endpoint {
	endpoints := make(map[string]*fhir.Endpoint)
	if len(parentOrg.Endpoint) == 0 {
		return endpoints
	}

	for _, entry := range entries {
		if entry.Resource == nil || entry.FullUrl == nil {
			continue
		}
		var endpoint fhir.Endpoint
		if err := json.Unmarshal(entry.Resource, &endpoint); err != nil || endpoint.Id == nil {
			continue
		}
		for _, parentEndpoint := range parentOrg.Endpoint {
			if parentEndpoint.Reference != nil && *endpoint.Id == extractReferenceID(parentEndpoint.Reference) {
				endpoints[*entry.FullUrl] = &endpoint
				break
			}
		}
	}
	return endpoints
}

func (c *Component) registerAdministrationDirectory(ctx context.Context, fhirBaseURL string, resourceTypes []string, discover bool, sourceURL string, authoritativeUra string, trusted bool) error {
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
			slog.InfoContext(ctx, "Skipping administration directory registration: excluded by configuration", logging.FHIRServer(fhirBaseURL))
			return nil
		}
	}

	exists := slices.ContainsFunc(c.administrationDirectories, func(directory administrationDirectory) bool {
		return directory.fhirBaseURL == fhirBaseURL && directory.authoritativeUra == authoritativeUra
	})
	if exists {
		return nil
	}
	c.administrationDirectories = append(c.administrationDirectories, administrationDirectory{
		resourceTypes:    resourceTypes,
		fhirBaseURL:      fhirBaseURL,
		discover:         discover,
		trusted:          trusted,
		sourceURL:        sourceURL,
		authoritativeUra: authoritativeUra,
	})
	slog.InfoContext(ctx, "Registered mCSD Directory", logging.FHIRServer(fhirBaseURL), slog.Bool("discover", discover), slog.Bool("trusted", trusted))
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
		slog.InfoContext(ctx, "Unregistered mCSD Directory after Endpoint deletion", slog.String("full_url", fullUrl))
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
