// Package lrza implements a synchronization client for a single trusted mCSD Directory (the LRZA -
// Landelijk Register Zorgaanbieders, the government-controlled registry of care providers).
//
// Unlike the mcsd component - which discovers peer directories and applies anti-spoofing validation
// to their contents - lrza syncs from one directory that is trusted wholesale. There is therefore no
// discovery phase and no per-resource validation: every resource the source returns is imported into
// the local query directory as-is, under ids derived from the source's (see localID). The first sync reads the current resources via a full search; once
// a timestamp has been recorded, later syncs read changes incrementally via _history with _since
// (which also propagates deletions). Each page of each resource type's feed is deduplicated to one
// entry per resource (a resource's versions can be split across pages, so dedup state spans the whole
// feed) and applied to the query directory as its own FHIR transaction, so memory use and FHIR
// transaction size are both bounded by the page size rather than by the size of the source directory.
//
// The package also implements the write path to the LRZA: a proxy through which tenants register their
// own addressing resources (see registration.go).
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
	"github.com/google/uuid"
	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/component"
	"github.com/nuts-foundation/nuts-knooppunt/component/tracing"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/httpauth"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/nuts-foundation/nuts-knooppunt/lib/tlsutil"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

var _ component.Lifecycle = &Component{}

// resourceTypes are the resource types synced from the trusted directory, in the NL-GF-recommended
// initial-load order (Organization -> Location -> HealthcareService -> Endpoint ->
// OrganizationAffiliation). Not configurable: the LRZA is a trusted national directory synced under a
// fixed specification, not a peer directory where an operator might reasonably want to narrow scope.
var resourceTypes = []string{"Organization", "Location", "HealthcareService", "Endpoint", "OrganizationAffiliation"}

// searchPageSize is a fixed FHIR search result page size, so behavior is deterministic across FHIR
// servers rather than relying on (widely varying) server defaults. Each page doubles as the FHIR
// transaction applied to the query directory (see syncResourceTypes), so this also bounds memory use
// and transaction size - there's no separate "batch size" to keep in sync with it. Splitting the sync
// into one transaction per page is safe because the query directory accepts resources whose references
// don't resolve yet (hapi.fhir.enforce_referential_integrity_on_write: false, per
// https://minvws.github.io/generiekefuncties-docs/en/csd.html) - a resource in one page's transaction
// may reference a resource that only lands in a later one. That relies on references staying literal
// (see appendTransactionEntry), and on the query directory creating a placeholder for a missing target
// (hapi.fhir.auto_create_placeholder_reference_targets: true): HAPI only indexes a reference whose
// target exists, so without one _include and chained search miss the link even after the target lands.
const searchPageSize = 100

// clockSkewBuffer is subtracted from local time when a Bundle's meta.lastUpdated is not available,
// to account for clock differences between this client and the FHIR server.
var clockSkewBuffer = 2 * time.Second

func DefaultConfig() Config {
	return Config{
		Registration: RegistrationConfig{
			URAAssignerKVK: coding.DefaultURAAssignerKVK,
		},
	}
}

type Config struct {
	// LRZABaseUrl is the base URL of the trusted source directory (the LRZA) to sync from.
	LRZABaseUrl string `koanf:"lrzabaseurl"`
	// QueryBaseUrl is the base URL of the local mCSD query directory that synced resources are
	// written into.
	QueryBaseUrl string `koanf:"querybaseurl"`
	// Auth optionally configures OAuth2 client-credentials authentication against the source.
	Auth httpauth.OAuth2Config `koanf:"auth"`
	// Config carries the optional mTLS client-certificate settings for the source connection
	// (tlscertfile/tlskeyfile/tlskeypassword/tlscafile). The national LRZA environment requires a
	// client certificate; the local query directory connection does not use these.
	tlsutil.Config `koanf:",squash"`
	// Registration configures the write path: registering and updating tenants' addressing
	// resources in the LRZA (see registration.go).
	Registration RegistrationConfig `koanf:"registration"`
}

// RegistrationConfig configures the LRZA registration proxy, through which tenants register their
// addressing resources in the LRZA. It is served whenever the LRZA component is configured, and uses
// the same base URL, mTLS certificate and OAuth2 settings as the sync client: the LRZA authorizes the
// certificate (the IT vendor), not the tenant.
type RegistrationConfig struct {
	// URAAssignerKVK is the KvK number of the authority that issues URA numbers (CIBG). It is written
	// to assigner.identifier.value of every URA identifier, since the CustodianAssignedIdentifier
	// profile requires each identifier to name the party that assigned it:
	// https://build.fhir.org/ig/minvws/generiekefuncties-docs/branches/1.0.0/en/StructureDefinition-nl-gf-custodianassignedidentifier.html
	// Not to be confused with Mandate.Vendor, which is this Knooppunt operator's own KvK number.
	URAAssignerKVK string `koanf:"uraassignerkvk"`
	// Mandate optionally makes the Knooppunt check, before writing, that the tenant has granted the
	// IT vendor the mandate the LRZA requires. The LRZA checks this itself either way; checking up
	// front gives a clearer error.
	Mandate MandateConfig `koanf:"mandate"`
	// Signing holds the signing certificate and key per tenant, keyed by the tenant's URA. When a
	// tenant has one, every write is followed by a signed Provenance (see provenance.go).
	Signing map[string]SigningConfig `koanf:"signing"`
}

// SigningConfig is a tenant's signing certificate and private key: a PEM certificate (leaf first,
// then any intermediates, which go into the signature's x5c) and PEM key, or a PKCS#12 file holding
// both. For production, a PKIoverheid certificate of the care provider.
type SigningConfig struct {
	// CertFile is the PEM certificate file, or a .p12/.pfx file with certificate and key.
	CertFile string `koanf:"certfile"`
	// KeyFile is the PEM private key file (not used for a .p12/.pfx file).
	KeyFile string `koanf:"keyfile"`
	// KeyPassword decrypts a .p12/.pfx file.
	KeyPassword string `koanf:"keypassword"`
}

// MandateConfig identifies the OrganizationAffiliation a tenant must have registered in the LRZA to
// authorize the IT vendor (the operator of this Knooppunt) to write on its behalf.
type MandateConfig struct {
	// Code is the affiliation code the mandate must carry, as a FHIR token (system|code). Leave empty
	// to skip the check.
	Code string `koanf:"code"`
	// Vendor is the identifier of the IT vendor's Organization, the affiliation's
	// participatingOrganization, as a FHIR token (system|value).
	Vendor string `koanf:"vendor"`
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
	fhirLRZAClient  fhirclient.Client
	fhirQueryClient fhirclient.Client

	lastUpdateTime string // _since value for the next incremental sync; empty means full sync
	updateMux      *sync.Mutex

	// registrar serves the /lrza/fhir write operations.
	registrar *registrar
	// syncRequests carries requests for a sync after a write. It has capacity 1, so a burst of
	// writes collapses into a single pending sync (see requestSync).
	syncRequests chan struct{}
	stopSync     context.CancelFunc
	syncDone     chan struct{}
}

// syncAfterWriteDelay is how long a sync after a write waits before it runs, so that a burst of writes
// (an EHR registering a Location, its Endpoint and the link between them, say) leads to one sync of
// the query directory rather than one per write.
var syncAfterWriteDelay = 5 * time.Second

// syncRun holds the state of a single sync cycle, threaded through each step (fetch -> build -> apply
// -> record) so the steps take one argument instead of a growing parameter list. The search
// configuration is set when the run is created; the remaining fields are filled as the run
// progresses.
type syncRun struct {
	// configuration, set at construction
	queryStart   time.Time
	searchParams url.Values

	// working state, filled as the run progresses
	// sourceLastUpdated is the source server's meta.lastUpdated from the first resource type's search
	// set (the server's own clock), recorded as the next _since timestamp. Nil if the server didn't
	// report one, in which case recordSyncTimestamp falls back to the local query start time.
	sourceLastUpdated *string
	tx                fhir.Bundle // transaction bundle applied to the query directory
	report            UpdateReport
}

func New(config Config) (*Component, error) {
	sourceBaseURL, err := url.Parse(config.LRZABaseUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid LRZA source FHIR base URL (url=%s): %w", config.LRZABaseUrl, err)
	}

	sourceHTTPClient, err := newSourceHTTPClient(config)
	if err != nil {
		return nil, err
	}

	queryBaseURL, err := url.Parse(config.QueryBaseUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid LRZA query directory FHIR base URL (url=%s): %w", config.QueryBaseUrl, err)
	}

	result := &Component{
		config:          config,
		fhirLRZAClient:  fhirclient.New(sourceBaseURL, sourceHTTPClient, &fhirclient.Config{UsePostSearch: false}),
		fhirQueryClient: fhirclient.New(queryBaseURL, tracing.NewHTTPClient(), &fhirclient.Config{UsePostSearch: false}),
		updateMux:       &sync.Mutex{},
		syncRequests:    make(chan struct{}, 1),
	}
	// The write path shares the sync client's HTTP client: same mTLS certificate, tracing and OAuth2,
	// and the same retry transport, whose rule is method-aware (see retryTransport).
	result.registrar, err = newRegistrar(config.Registration, config.LRZABaseUrl, result.fhirLRZAClient, result.requestSync)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// newSourceHTTPClient builds the HTTP client used to talk to the trusted source directory. It layers,
// from the bottom up: an optional mTLS transport when a client certificate is configured (required by
// the national LRZA environment), then OpenTelemetry tracing, then retrying of transient failures (see
// retryTransport), then optional OAuth2 client-credentials. The same base transport - including mTLS
// and retrying - is reused for OAuth2 token requests.
func newSourceHTTPClient(config Config) (*http.Client, error) {
	var baseTransport = http.DefaultTransport
	if config.TLSCertFile != "" {
		tlsConfig, err := tlsutil.CreateTLSConfig(config.Config)
		if err != nil {
			return nil, fmt.Errorf("LRZA mTLS is configured but failed to load: %w", err)
		}
		baseTransport = &http.Transport{TLSClientConfig: tlsConfig}
	}

	tracedTransport := tracing.WrapTransport(baseTransport)
	retryingTransport := wrapRetryTransport(tracedTransport)

	// The current implementation in the iRealisatie proeftuin does not use oAuth delegation, this is therefore untested
	if config.Auth.IsConfigured() {
		return httpauth.NewOAuth2HTTPClient(config.Auth, retryingTransport)
	}
	return &http.Client{Transport: retryingTransport}, nil
}

func (c *Component) Start() error {
	slog.Info("Starting LRZA sync component",
		logging.FHIRServer(c.config.LRZABaseUrl),
		slog.Any("resourceTypes", resourceTypes))

	ctx, cancel := context.WithCancel(context.Background())
	c.stopSync = cancel
	c.syncDone = make(chan struct{})
	go c.runRequestedSyncs(ctx)
	return nil
}

func (c *Component) Stop(_ context.Context) error {
	if c.stopSync != nil {
		c.stopSync()
		<-c.syncDone
	}
	return nil
}

// requestSync asks for a sync of the query directory after a write, without waiting for it. It does
// nothing without a query directory. If a sync is already pending the request is dropped: that
// pending sync picks up this write too.
func (c *Component) requestSync() {
	if c.config.QueryBaseUrl == "" {
		return
	}
	select {
	case c.syncRequests <- struct{}{}:
	default:
	}
}

// runRequestedSyncs runs a sync for requests on syncRequests until ctx is cancelled. Each sync waits
// syncAfterWriteDelay first, so the writes of a burst are picked up by one sync; a write that comes in
// while a sync runs schedules exactly one more. update holds updateMux, so these serialize with syncs
// triggered through /lrza/update.
func (c *Component) runRequestedSyncs(ctx context.Context) {
	defer close(c.syncDone)
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.syncRequests:
		}
		if err := sleep(ctx, syncAfterWriteDelay); err != nil {
			return
		}
		// A request that arrived during the delay is covered by this sync.
		select {
		case <-c.syncRequests:
		default:
		}
		if _, err := c.update(ctx); err != nil && ctx.Err() == nil {
			slog.ErrorContext(ctx, "LRZA sync after write failed", logging.Error(err))
		}
	}
}

// RegisterHttpHandlers registers no routes: /lrza/update is served through the generated
// OpenAPI strict server, wired up in strictAPIServer.RegisterHttpHandlers in package cmd.
func (c *Component) RegisterHttpHandlers(_, _ *http.ServeMux) {
}

func (c *Component) TriggerLrzaSync(ctx context.Context, _ api.TriggerLrzaSyncRequestObject) (api.TriggerLrzaSyncResponseObject, error) {
	report, err := c.update(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "LRZA update failed", logging.Error(err))
		return api.TriggerLrzaSync500JSONResponse{Error: new("Failed to update LRZA: " + err.Error())}, nil
	}
	return api.TriggerLrzaSync200JSONResponse(api.DirectoryUpdateReport{
		Created:  new(report.CountCreated),
		Updated:  new(report.CountUpdated),
		Deleted:  new(report.CountDeleted),
		Warnings: new(report.Warnings),
		Errors:   new(report.Errors),
	}), nil
}

// update runs one sync cycle: streams the trusted source's history page by page, applying each page as
// its own transaction to the query directory, and records the timestamp for the next incremental sync
// once everything has been applied successfully.
func (c *Component) update(ctx context.Context) (UpdateReport, error) {
	c.updateMux.Lock()
	defer c.updateMux.Unlock()

	run := c.newSyncRun()
	slog.InfoContext(ctx, "Updating from central LRZA directory",
		slog.Bool("incremental", run.incremental()))

	if err := c.syncResourceTypes(ctx, run); err != nil {
		// Return progress tallied by already-applied pages rather than discarding it, and skip
		// recordSyncTimestamp below so the next sync retries the whole window from the old
		// lastUpdateTime instead of skipping past what never got applied. Safe to retry: every write
		// is a PUT or DELETE by an id derived from the source's, so re-applying already-applied pages
		// is a no-op.
		return run.finalizedReport(), err
	}
	c.recordSyncTimestamp(ctx, run)

	return run.finalizedReport(), nil
}

// newSyncRun creates a run with the search parameters for this cycle. The first sync (no timestamp
// known) is a full search of current resources; later syncs are incremental _history queries with
// _since. Both pin newest-first ordering so deduplication can keep the first entry per resource.
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

// incremental selects the read mode, derived from the search parameters: with a _since value set it
// does an incremental _history query; without one it does a full search of current resources (the
// initial sync, when no timestamp is known yet).
func (run *syncRun) incremental() bool {
	return run.searchParams.Has("_since")
}

// syncResourceTypes streams every configured resource type's feed page by page from the trusted
// source and applies each page as its own transaction to the query directory (see queryResourceType
// and syncPage). The first sync reads current resources via a full search (so it imports only what
// currently exists, with no deletions to replay); later syncs read changes via _history.
func (c *Component) syncResourceTypes(ctx context.Context, run *syncRun) error {
	for i, resourceType := range resourceTypes {
		dedup := libfhir.NewHistoryDeduplicator()
		firstPage := true
		err := c.queryResourceType(ctx, run, resourceType, func(page *fhir.Bundle) error {
			if i == 0 && firstPage && page.Meta != nil && page.Meta.LastUpdated != nil && *page.Meta.LastUpdated != "" {
				run.sourceLastUpdated = page.Meta.LastUpdated
			}
			firstPage = false
			return c.syncPage(ctx, run, dedup, page.Entry)
		})
		if err != nil {
			return fmt.Errorf("failed to query %s: %w", resourceType, err)
		}
	}
	return nil
}

// syncPage deduplicates one page's entries against dedup (which must be reused across every page of
// the same resource type's feed - see HistoryDeduplicator), builds a transaction from the survivors,
// and applies it to the query directory.
func (c *Component) syncPage(ctx context.Context, run *syncRun, dedup *libfhir.HistoryDeduplicator, pageEntries []fhir.BundleEntry) error {
	var entries []fhir.BundleEntry
	for _, entry := range pageEntries {
		if dedup.Keep(entry) {
			entries = append(entries, entry)
		}
	}
	c.buildTransaction(ctx, run, entries)
	if len(run.tx.Entry) == 0 {
		return nil
	}
	return c.applyTransaction(ctx, run)
}

// queryResourceType queries a single resource type, calling onPage for every page as it's fetched,
// so callers can process and apply entries page by page instead of buffering
// the whole feed in memory. It reads the _history endpoint for an incremental sync, or searches the
// resource type directly for the initial full sync.
func (c *Component) queryResourceType(ctx context.Context, run *syncRun, resourceType string, onPage func(*fhir.Bundle) error) error {
	path := resourceType
	if run.incremental() {
		path = resourceType + "/_history"
	}

	var searchSet fhir.Bundle
	if err := c.fhirLRZAClient.SearchWithContext(ctx, "", run.searchParams, &searchSet, fhirclient.AtPath(path)); err != nil {
		return fmt.Errorf("search of %s failed: %w", path, err)
	}

	if err := fhirclient.Paginate(ctx, c.fhirLRZAClient, searchSet, func(page *fhir.Bundle) (bool, error) {
		if err := onPage(page); err != nil {
			return false, err
		}
		return true, nil
	}); err != nil {
		return fmt.Errorf("processing %s failed: %w", path, err)
	}
	return nil
}

// buildTransaction converts entries into a FHIR transaction bundle for the query directory, stored as
// run.tx. Entries that can't be processed are recorded as warnings rather than failing the whole sync.
func (c *Component) buildTransaction(ctx context.Context, run *syncRun, entries []fhir.BundleEntry) {
	run.tx = fhir.Bundle{
		Type:  fhir.BundleTypeTransaction,
		Entry: make([]fhir.BundleEntry, 0, len(entries)),
	}
	for i, entry := range entries {
		if err := c.appendTransactionEntry(ctx, run, entry); err != nil {
			run.report.Warnings = append(run.report.Warnings, fmt.Sprintf("entry #%d: %s", i, err.Error()))
		}
	}
}

// appendTransactionEntry translates one fetched entry into an upsert or delete against the query
// directory and appends it to run.tx. Each resource is written under an id derived from its source
// URL (see localID), and its references are rewritten to the derived ids of their targets, so an
// update or delete targets its resource without a lookup. meta.source records where it came from.
//
// References stay literal, which is what lets each page be its own transaction. A literal reference
// to a resource that only lands in a later page is accepted, because the query directory doesn't
// enforce referential integrity on write (see searchPageSize). A conditional reference would not be:
// the FHIR server must resolve it within the transaction that carries it.
//
// Entries come from two shapes: _history entries (incremental sync) carry a request whose method is
// POST/PUT (upsert) or DELETE; full-search entries (initial sync) carry only a resource body and no
// request, and are always upserts. Only a DELETE request produces a delete.
//
// This is lrza's trimmed counterpart to mcsd's buildUpdateTransaction: because the source is trusted,
// there is no anti-spoofing validation and no discoverable-directory filtering - every entry is
// imported as-is.
func (c *Component) appendTransactionEntry(ctx context.Context, run *syncRun, entry fhir.BundleEntry) error {
	// A DELETE history entry carries no resource body; its request URL names the resource. Everything
	// else (a history upsert, or a full-search result with no request) is an upsert.
	if entry.Request != nil && entry.Request.Method == fhir.HTTPVerbDELETE {
		resourceType, resourceID, ok := libfhir.TypeAndIDFromReference(entry.Request.Url)
		if !ok {
			return fmt.Errorf("invalid DELETE URL format: %s", entry.Request.Url)
		}
		_, id, err := localID(c.config.LRZABaseUrl, resourceType, resourceID)
		if err != nil {
			return fmt.Errorf("failed to derive local id for DELETE: %w", err)
		}
		slog.DebugContext(ctx, "Deleting resource", slog.String("type", resourceType), slog.String("id", resourceID))
		run.tx.Entry = append(run.tx.Entry, fhir.BundleEntry{
			Request: &fhir.BundleEntryRequest{
				Method: fhir.HTTPVerbDELETE,
				Url:    resourceType + "/" + id,
			},
		})
		return nil
	}

	if entry.Resource == nil {
		return errors.New("entry has neither a resource body nor a DELETE request")
	}
	info, err := libfhir.ExtractResourceInfo(entry.Resource)
	if err != nil {
		return err
	}
	if info.ResourceType == "" {
		return errors.New("resource has no resourceType")
	}
	if info.ID == "" {
		return errors.New("resource has no id")
	}
	// As a plain map, which convertReferencesToLocal walks.
	resourceType, resourceID, resource := info.ResourceType, info.ID, map[string]any(info.Resource)
	sourceURL, id, err := localID(c.config.LRZABaseUrl, resourceType, resourceID)
	if err != nil {
		return fmt.Errorf("failed to derive local id: %w", err)
	}

	setResourceSource(resource, sourceURL)
	resource["id"] = id
	if err := convertReferencesToLocal(resource, c.config.LRZABaseUrl); err != nil {
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
			Url:    resourceType + "/" + id,
		},
	})
	return nil
}

// localID derives the query directory's id for a source resource: a name-based (v5) UUID of its
// source URL, which is also returned for meta.source. The source's own id can't be reused: a FHIR
// server that hosts the source directory and the query directory as partitions (as HAPI does in the
// sandbox and the test harness) requires client-assigned ids to be unique across partitions. Deriving
// the id from the source URL keeps it deterministic, so writes stay idempotent and references can be
// rewritten without looking anything up.
func localID(sourceBaseURL, resourceType, resourceID string) (sourceURL, id string, err error) {
	sourceURL, err = libfhir.BuildSourceURL(sourceBaseURL, resourceType, resourceID)
	if err != nil {
		return "", "", err
	}
	return sourceURL, uuid.NewSHA1(uuid.NameSpaceURL, []byte(sourceURL)).String(), nil
}

// applyTransaction sends the transaction bundle to the query directory and tallies the per-entry
// outcomes (created/updated/deleted) into the report.
//
// Outcomes are classified by the request method we sent, not by the response status code alone. The
// status is ambiguous on its own: per the FHIR spec a DELETE returns 200 (with a payload) or 204 (no
// payload) for the same result, so mapping 200->updated / 204->deleted would miscount deletes that
// carry an OperationOutcome body as updates. A transaction response keeps one entry per request entry
// in the same order, so the request at the same index tells us which operation each outcome belongs
// to; the status then only distinguishes created (201) from updated (200) within a PUT.
func (c *Component) applyTransaction(ctx context.Context, run *syncRun) error {
	var txResult fhir.Bundle
	if err := c.fhirQueryClient.CreateWithContext(ctx, run.tx, &txResult, fhirclient.AtPath("/")); err != nil {
		return fmt.Errorf("failed to apply LRZA update to query directory: %w", err)
	}
	run.tallyTransactionResult(txResult)
	return nil
}

// tallyTransactionResult classifies each response entry against the request that produced it (same
// index, since transaction responses preserve request order) and accumulates the counts and warnings
// onto the run's report. See applyTransaction for why classification is by request method.
func (run *syncRun) tallyTransactionResult(txResult fhir.Bundle) {
	for i, entry := range txResult.Entry {
		if entry.Response == nil {
			run.report.Warnings = append(run.report.Warnings, fmt.Sprintf("Skipping entry with no response: #%d", i))
			continue
		}
		status := entry.Response.Status
		method, ok := requestMethodAt(run.tx, i)
		if !ok {
			run.report.Warnings = append(run.report.Warnings, fmt.Sprintf("Response #%d has no matching request to classify (status=%q, url=%v)", i, status, entry.FullUrl))
			continue
		}
		switch method {
		case fhir.HTTPVerbDELETE:
			if strings.HasPrefix(status, "2") {
				run.report.CountDeleted++
			} else {
				run.report.Warnings = append(run.report.Warnings, fmt.Sprintf("Unexpected status %q for DELETE (url=%v)", status, entry.FullUrl))
			}
		case fhir.HTTPVerbPUT:
			switch {
			case strings.HasPrefix(status, "201"):
				run.report.CountCreated++
			case strings.HasPrefix(status, "2"):
				// Any other 2xx (200, or 204 with no body) is a successful upsert of an existing resource.
				run.report.CountUpdated++
			default:
				run.report.Warnings = append(run.report.Warnings, fmt.Sprintf("Unexpected status %q for PUT (url=%v)", status, entry.FullUrl))
			}
		default:
			run.report.Warnings = append(run.report.Warnings, fmt.Sprintf("Unexpected request method for response #%d (status=%q, url=%v)", i, status, entry.FullUrl))
		}
	}
}

// requestMethodAt returns the HTTP method of the request entry at index i in the transaction bundle.
// Transaction responses preserve request order, so this correlates a response outcome with the
// operation that produced it. ok is false when there is no request entry at that index.
func requestMethodAt(tx fhir.Bundle, i int) (method fhir.HTTPVerb, ok bool) {
	if i < len(tx.Entry) && tx.Entry[i].Request != nil {
		return tx.Entry[i].Request.Method, true
	}
	return method, false
}

// recordSyncTimestamp stores the timestamp used as the _since value for the next incremental sync.
// It prefers the search result Bundle's meta.lastUpdated (the FHIR server's own clock, avoiding
// skew) and falls back to the local query start time minus a buffer.
func (c *Component) recordSyncTimestamp(ctx context.Context, run *syncRun) {
	if run.sourceLastUpdated != nil {
		c.lastUpdateTime = *run.sourceLastUpdated
		return
	}
	c.lastUpdateTime = run.queryStart.Add(-clockSkewBuffer).Format(time.RFC3339Nano)
	slog.WarnContext(ctx, "Bundle meta.lastUpdated not available, using local time with buffer - may cause clock skew issues", logging.FHIRServer(c.config.LRZABaseUrl))
}

// finalizedReport returns the run's report with nil slices replaced by empty ones, for a nicer JSON
// REST response.
func (run *syncRun) finalizedReport() UpdateReport {
	report := run.report
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

// convertReferencesToLocal rewrites every relative "ResourceType/id" reference in the resource to
// the target's id in the query directory (see localID). The target doesn't have to exist yet.
func convertReferencesToLocal(obj any, sourceBaseURL string) error {
	switch v := obj.(type) {
	case map[string]any:
		if ref, ok := v["reference"].(string); ok {
			parts := strings.Split(ref, "/")
			if len(parts) == 2 {
				_, id, err := localID(sourceBaseURL, parts[0], parts[1])
				if err != nil {
					return fmt.Errorf("failed to derive local id for reference: %w", err)
				}
				v["reference"] = parts[0] + "/" + id
			}
		}
		for _, value := range v {
			if err := convertReferencesToLocal(value, sourceBaseURL); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := convertReferencesToLocal(item, sourceBaseURL); err != nil {
				return err
			}
		}
	}
	return nil
}
