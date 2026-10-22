package lrza

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/google/uuid"
	"github.com/nuts-foundation/nuts-knooppunt/api"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
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

// writableResourceTypes are the resource types a tenant registers through the proxy.
var writableResourceTypes = map[string]bool{
	"Organization":      true,
	"Location":          true,
	"HealthcareService": true,
	"Endpoint":          true,
}

// forbiddenResourceTypes are rejected with 403 rather than 400, since they are valid LRZA resource
// types the proxy deliberately does not write, each with its reason.
var forbiddenResourceTypes = map[string]string{
	"OrganizationAffiliation": "the mandate is granted by the care provider itself; the IT vendor may not write it",
	"Provenance":              "Provenance resources are created by the Knooppunt itself",
	"Practitioner":            "Practitioner is out of scope of the Care Services Directory",
	"PractitionerRole":        "PractitionerRole is out of scope of the Care Services Directory",
}

// resource is a FHIR resource as generic JSON, so that every element the client sent - including
// ones this code doesn't know about, such as extensions - is forwarded unchanged.
type resource map[string]any

func parseResource(data []byte) (resource, error) {
	var result resource
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fhirapi.BadRequestError("request body is not a valid FHIR resource: "+err.Error(), err)
	}
	if result == nil {
		return nil, fhirapi.BadRequestError("request body is not a valid FHIR resource", nil)
	}
	return result, nil
}

func (r resource) resourceType() string {
	value, _ := r["resourceType"].(string)
	return value
}

func (r resource) id() string {
	value, _ := r["id"].(string)
	return value
}

// reference is the relative reference to this resource, e.g. "Organization/123".
func (r resource) reference() string {
	return r.resourceType() + "/" + r.id()
}

func (r resource) versionID() string {
	meta, _ := r["meta"].(map[string]any)
	value, _ := meta["versionId"].(string)
	return value
}

// clone returns a deep copy, so a resource read once can be both checked and modified.
func (r resource) clone() resource {
	data, _ := json.Marshal(r)
	var result resource
	_ = json.Unmarshal(data, &result)
	return result
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
	// warning is set when the write succeeded, but signing its Provenance failed.
	warning string
}

// writeContext holds the state of one request: who it is for, and what it has read from the LRZA
// so far, so that a reference followed more than once (e.g. the tenant's Organization) is read once.
type writeContext struct {
	*registrar
	tenantURA string
	// cache holds LRZA reads by relative reference ("Organization/123"). A nil value records that
	// the resource doesn't exist.
	cache map[string]resource
	// bundle holds the resources of a transaction by fullUrl and, for updates, by relative
	// reference, so references between entries resolve to the version being written.
	bundle map[string]resource
	// tenantOrganization caches the tenant's URA Organization; see tenantOrganization.
	tenantOrganization resource
}

func (r *registrar) newWriteContext(tenantURA string) *writeContext {
	return &writeContext{
		registrar: r,
		tenantURA: tenantURA,
		cache:     map[string]resource{},
		bundle:    map[string]resource{},
	}
}

// checkResourceType rejects resource types the proxy doesn't write.
func checkResourceType(resourceType string) error {
	if reason, forbidden := forbiddenResourceTypes[resourceType]; forbidden {
		return forbiddenError(fmt.Sprintf("resource type %s can't be written through the Knooppunt: %s", resourceType, reason))
	}
	if !writableResourceTypes[resourceType] {
		return fhirapi.BadRequestError(fmt.Sprintf("unsupported resource type: %s (expected Organization, Location, HealthcareService or Endpoint)", resourceType), nil)
	}
	return nil
}

// parseRequestResource parses a request body and checks it is of the resource type of the request
// path, and (for an update) has the id of the request path.
func parseRequestResource(body []byte, resourceType, id string) (resource, error) {
	res, err := parseResource(body)
	if err != nil {
		return nil, err
	}
	if res.resourceType() != resourceType {
		return nil, fhirapi.BadRequestError(fmt.Sprintf("resourceType %q does not match the request path (%s)", res.resourceType(), resourceType), nil)
	}
	if id != "" {
		if res.id() != "" && res.id() != id {
			return nil, fhirapi.BadRequestError(fmt.Sprintf("resource id %q does not match the request path (%s)", res.id(), id), nil)
		}
		res["id"] = id
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
	if err := w.requireOwned(ctx, current, "the resource does not belong to the tenant"); err != nil {
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
	if err := w.requireOwned(ctx, current, "the current version in the LRZA does not belong to the tenant"); err != nil {
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
	if err := w.requireOwned(ctx, current, "the current version in the LRZA does not belong to the tenant"); err != nil {
		return nil, err
	}
	if err := w.checkMandate(ctx); err != nil {
		return nil, err
	}
	withdrawn := current.clone()
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
func (w *writeContext) prepare(ctx context.Context, res resource, current resource) error {
	if err := w.enrich(ctx, res, current); err != nil {
		return err
	}
	if err := w.checkNewVersion(ctx, res); err != nil {
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
		body:     stored,
		etag:     etagOf(headers, stored),
		location: headers.Get("Location"),
	}
	if err := w.signProvenance(ctx, stored); err != nil {
		slog.WarnContext(ctx, "LRZA write succeeded, but signing its Provenance failed", logging.Error(err))
		result.warning = fmt.Sprintf(`199 knooppunt "Provenance signing failed: %s"`, strings.ReplaceAll(err.Error(), `"`, `'`))
	}
	w.requestSync()
	return result
}

// ifMatchFor is the If-Match to send with an update: the client's own, unchanged, or else the version
// the ownership check read. The latter also closes the gap between that check and the write: if
// someone else changed the resource in between, the LRZA rejects the write with 412.
func ifMatchFor(clientIfMatch *string, etag string, current resource) string {
	if clientIfMatch != nil && *clientIfMatch != "" {
		return *clientIfMatch
	}
	if etag != "" {
		return etag
	}
	if versionID := current.versionID(); versionID != "" {
		return `W/"` + versionID + `"`
	}
	return ""
}

// etagOf is the ETag of a written resource: the LRZA's header, or else derived from meta.versionId.
func etagOf(headers fhirclient.Headers, stored []byte) string {
	if etag := headers.Get("ETag"); etag != "" {
		return etag
	}
	var res resource
	if json.Unmarshal(stored, &res) == nil {
		if versionID := res.versionID(); versionID != "" {
			return `W/"` + versionID + `"`
		}
	}
	return ""
}

// readCurrent reads the current version of a resource from the LRZA, with its ETag.
func (w *writeContext) readCurrent(ctx context.Context, resourceType, id string) (resource, string, error) {
	var data []byte
	var headers fhirclient.Headers
	var status int
	err := w.client.ReadWithContext(ctx, resourceType+"/"+id, &data,
		fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status))
	if err != nil {
		return nil, "", upstreamError("read "+resourceType+"/"+id+" from", err, status, headers.Header)
	}
	res, err := parseResource(data)
	if err != nil {
		return nil, "", &fhirapi.Error{Message: "LRZA returned an invalid resource", Cause: err, IssueType: fhir.IssueTypeException}
	}
	w.cache[resourceType+"/"+id] = res
	return res, headers.Get("ETag"), nil
}

// lookup reads a resource from the LRZA by relative reference, through the request's cache. It
// returns nil (and no error) if the resource doesn't exist.
func (w *writeContext) lookup(ctx context.Context, resourceType, id string) (resource, error) {
	key := resourceType + "/" + id
	if res, ok := w.cache[key]; ok {
		return res, nil
	}
	var data []byte
	var headers fhirclient.Headers
	var status int
	err := w.client.ReadWithContext(ctx, key, &data, fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status))
	if status == http.StatusNotFound || status == http.StatusGone {
		w.cache[key] = nil
		return nil, nil
	}
	if err != nil {
		return nil, upstreamError("read "+key+" from", err, status, headers.Header)
	}
	res, err := parseResource(data)
	if err != nil {
		return nil, &fhirapi.Error{Message: "LRZA returned an invalid resource", Cause: err, IssueType: fhir.IssueTypeException}
	}
	w.cache[key] = res
	return res, nil
}

// preferRepresentation asks the LRZA to return the stored resource, which is returned to the client
// and signed.
var preferRepresentation = http.Header{"Prefer": []string{"return=representation"}}

func (w *writeContext) create(ctx context.Context, res resource) ([]byte, fhirclient.Headers, error) {
	data, err := json.Marshal(res)
	if err != nil {
		return nil, fhirclient.Headers{}, err
	}
	var stored []byte
	var headers fhirclient.Headers
	var status int
	err = w.client.CreateWithContext(ctx, data, &stored,
		fhirclient.RequestHeaders(preferRepresentation),
		fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status))
	if err != nil {
		return nil, headers, upstreamError("create "+res.resourceType()+" at", err, status, headers.Header)
	}
	stored, err = w.ensureRepresentation(ctx, stored, headers.Get("Location"), res.resourceType())
	return stored, headers, err
}

func (w *writeContext) update(ctx context.Context, res resource, ifMatch string) ([]byte, fhirclient.Headers, error) {
	requestHeaders := preferRepresentation.Clone()
	if ifMatch != "" {
		requestHeaders.Set("If-Match", ifMatch)
	}
	var stored []byte
	var headers fhirclient.Headers
	var status int
	err := w.client.UpdateWithContext(ctx, res.reference(), res, &stored,
		fhirclient.RequestHeaders(requestHeaders),
		fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status))
	if err != nil {
		return nil, headers, upstreamError("update "+res.reference()+" at", err, versionConflictStatus(status, ifMatch != ""), headers.Header)
	}
	location := headers.Get("Location")
	if location == "" {
		location = res.reference()
	}
	stored, err = w.ensureRepresentation(ctx, stored, location, res.resourceType())
	return stored, headers, err
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

// ensureRepresentation returns the stored resource. An LRZA that ignores Prefer: return=representation
// returns no body, in which case the resource is read from the location it was written to.
func (w *writeContext) ensureRepresentation(ctx context.Context, stored []byte, location, resourceType string) ([]byte, error) {
	if len(strings.TrimSpace(string(stored))) > 0 {
		return stored, nil
	}
	if location == "" {
		return nil, &fhirapi.Error{Message: "LRZA returned neither the stored " + resourceType + " nor its location", IssueType: fhir.IssueTypeException}
	}
	var data []byte
	var headers fhirclient.Headers
	var status int
	if err := w.client.ReadWithContext(ctx, location, &data, fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status)); err != nil {
		return nil, upstreamError("read the stored "+resourceType+" from", err, status, headers.Header)
	}
	return data, nil
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
	if c.registrar == nil {
		return nil, errors.New("LRZA registration is disabled")
	}
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
	if c.registrar == nil {
		return nil, errors.New("LRZA registration is disabled")
	}
	result, err := c.registrar.createResource(ctx, request.Params.XTenantID.URA(), string(request.ResourceType), *request.Body)
	if err != nil {
		return errorResponse(ctx, err), nil
	}
	return api.CreateLrzaResource201ApplicationFhirPlusJSONResponse{
		Body: api.FHIRResource(result.body),
		Headers: api.CreateLrzaResource201ResponseHeaders{
			ETag:     optionalHeader(result.etag),
			Location: optionalHeader(result.location),
			Warning:  optionalHeader(result.warning),
		},
	}, nil
}

func (c *Component) ReadLrzaResource(ctx context.Context, request api.ReadLrzaResourceRequestObject) (api.ReadLrzaResourceResponseObject, error) {
	if c.registrar == nil {
		return nil, errors.New("LRZA registration is disabled")
	}
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
	if c.registrar == nil {
		return nil, errors.New("LRZA registration is disabled")
	}
	result, err := c.registrar.updateResource(ctx, request.Params.XTenantID.URA(), string(request.ResourceType), request.Id, request.Params.IfMatch, *request.Body)
	if err != nil {
		return errorResponse(ctx, err), nil
	}
	return api.UpdateLrzaResource200ApplicationFhirPlusJSONResponse{
		Body: api.FHIRResource(result.body),
		Headers: api.UpdateLrzaResource200ResponseHeaders{
			ETag:    optionalHeader(result.etag),
			Warning: optionalHeader(result.warning),
		},
	}, nil
}

func (c *Component) WithdrawLrzaResource(ctx context.Context, request api.WithdrawLrzaResourceRequestObject) (api.WithdrawLrzaResourceResponseObject, error) {
	if c.registrar == nil {
		return nil, errors.New("LRZA registration is disabled")
	}
	result, err := c.registrar.withdrawResource(ctx, request.Params.XTenantID.URA(), string(request.ResourceType), request.Id, request.Params.IfMatch)
	if err != nil {
		return errorResponse(ctx, err), nil
	}
	return api.WithdrawLrzaResource200ApplicationFhirPlusJSONResponse{
		Body: api.FHIRResource(result.body),
		Headers: api.WithdrawLrzaResource200ResponseHeaders{
			ETag:    optionalHeader(result.etag),
			Warning: optionalHeader(result.warning),
		},
	}, nil
}
