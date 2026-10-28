package lrza

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/google/uuid"
	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/lib/coding"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	libfhir "github.com/nuts-foundation/nuts-knooppunt/lib/fhirutil"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// The registration proxy: the write path from a tenant's EHR, through the Knooppunt, to the LRZA
// (the ITI-130-NL "Data Source" role of https://build.fhir.org/ig/minvws/generiekefuncties-docs/branches/1.0.0/en/csd.html).
//
// The Knooppunt is the authorized party (the IT vendor): one mTLS certificate serves every tenant, and
// the LRZA authorizes that certificate, not the tenant. So every write goes through the same pipeline,
// which makes sure a tenant only writes its own resources and does the IG's work for the client:
//
//  1. ownership: the current version (for an update or withdrawal) and the new version must belong to
//     the tenant (ownership.go);
//  2. enrichment: profile, identifiers and Endpoint details the IG requires (enrich.go);
//  3. local validation of cardinalities and invariants (validate.go), the LRZA being the final judge;
//  4. optimistic locking: the client's If-Match, or the version read in step 1;
//  5. the write itself, a delete being rewritten to a withdrawal by status (withdraw.go);
//  6. an optional signed Provenance (provenance.go), and a sync of the change into the query directory.

// writableResourceTypes are the resource types a tenant registers through the proxy: the ones the IG
// lets a Data Source create and update. This has to be an allowlist, even though the LRZA rejects
// some other types itself (an OrganizationAffiliation written by the IT vendor, say): every write goes
// out under the vendor's certificate, and tenant ownership can only be checked for these four. The
// LRZA accepts a Provenance from a Data Source, for instance, so forwarding one unchecked would let a
// tenant sign off on another tenant's resource.
var writableResourceTypes = map[string]bool{
	"Organization":      true,
	"Location":          true,
	"HealthcareService": true,
	"Endpoint":          true,
}

// registrar implements the /lrza/fhir operations.
type registrar struct {
	config RegistrationConfig
	// client talks to the LRZA; it is the sync client's FHIR client (see New).
	client fhirclient.Client
	// baseURL is the LRZA's FHIR base URL, to recognize absolute references to it.
	baseURL     string
	requestSync func()
	mandate     *mandate
	// signers holds the Provenance signer per tenant URA.
	signers map[string]*signer
	now     func() time.Time
	newUUID func() string
}

func newRegistrar(config RegistrationConfig, baseURL string, client fhirclient.Client, requestSync func()) (*registrar, error) {
	mandate, err := parseMandate(config.Mandate)
	if err != nil {
		return nil, err
	}
	signers := make(map[string]*signer, len(config.Signing))
	for ura, signingConfig := range config.Signing {
		s, err := loadSigner(signingConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to load LRZA Provenance signing key for tenant %s: %w", ura, err)
		}
		signers[ura] = s
	}
	return &registrar{
		config:      config,
		client:      client,
		baseURL:     strings.TrimSuffix(baseURL, "/"),
		requestSync: requestSync,
		mandate:     mandate,
		signers:     signers,
		now:         time.Now,
		newUUID:     uuid.NewString,
	}, nil
}

// writeResult is the outcome of a write of a single resource.
type writeResult struct {
	body     json.RawMessage
	etag     string
	location string
}

// writeContext holds the state of one request: who it is for, and a client that remembers what it has
// read from the LRZA, so a reference followed more than once (e.g. the tenant's Organization) is read
// once.
type writeContext struct {
	*registrar
	tenantURA string
	// client is a caching client over the registrar's, for this request only.
	client fhirclient.Client
	owners *ownershipChecker
	// tenantOrganizationFound and tenantOrganizationResult memoize tenantOrganization.
	tenantOrganizationFound  bool
	tenantOrganizationResult libfhir.Resource
}

func (r *registrar) newWriteContext(tenantURA string) *writeContext {
	client := libfhir.NewCachingClient(r.client)
	return &writeContext{
		registrar: r,
		tenantURA: tenantURA,
		client:    client,
		owners: &ownershipChecker{
			client:    client,
			baseURL:   r.baseURL,
			tenantURA: tenantURA,
			bundle:    map[string]libfhir.Resource{},
		},
	}
}

// checkResourceType rejects resource types the proxy doesn't write.
func checkResourceType(resourceType string) error {
	if !writableResourceTypes[resourceType] {
		return fhirapi.BadRequestError(fmt.Sprintf("unsupported resource type: %s (expected Organization, Location, HealthcareService or Endpoint)", resourceType), nil)
	}
	return nil
}

// parseRequestResource parses a request body as a resource of the type (and, for an update, with the
// id) of the request path.
func parseRequestResource(body []byte, resourceType, id string) (libfhir.Resource, error) {
	res, err := libfhir.ParseResourceOfType(body, resourceType, id)
	if err != nil {
		return nil, fhirapi.BadRequestError("invalid request body: "+err.Error(), err)
	}
	return res, nil
}

func (r *registrar) createResource(ctx context.Context, tenantURA, resourceType string, body []byte) (*writeResult, error) {
	if err := checkResourceType(resourceType); err != nil {
		return nil, err
	}
	res, err := parseRequestResource(body, resourceType, "")
	if err != nil {
		return nil, err
	}
	w := r.newWriteContext(tenantURA)
	if err := w.prepare(ctx, res, nil); err != nil {
		return nil, err
	}
	stored, headers, err := w.create(ctx, res)
	if err != nil {
		return nil, err
	}
	return w.finish(ctx, stored, headers), nil
}

func (r *registrar) readResource(ctx context.Context, tenantURA, resourceType, id string) (*writeResult, error) {
	if err := checkResourceType(resourceType); err != nil {
		return nil, err
	}
	w := r.newWriteContext(tenantURA)
	current, etag, err := w.readCurrent(ctx, resourceType, id)
	if err != nil {
		return nil, err
	}
	if err := w.owners.requireOwned(ctx, current, "the resource does not belong to the tenant"); err != nil {
		return nil, err
	}
	body, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	return &writeResult{body: body, etag: etag}, nil
}

func (r *registrar) updateResource(ctx context.Context, tenantURA, resourceType, id string, ifMatch *string, body []byte) (*writeResult, error) {
	if err := checkResourceType(resourceType); err != nil {
		return nil, err
	}
	res, err := parseRequestResource(body, resourceType, id)
	if err != nil {
		return nil, err
	}
	w := r.newWriteContext(tenantURA)
	current, etag, err := w.readCurrent(ctx, resourceType, id)
	if err != nil {
		return nil, err
	}
	if err := w.owners.requireOwned(ctx, current, "the current version in the LRZA does not belong to the tenant"); err != nil {
		return nil, err
	}
	if err := w.prepare(ctx, res, current); err != nil {
		return nil, err
	}
	stored, headers, err := w.update(ctx, res, ifMatchFor(ifMatch, etag, current))
	if err != nil {
		return nil, err
	}
	return w.finish(ctx, stored, headers), nil
}

func (r *registrar) withdrawResource(ctx context.Context, tenantURA, resourceType, id string, ifMatch *string) (*writeResult, error) {
	if err := checkResourceType(resourceType); err != nil {
		return nil, err
	}
	w := r.newWriteContext(tenantURA)
	current, etag, err := w.readCurrent(ctx, resourceType, id)
	if err != nil {
		return nil, err
	}
	if err := w.owners.requireOwned(ctx, current, "the current version in the LRZA does not belong to the tenant"); err != nil {
		return nil, err
	}
	if err := w.checkMandate(ctx); err != nil {
		return nil, err
	}
	withdrawn := current.Clone()
	if err := withdraw(withdrawn, r.now()); err != nil {
		return nil, err
	}
	stored, headers, err := w.update(ctx, withdrawn, ifMatchFor(ifMatch, etag, current))
	if err != nil {
		return nil, err
	}
	return w.finish(ctx, stored, headers), nil
}

// prepare runs the pipeline steps before a write of a new version of a resource: enrichment,
// ownership of the new version, local validation and the mandate check. current is the version in
// the LRZA, or nil for a create.
func (w *writeContext) prepare(ctx context.Context, res, current libfhir.Resource) error {
	if err := enrich(ctx, res, current, w.enrichOptions()); err != nil {
		return err
	}
	if err := w.owners.checkNewVersion(ctx, res); err != nil {
		return err
	}
	if issues := validateResource(res); len(issues) > 0 {
		return validationError(issues)
	}
	return w.checkMandate(ctx)
}

// finish runs the steps after a successful write: signing a Provenance and requesting a sync.
func (w *writeContext) finish(ctx context.Context, stored []byte, headers fhirclient.Headers) *writeResult {
	result := &writeResult{
		body: stored,
		etag: etagOf(headers, stored),
	}
	if location := relativeLocation(stored, headers.Get("Location")); location != "" {
		result.location = "/lrza/fhir/" + location
	}
	// Signing is optional in the IG, so a failure doesn't undo the write. It is only logged: it's the
	// operator's signing configuration that needs fixing, nothing the client could act on.
	if err := w.signProvenance(ctx, stored); err != nil {
		slog.WarnContext(ctx, "LRZA write succeeded, but signing its Provenance failed", logging.Error(err))
	}
	w.requestSync()
	return result
}

// enrichOptions is what enrichment needs to know about this request.
func (w *writeContext) enrichOptions() enrichOptions {
	return enrichOptions{
		tenantURA:          w.tenantURA,
		uraAssignerKVK:     w.config.URAAssignerKVK,
		tenantOrganization: w.tenantOrganizationReference,
		newUUID:            w.newUUID,
	}
}

// tenantOrganizationReference returns the reference to the tenant's URA Organization: an entry of the
// transaction being processed, or else the Organization in the LRZA. It is a 422 if there is neither:
// a care provider is registered in the LRZA before its Endpoints are.
func (w *writeContext) tenantOrganizationReference(ctx context.Context) (string, error) {
	keys := make([]string, 0, len(w.owners.bundle))
	for key := range w.owners.bundle {
		keys = append(keys, key)
	}
	slices.Sort(keys) // a deterministic choice, should there be more than one
	for _, key := range keys {
		res := w.owners.bundle[key]
		if res.ResourceType() == "Organization" && slices.Contains(uraValues(res), w.tenantURA) {
			return key, nil
		}
	}
	organization, err := w.tenantOrganization(ctx)
	if err != nil {
		return "", err
	}
	if organization == nil {
		return "", unprocessableError(fmt.Sprintf("the tenant's Organization (URA %s) is not registered in the LRZA; register it first", w.tenantURA))
	}
	return organization.Reference(), nil
}

// tenantOrganization finds the tenant's Organization in the LRZA by its URA; nil if it isn't
// registered. The search is done once per request.
func (w *writeContext) tenantOrganization(ctx context.Context) (libfhir.Resource, error) {
	if w.tenantOrganizationFound {
		return w.tenantOrganizationResult, nil
	}
	var searchSet fhir.Bundle
	query := url.Values{"identifier": []string{coding.URAOIDNamingSystem + "|" + w.tenantURA}}
	if err := w.client.SearchWithContext(ctx, "Organization", query, &searchSet); err != nil {
		return nil, &fhirapi.Error{Message: "Failed to search the tenant's Organization in the LRZA", Cause: err, IssueType: fhir.IssueTypeTransient}
	}
	w.tenantOrganizationFound = true
	for _, entry := range searchSet.Entry {
		if entry.Search != nil && entry.Search.Mode != nil && *entry.Search.Mode != fhir.SearchEntryModeMatch {
			continue
		}
		res, err := libfhir.ParseResource(entry.Resource)
		if err != nil || res.ResourceType() != "Organization" {
			continue
		}
		w.tenantOrganizationResult = res
		break
	}
	return w.tenantOrganizationResult, nil
}

// checkMandate checks the tenant granted the configured mandate; it does nothing without one.
func (w *writeContext) checkMandate(ctx context.Context) error {
	if w.mandate == nil {
		return nil
	}
	organization, err := w.tenantOrganization(ctx)
	if err != nil {
		return err
	}
	return w.mandate.check(ctx, w.client, w.owners.resolve, w.tenantURA, organization)
}

// signProvenance signs a stored resource for a tenant with a signing key; it does nothing for others.
func (w *writeContext) signProvenance(ctx context.Context, stored []byte) error {
	s := w.signers[w.tenantURA]
	if s == nil {
		return nil
	}
	signingTime := w.now()
	organization, err := w.tenantOrganization(ctx)
	if err != nil {
		return err
	}
	return signProvenance(ctx, w.client, s, w.tenantURA, organization, signingTime, stored)
}

// ifMatchFor is the If-Match to send with an update: the client's own, unchanged, or else the version
// the ownership check read. The latter also closes the gap between that check and the write: if
// someone else changed the resource in between, the LRZA rejects the write with 412.
func ifMatchFor(clientIfMatch *string, etag string, current libfhir.Resource) string {
	if clientIfMatch != nil && *clientIfMatch != "" {
		return *clientIfMatch
	}
	if etag != "" {
		return etag
	}
	if versionID := current.VersionID(); versionID != "" {
		return `W/"` + versionID + `"`
	}
	return ""
}

// etagOf is the ETag of a written resource: the LRZA's header, or else derived from meta.versionId.
func etagOf(headers fhirclient.Headers, stored []byte) string {
	if etag := headers.Get("ETag"); etag != "" {
		return etag
	}
	if res, err := libfhir.ParseResource(stored); err == nil {
		if versionID := res.VersionID(); versionID != "" {
			return `W/"` + versionID + `"`
		}
	}
	return ""
}

// relativeLocation is the location of a written resource version relative to the FHIR base, e.g.
// "Location/123/_history/1". The LRZA's own location isn't passed on to the client as is, since it
// reveals the LRZA's base URL, and the client should talk to the Knooppunt anyway. It is derived from
// the stored resource, or else from the trailing segments of the LRZA's location; "" if neither has it.
func relativeLocation(stored []byte, lrzaLocation string) string {
	if res, err := libfhir.ParseResource(stored); err == nil && res.ResourceType() != "" && res.ID() != "" {
		if versionID := res.VersionID(); versionID != "" {
			return res.Reference() + "/_history/" + versionID
		}
		return res.Reference()
	}
	if parsed, err := url.Parse(lrzaLocation); err == nil {
		segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		n := len(segments)
		switch {
		case n >= 4 && segments[n-2] == "_history":
			return strings.Join(segments[n-4:], "/")
		case n >= 2 && segments[n-2] != "" && segments[n-1] != "":
			return strings.Join(segments[n-2:], "/")
		}
	}
	return ""
}

// readCurrent reads the current version of a resource from the LRZA, with its ETag.
func (w *writeContext) readCurrent(ctx context.Context, resourceType, id string) (libfhir.Resource, string, error) {
	var data []byte
	var headers fhirclient.Headers
	var status int
	err := w.client.ReadWithContext(ctx, resourceType+"/"+id, &data,
		fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status))
	if err != nil {
		return nil, "", upstreamError("read "+resourceType+"/"+id+" from", err, status, headers.Header)
	}
	res, err := libfhir.ParseResource(data)
	if err != nil {
		return nil, "", &fhirapi.Error{Message: "LRZA returned an invalid resource", Cause: err, IssueType: fhir.IssueTypeException}
	}
	return res, headers.Get("ETag"), nil
}

// create and update write a resource with fhirclient.ReturnRepresentation, so they return the resource
// as the LRZA stored it - with its id and versionId - which is returned to the client and signed.

func (w *writeContext) create(ctx context.Context, res libfhir.Resource) ([]byte, fhirclient.Headers, error) {
	data, err := json.Marshal(res)
	if err != nil {
		return nil, fhirclient.Headers{}, err
	}
	var stored []byte
	var headers fhirclient.Headers
	var status int
	err = w.client.CreateWithContext(ctx, data, &stored, fhirclient.ReturnRepresentation(),
		fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status))
	if err != nil {
		return nil, headers, upstreamError("create "+res.ResourceType()+" at", err, status, headers.Header)
	}
	return stored, headers, nil
}

func (w *writeContext) update(ctx context.Context, res libfhir.Resource, ifMatch string) ([]byte, fhirclient.Headers, error) {
	requestHeaders := http.Header{}
	if ifMatch != "" {
		requestHeaders.Set("If-Match", ifMatch)
	}
	var stored []byte
	var headers fhirclient.Headers
	var status int
	err := w.client.UpdateWithContext(ctx, res.Reference(), res, &stored, fhirclient.ReturnRepresentation(),
		fhirclient.RequestHeaders(requestHeaders),
		fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status))
	if err != nil {
		return nil, headers, upstreamError("update "+res.Reference()+" at", err, versionConflictStatus(status, ifMatch != ""), headers.Header)
	}
	return stored, headers, nil
}

// versionConflictStatus maps a 409 Conflict on a write that carried If-Match to 412 Precondition
// Failed. The IG prescribes 412 for a stale version, but some FHIR servers (HAPI among them) answer
// 409 instead; the client gets the same answer either way.
func versionConflictStatus(status int, withIfMatch bool) int {
	if status == http.StatusConflict && withIfMatch {
		return http.StatusPreconditionFailed
	}
	return status
}

// registrationError is an error with its own HTTP status and OperationOutcome, for the cases
// fhirapi.Error's issue-type mapping doesn't cover: 403 (not the tenant's resource), 422 (failed
// validation), and upstream LRZA errors passed through to the client (412, 429, ...).
type registrationError struct {
	status  int
	outcome fhir.OperationOutcome
	header  http.Header
	cause   error
}

func (e *registrationError) Error() string {
	var messages []string
	for _, issue := range e.outcome.Issue {
		if issue.Diagnostics != nil {
			messages = append(messages, *issue.Diagnostics)
		}
	}
	message := fmt.Sprintf("status %d: %s", e.status, strings.Join(messages, "; "))
	if e.cause != nil {
		message += ": " + e.cause.Error()
	}
	return message
}

func (e *registrationError) Unwrap() error {
	return e.cause
}

func errorIssue(code fhir.IssueType, diagnostics string, expression ...string) fhir.OperationOutcomeIssue {
	return fhir.OperationOutcomeIssue{
		Severity:    fhir.IssueSeverityError,
		Code:        code,
		Diagnostics: &diagnostics,
		Expression:  expression,
	}
}

func forbiddenError(message string) error {
	return &registrationError{
		status:  http.StatusForbidden,
		outcome: fhir.OperationOutcome{Issue: []fhir.OperationOutcomeIssue{errorIssue(fhir.IssueTypeForbidden, message)}},
	}
}

// unprocessableError is a 422: the request is well-formed, but can't be processed as it stands.
func unprocessableError(message string) error {
	return &registrationError{
		status:  http.StatusUnprocessableEntity,
		outcome: fhir.OperationOutcome{Issue: []fhir.OperationOutcomeIssue{errorIssue(fhir.IssueTypeProcessing, message)}},
	}
}

// upstreamError turns a failed LRZA request into the error returned to the client. An LRZA error
// response is passed through with its status code and OperationOutcome, so the client sees why the
// LRZA rejected the request; a Retry-After is passed on too. A 401 is our credentials' problem, not
// the client's, so it becomes a 502. Without a response at all, the LRZA is unavailable (503).
func upstreamError(action string, err error, status int, header http.Header) error {
	if status < 300 || status == 0 {
		return &fhirapi.Error{Message: "Failed to " + action + " the LRZA", Cause: err, IssueType: fhir.IssueTypeTransient}
	}
	result := &registrationError{status: status, cause: err}
	var outcome fhirclient.OperationOutcomeError
	if errors.As(err, &outcome) && len(outcome.Issue) > 0 {
		result.outcome = outcome.OperationOutcome
	} else {
		result.outcome = fhir.OperationOutcome{Issue: []fhir.OperationOutcomeIssue{
			errorIssue(fhir.IssueTypeException, fmt.Sprintf("Failed to %s the LRZA (status=%d)", action, status)),
		}}
	}
	if status == http.StatusUnauthorized {
		result.status = http.StatusBadGateway
	}
	if retryAfter := header.Get("Retry-After"); retryAfter != "" {
		result.header = http.Header{"Retry-After": []string{retryAfter}}
	}
	return result
}

// errorResponse builds the OperationOutcome response for an error of the registration pipeline.
func errorResponse(ctx context.Context, err error) api.OperationOutcomeResponse {
	var regErr *registrationError
	if errors.As(err, &regErr) {
		slog.WarnContext(ctx, "LRZA registration request rejected", logging.Error(err))
		return api.OperationOutcomeResponse{StatusCode: regErr.status, Outcome: regErr.outcome, Header: regErr.header}
	}
	return api.NewOperationOutcomeResponse(ctx, err)
}

func optionalHeader(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (c *Component) SubmitLrzaTransaction(ctx context.Context, request api.SubmitLrzaTransactionRequestObject) (api.SubmitLrzaTransactionResponseObject, error) {
	result, err := c.registrar.transaction(ctx, request.Params.XTenantID.URA(), *request.Body)
	if err != nil {
		return errorResponse(ctx, err), nil
	}
	return transactionResponse(result), nil
}

// transactionResponse writes the transaction-response Bundle. The generated
// SubmitLrzaTransaction200ApplicationFhirPlusJSONResponse can't be used: it is a named []byte rather
// than a json.RawMessage, so it would be encoded as a base64 string.
type transactionResponse json.RawMessage

func (r transactionResponse) VisitSubmitLrzaTransactionResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/fhir+json")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(r)
	return err
}

func (c *Component) CreateLrzaResource(ctx context.Context, request api.CreateLrzaResourceRequestObject) (api.CreateLrzaResourceResponseObject, error) {
	result, err := c.registrar.createResource(ctx, request.Params.XTenantID.URA(), string(request.ResourceType), *request.Body)
	if err != nil {
		return errorResponse(ctx, err), nil
	}
	return api.CreateLrzaResource201ApplicationFhirPlusJSONResponse{
		Body: api.FHIRResource(result.body),
		Headers: api.CreateLrzaResource201ResponseHeaders{
			ETag:     optionalHeader(result.etag),
			Location: optionalHeader(result.location),
		},
	}, nil
}

func (c *Component) ReadLrzaResource(ctx context.Context, request api.ReadLrzaResourceRequestObject) (api.ReadLrzaResourceResponseObject, error) {
	result, err := c.registrar.readResource(ctx, request.Params.XTenantID.URA(), string(request.ResourceType), request.Id)
	if err != nil {
		return errorResponse(ctx, err), nil
	}
	return api.ReadLrzaResource200ApplicationFhirPlusJSONResponse{
		Body:    api.FHIRResource(result.body),
		Headers: api.ReadLrzaResource200ResponseHeaders{ETag: optionalHeader(result.etag)},
	}, nil
}

func (c *Component) UpdateLrzaResource(ctx context.Context, request api.UpdateLrzaResourceRequestObject) (api.UpdateLrzaResourceResponseObject, error) {
	result, err := c.registrar.updateResource(ctx, request.Params.XTenantID.URA(), string(request.ResourceType), request.Id, request.Params.IfMatch, *request.Body)
	if err != nil {
		return errorResponse(ctx, err), nil
	}
	return api.UpdateLrzaResource200ApplicationFhirPlusJSONResponse{
		Body: api.FHIRResource(result.body),
		Headers: api.UpdateLrzaResource200ResponseHeaders{
			ETag: optionalHeader(result.etag),
		},
	}, nil
}

func (c *Component) WithdrawLrzaResource(ctx context.Context, request api.WithdrawLrzaResourceRequestObject) (api.WithdrawLrzaResourceResponseObject, error) {
	result, err := c.registrar.withdrawResource(ctx, request.Params.XTenantID.URA(), string(request.ResourceType), request.Id, request.Params.IfMatch)
	if err != nil {
		return errorResponse(ctx, err), nil
	}
	return api.WithdrawLrzaResource200ApplicationFhirPlusJSONResponse{
		Body: api.FHIRResource(result.body),
		Headers: api.WithdrawLrzaResource200ResponseHeaders{
			ETag: optionalHeader(result.etag),
		},
	}, nil
}
