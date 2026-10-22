package lrza

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	fhirclient "github.com/SanteonNL/go-fhir-client"
	"github.com/nuts-foundation/nuts-knooppunt/lib/fhirapi"
	"github.com/nuts-foundation/nuts-knooppunt/lib/logging"
	"github.com/zorgbijjou/golang-fhir-models/fhir-models/fhir"
)

// Transactions: several resources registered all-or-nothing, e.g. a new Location with its Endpoint.
// Each entry goes through the same pipeline as a single write, with two differences: entries can
// refer to each other by fullUrl (urn:uuid), and every check runs before anything is sent, so the
// LRZA only gets a transaction that passed as a whole.

// txEntry is one entry of a transaction being processed.
type txEntry struct {
	index        int
	method       fhir.HTTPVerb
	resourceType string
	id           string
	// res is the version to write; for a DELETE, the withdrawn current version.
	res resource
	// current is the version in the LRZA (PUT and DELETE), with its ETag.
	current     resource
	currentETag string
}

func (r *registrar) transaction(ctx context.Context, tenantURA string, body []byte) (json.RawMessage, error) {
	var bundle fhir.Bundle
	if err := json.Unmarshal(body, &bundle); err != nil {
		return nil, fhirapi.BadRequestError("request body is not a valid Bundle: "+err.Error(), err)
	}
	if bundle.Type != fhir.BundleTypeTransaction {
		return nil, fhirapi.BadRequestError(fmt.Sprintf("Bundle.type must be transaction, got %s", bundle.Type.Code()), nil)
	}
	if len(bundle.Entry) == 0 {
		return nil, fhirapi.BadRequestError("the transaction has no entries", nil)
	}

	w := r.newWriteContext(tenantURA)
	entries := make([]*txEntry, len(bundle.Entry))
	for i, entry := range bundle.Entry {
		parsed, err := parseTransactionEntry(i, entry)
		if err != nil {
			return nil, err
		}
		entries[i] = parsed
		// Only a urn: fullUrl names the entry itself. Any other fullUrl is ignored: the LRZA resolves
		// a literal reference like Organization/123 to the resource with that id, so letting an
		// entry claim it would let the ownership check pass for a reference to someone else's.
		if entry.FullUrl != nil && strings.HasPrefix(*entry.FullUrl, "urn:") && parsed.res != nil {
			w.bundle[*entry.FullUrl] = parsed.res
		}
		if parsed.method == fhir.HTTPVerbPUT {
			w.bundle[parsed.resourceType+"/"+parsed.id] = parsed.res
		}
	}

	// The current versions in the LRZA must belong to the tenant, before anything else is looked at.
	for _, entry := range entries {
		if entry.method == fhir.HTTPVerbPOST {
			continue
		}
		current, etag, err := w.readCurrent(ctx, entry.resourceType, entry.id)
		if err != nil {
			return nil, entryError(entry.index, err)
		}
		if err := w.requireOwned(ctx, current, fmt.Sprintf("entry[%d]: the current version in the LRZA does not belong to the tenant", entry.index)); err != nil {
			return nil, err
		}
		entry.current, entry.currentETag = current, etag
		if entry.method == fhir.HTTPVerbDELETE {
			entry.res = current.clone()
			if err := withdraw(entry.res, r.now()); err != nil {
				return nil, entryError(entry.index, err)
			}
		}
	}

	// Enrich every entry first, so references between entries resolve to their enriched versions.
	for _, entry := range entries {
		if entry.method == fhir.HTTPVerbDELETE {
			continue
		}
		if err := w.enrich(ctx, entry.res, entry.current); err != nil {
			return nil, entryError(entry.index, err)
		}
	}
	var issues []validationIssue
	for _, entry := range entries {
		if entry.method == fhir.HTTPVerbDELETE {
			continue
		}
		if err := w.checkNewVersion(ctx, entry.res); err != nil {
			return nil, entryError(entry.index, err)
		}
		for _, issue := range validateResource(entry.res) {
			// Issue expressions start with the resource type; anchor them at the entry instead.
			issue.expression = fmt.Sprintf("Bundle.entry[%d].resource", entry.index) + strings.TrimPrefix(issue.expression, entry.resourceType)
			issues = append(issues, issue)
		}
	}
	if len(issues) > 0 {
		return nil, validationError(issues)
	}
	if err := w.checkMandate(ctx); err != nil {
		return nil, err
	}

	// Rewrite the entries to what is sent: a DELETE becomes a PUT of the withdrawn version, and every
	// update carries the client's ifMatch or the version the ownership check read.
	for i, entry := range entries {
		data, err := json.Marshal(entry.res)
		if err != nil {
			return nil, err
		}
		bundle.Entry[i].Resource = data
		if entry.method == fhir.HTTPVerbPOST {
			continue
		}
		request := bundle.Entry[i].Request
		request.Method = fhir.HTTPVerbPUT
		request.Url = entry.resourceType + "/" + entry.id
		request.IfMatch = new(ifMatchFor(request.IfMatch, entry.currentETag, entry.current))
		if *request.IfMatch == "" {
			request.IfMatch = nil
		}
	}

	var result fhir.Bundle
	var headers fhirclient.Headers
	var status int
	err := w.client.CreateWithContext(ctx, bundle, &result, fhirclient.AtPath("/"),
		fhirclient.RequestHeaders(preferRepresentation),
		fhirclient.ResponseHeaders(&headers), fhirclient.ResponseStatusCode(&status))
	if err != nil {
		withIfMatch := false
		for _, entry := range bundle.Entry {
			withIfMatch = withIfMatch || entry.Request.IfMatch != nil
		}
		return nil, upstreamError("submit the transaction to", err, versionConflictStatus(status, withIfMatch), headers.Header)
	}

	w.signTransactionResult(ctx, &result)
	rewriteResponseLocations(&result)
	w.requestSync()
	return json.Marshal(result)
}

// rewriteResponseLocations makes the entries' response.location relative to the Knooppunt's FHIR base,
// rather than the LRZA's (see relativeLocation).
func rewriteResponseLocations(result *fhir.Bundle) {
	for i, entry := range result.Entry {
		if entry.Response == nil || entry.Response.Location == nil {
			continue
		}
		if location := relativeLocation(entry.Resource, *entry.Response.Location); location != "" {
			result.Entry[i].Response.Location = &location
		} else {
			result.Entry[i].Response.Location = nil
		}
	}
}

// parseTransactionEntry checks an entry is a write the proxy supports, and parses its resource.
func parseTransactionEntry(index int, entry fhir.BundleEntry) (*txEntry, error) {
	fail := func(message string) error {
		return fhirapi.BadRequestError(fmt.Sprintf("entry[%d]: %s", index, message), nil)
	}
	if entry.Request == nil {
		return nil, fail("request is missing")
	}
	if strings.Contains(entry.Request.Url, "?") {
		return nil, fail("conditional requests are not supported")
	}
	result := &txEntry{index: index, method: entry.Request.Method}
	path := strings.Trim(entry.Request.Url, "/")
	parts := strings.Split(path, "/")
	switch entry.Request.Method {
	case fhir.HTTPVerbPOST:
		if len(parts) != 1 {
			return nil, fail("a POST must be to a resource type, e.g. Endpoint")
		}
		result.resourceType = parts[0]
	case fhir.HTTPVerbPUT, fhir.HTTPVerbDELETE:
		if len(parts) != 2 || parts[1] == "" {
			return nil, fail(fmt.Sprintf("a %s must be to a resource, e.g. Endpoint/123", entry.Request.Method.Code()))
		}
		result.resourceType, result.id = parts[0], parts[1]
	default:
		return nil, fail(fmt.Sprintf("method %s is not supported (expected POST, PUT or DELETE)", entry.Request.Method.Code()))
	}
	if err := checkResourceType(result.resourceType); err != nil {
		return nil, entryError(index, err)
	}
	if entry.Request.Method == fhir.HTTPVerbDELETE {
		return result, nil
	}
	if len(entry.Resource) == 0 {
		return nil, fail("resource is missing")
	}
	res, err := parseRequestResource(entry.Resource, result.resourceType, result.id)
	if err != nil {
		return nil, entryError(index, err)
	}
	result.res = res
	return result, nil
}

// entryError prefixes the message of an error with the entry it is about.
func entryError(index int, err error) error {
	prefix := fmt.Sprintf("entry[%d]: ", index)
	switch typed := err.(type) {
	case *registrationError:
		for i, issue := range typed.outcome.Issue {
			if issue.Diagnostics != nil {
				typed.outcome.Issue[i].Diagnostics = new(prefix + *issue.Diagnostics)
			}
		}
		return typed
	case *fhirapi.Error:
		typed.Message = prefix + typed.Message
		return typed
	default:
		return fmt.Errorf("%s%w", prefix, err)
	}
}

// signTransactionResult signs a Provenance for every entry the transaction changed. A failure doesn't
// undo the transaction: signing is optional. It is reported as a warning issue in the entry's
// response.outcome instead.
func (w *writeContext) signTransactionResult(ctx context.Context, result *fhir.Bundle) {
	if w.signers[w.tenantURA] == nil {
		return
	}
	for i, entry := range result.Entry {
		if entry.Response == nil || !strings.HasPrefix(entry.Response.Status, "2") {
			continue
		}
		stored := []byte(entry.Resource)
		var err error
		if len(stored) == 0 && entry.Response.Location != nil {
			stored, err = w.ensureRepresentation(ctx, nil, *entry.Response.Location, "resource")
		}
		if err == nil {
			err = w.signProvenance(ctx, stored)
		}
		if err != nil {
			slog.WarnContext(ctx, "LRZA transaction succeeded, but signing a Provenance failed", slog.Int("entry", i), logging.Error(err))
			result.Entry[i].Response.Outcome = addWarning(entry.Response.Outcome, "Provenance signing failed: "+err.Error())
		}
	}
}

// addWarning adds a warning issue to an entry's response.outcome, creating it if absent.
func addWarning(outcome json.RawMessage, message string) json.RawMessage {
	var parsed fhir.OperationOutcome
	if len(outcome) > 0 {
		_ = json.Unmarshal(outcome, &parsed)
	}
	parsed.Issue = append(parsed.Issue, fhir.OperationOutcomeIssue{
		Severity:    fhir.IssueSeverityWarning,
		Code:        fhir.IssueTypeIncomplete,
		Diagnostics: &message,
	})
	data, err := json.Marshal(parsed)
	if err != nil {
		return outcome
	}
	return data
}
